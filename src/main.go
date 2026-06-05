package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

//go:embed config.example.json
var defaultConfig []byte

//go:embed index.html
var defaultHTML []byte

type TVHConfig struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type Provider struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	SourceID string `json:"source_id"`
}

type AppConfig struct {
	Sources         map[string][]string `json:"sources"`
	XtreamProviders map[string]Provider `json:"xtream_providers"`
	FallbackMode    bool                `json:"fallback_mode"`
	AutoFallback    bool                `json:"auto_fallback"`
	RedirectMode    bool                `json:"redirect_mode"`
	InternalM3u8    bool                `json:"internal_m3u8"`
	Engine          string              `json:"engine"`
	TVHeadend       TVHConfig           `json:"tvheadend"`
	Proxies         []string            `json:"proxies"`
	Ppproxies       []string            `json:"ppproxies"`
	AllowedIPs      []string            `json:"allowed_ips"`
	AllowedDomains  []string            `json:"allowed_domains"`
	GeminiAPIKey    string              `json:"gemini_api_key"`
	OllamaAPIUrl    string              `json:"ollama_api_url"`
	OllamaAPIKey    string              `json:"ollama_api_key"`
	OllamaModel     string              `json:"ollama_model"`
}

var (
	Config       AppConfig
	configLock   sync.RWMutex
	streams      = make(map[string]*Stream)
	streamsLock  sync.RWMutex
	startTime    time.Time
	
	// Global Channel Name Cache (SourceID:ChannelID -> Name)
	ChannelNames     = make(map[string]string)
	channelNamesLock sync.RWMutex
	
	// EPG Mapping (Stream Name -> tvg-id)
	EPGMapping       = make(map[string]string)
	epgMappingLock   sync.RWMutex

	cooldowns     = make(map[string]time.Time)
	cooldownsLock sync.RWMutex

	configFilePath string
	scriptDir      string
)

const (
	ProxyPort           = 9000
	APIPort             = 9005
	StreamBuffer        = 5264 // 28 TS packets (188 bytes each)
	BufferQueueSize     = 5000 // ~26MB buffer per client
	DataTimeout         = 60 * time.Second
	CleanupDelay        = 30 * time.Second
	StartupTimeout      = 30 * time.Second
	SourceRetryInterval = 60 * time.Second
	SourceCheckTimeout  = 15 * time.Second
	TVHCheckInterval    = 3 * time.Second
	FallbackURL         = "https://theariatv.github.io/channeldead.mp4"
    TVHGracePeriod      = 60 * time.Second
)

func main() {
	startTime = time.Now()
	exePath, _ := os.Executable()
	scriptDir = filepath.Dir(exePath)
	configFilePath = filepath.Join(scriptDir, "config.json")

	loadConfig()
	detectXtream()
	loadEpgMapping()
	
	// Download fallback video locally so ffmpeg starts instantly
	go downloadFallbackVideo()
	
	// Start background channel name refresher
	go refreshChannelNamesLoop()

	go monitorTVH()
	go monitorSourceRecovery()

	mux := http.NewServeMux()
	setupAPIRoutes(mux)
	setupM3URoutes(mux)

	go func() {
		log.Printf("API running on :%d", APIPort)
		if err := http.ListenAndServe(fmt.Sprintf(":%d", APIPort), mux); err != nil {
			log.Fatalf("API failed: %v", err)
		}
	}()

	log.Printf("Proxy running on :%d", ProxyPort)
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", ProxyPort),
		Handler: http.HandlerFunc(handleProxy),
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Proxy failed: %v", err)
		}
	}()

	<-stop
	log.Println("\nShutting down... cleaning up active pure HTTP streams...")
	streamsLock.Lock()
	for key, stream := range streams {
		stream.Mu.Lock()
		if stream.CancelFunc != nil {
			log.Printf("Stopping pure HTTP stream: %s", key)
			stream.CancelFunc()
		}
		stream.Mu.Unlock()
	}
	streamsLock.Unlock()
	log.Println("Cleanup complete. Goodbye!")
}

func loadConfig() {
	if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
		log.Println("config.json not found, creating from example...")
		os.WriteFile(configFilePath, defaultConfig, 0644)
	}

	file, err := os.ReadFile(configFilePath)
	if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}

	configLock.Lock()
	if err := json.Unmarshal(file, &Config); err != nil {
		log.Printf("Failed to decode config.json: %v", err)
	}

	configLock.Unlock()
	log.Printf("Loaded config successfully. AutoFallback: %v, Sources: %d", Config.AutoFallback, len(Config.Sources))
}

func downloadFallbackVideo() {
	localPath := filepath.Join(scriptDir, "fallback.mp4")
	if _, err := os.Stat(localPath); err == nil {
		return // already downloaded
	}

	log.Printf("Downloading fallback video to %s...", localPath)
	resp, err := http.Get(FallbackURL)
	if err != nil {
		log.Printf("Warning: failed to download fallback video: %v", err)
		return
	}
	defer resp.Body.Close()

	out, err := os.Create(localPath)
	if err != nil {
		log.Printf("Warning: failed to create fallback video file: %v", err)
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		log.Printf("Warning: failed to write fallback video: %v", err)
	} else {
		log.Printf("Fallback video successfully downloaded.")
	}
}

func saveConfig() {
	configLock.RLock()
	data, err := json.MarshalIndent(Config, "", "  ")
	configLock.RUnlock()
	if err != nil {
		log.Printf("Failed to marshal config: %v", err)
		return
	}
	if err := os.WriteFile(configFilePath, data, 0644); err != nil {
		log.Printf("Failed to save config: %v", err)
	} else {
		log.Printf("Config saved to config.json")
	}
}

func loadEpgMapping() {
	mappingFile := filepath.Join(scriptDir, "epg_mapping.json")
	file, err := os.Open(mappingFile)
	if err != nil {
		return
	}
	defer file.Close()

	epgMappingLock.Lock()
	defer epgMappingLock.Unlock()
	json.NewDecoder(file).Decode(&EPGMapping)
	log.Printf("Loaded %d EPG mappings", len(EPGMapping))
}

func saveEpgMapping() {
	mappingFile := filepath.Join(scriptDir, "epg_mapping.json")
	file, err := os.Create(mappingFile)
	if err != nil {
		log.Printf("Failed to create epg mapping file: %v", err)
		return
	}
	defer file.Close()
	
	epgMappingLock.RLock()
	defer epgMappingLock.RUnlock()
	json.NewEncoder(file).Encode(EPGMapping)
}

func getProxies() []string {
	configLock.RLock()
	defer configLock.RUnlock()
	if len(Config.Ppproxies) > 0 {
		return Config.Ppproxies
	}
	return Config.Proxies
}
