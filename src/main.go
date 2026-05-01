package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
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
	CleanupDelay        = 10 * time.Second
	StartupTimeout      = 30 * time.Second
	SourceRetryInterval = 60 * time.Second
	SourceCheckTimeout  = 15 * time.Second
	TVHCheckInterval    = 3 * time.Second
	FallbackURL         = "https://theariatv.github.io/channeldead.mp4"
    TVHGracePeriod      = 30 * time.Second
)

func main() {
	startTime = time.Now()
	exePath, _ := os.Executable()
	scriptDir = filepath.Dir(exePath)
	configFilePath = filepath.Join(scriptDir, "config.json")

	loadConfig()
	detectXtream()
	
	// Start background channel name refresher
	go refreshChannelNamesLoop()

	go monitorTVH()
	go monitorSourceRecovery()

	mux := http.NewServeMux()
	setupAPIRoutes(mux)

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
		log.Fatalf("Failed to parse config: %v", err)
	}
	configLock.Unlock()
	log.Printf("Loaded config successfully. AutoFallback: %v, Sources: %d", Config.AutoFallback, len(Config.Sources))
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

func getProxies() []string {
	configLock.RLock()
	defer configLock.RUnlock()
	if len(Config.Ppproxies) > 0 {
		return Config.Ppproxies
	}
	return Config.Proxies
}
