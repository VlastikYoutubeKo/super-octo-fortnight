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
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed config.example.json
var defaultConfig []byte

//go:embed index.html
var defaultHTML []byte

const (
	ProxyPort           = 9000
	APIPort             = 9005
	FfmpegBuffer        = 65536 // 64KB
	BufferQueueSize     = 5000
	DataTimeout         = 12 * time.Second
	CleanupDelay        = 10 * time.Second
	StartupTimeout      = 30 * time.Second
	SourceRetryInterval = 60 * time.Second
	SourceCheckTimeout  = 15 * time.Second
	TVHCheckInterval    = 3 * time.Second
	TVHGracePeriod      = 10 * time.Second
	FallbackURL         = "https://theariatv.github.io/channeldead.mp4"
)

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
	TVHeadend       TVHConfig           `json:"tvheadend"`
	Proxies         []string            `json:"proxies"`
	Ppproxies       []string            `json:"ppproxies"`
	AllowedIPs      []string            `json:"allowed_ips"`
	AllowedDomains  []string            `json:"allowed_domains"`
}

var (
	configFilePath string
	Config         AppConfig
	configLock     sync.RWMutex

	streams     = make(map[string]*Stream)
	streamsLock sync.RWMutex
	startTime   = time.Now()

	cooldowns     = make(map[string]time.Time)
	cooldownsLock sync.RWMutex
    
    scriptDir string
)

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
    ex, err := os.Executable()
    if err != nil {
        scriptDir = "."
    } else {
        scriptDir = filepath.Dir(ex)
    }
}

func getProxies() []string {
    configLock.RLock()
    defer configLock.RUnlock()
    if len(Config.Proxies) > 0 {
        return Config.Proxies
    }
    return Config.Ppproxies
}

func loadConfig() {
	// Create index.html from embedded example if it doesn't exist
	if _, err := os.Stat("index.html"); os.IsNotExist(err) {
		log.Printf("index.html not found, creating a new one from template")
		if err := os.WriteFile("index.html", defaultHTML, 0644); err != nil {
			log.Printf("Warning: Failed to create default index.html: %v", err)
		}
	}

	configFilePath = filepath.Join("..", "config.json")
	if _, err := os.Stat("config.json"); err == nil {
		configFilePath = "config.json"
	} else if os.IsNotExist(err) {
		// Create config.json from embedded example
		log.Printf("config.json not found, creating a new one from template")
		if err := os.WriteFile("config.json", defaultConfig, 0644); err != nil {
			log.Printf("Warning: Failed to create default config.json: %v", err)
		} else {
			configFilePath = "config.json"
		}
	}

	data, err := os.ReadFile(configFilePath)
	if err != nil {
		log.Printf("Warning: Could not read config file %s: %v", configFilePath, err)
        configLock.Lock()
        Config.AutoFallback = true
        Config.Sources = make(map[string][]string)
        Config.XtreamProviders = make(map[string]Provider)
        configLock.Unlock()
		return
	}

    var newConfig AppConfig
	if err := json.Unmarshal(data, &newConfig); err != nil {
		log.Printf("Error: Failed to parse config JSON: %v", err)
	} else {
        configLock.Lock()
        if newConfig.Sources == nil {
            newConfig.Sources = make(map[string][]string)
        }
        if newConfig.XtreamProviders == nil {
            newConfig.XtreamProviders = make(map[string]Provider)
        }
        if newConfig.AllowedIPs == nil {
            newConfig.AllowedIPs = make([]string, 0)
        }
        if newConfig.AllowedDomains == nil {
            newConfig.AllowedDomains = make([]string, 0)
        }
        Config = newConfig
        configLock.Unlock()
		log.Printf("Loaded config successfully. AutoFallback: %v, Sources: %d", Config.AutoFallback, len(Config.Sources))
	}
    detectXtream()
    saveConfig()
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
        log.Printf("Config saved to %s", configFilePath)
    }
}

func main() {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("IPTV Stream Proxy Manager (Go Production)")
	fmt.Println(strings.Repeat("=", 60))

	loadConfig()

	go monitorTVH()
	go monitorSourceRecovery()

	// Setup Proxy Server
	proxyMux := http.NewServeMux()
	proxyMux.HandleFunc("/", handleProxy)
	
	// Setup API Server
	apiMux := http.NewServeMux()
    setupAPIRoutes(apiMux)

	go func() {
		log.Printf("API running on :%d", APIPort)
		if err := http.ListenAndServe(fmt.Sprintf(":%d", APIPort), apiMux); err != nil {
			log.Fatalf("API server failed: %v", err)
		}
	}()

	go func() {
		log.Printf("Proxy running on :%d", ProxyPort)
		if err := http.ListenAndServe(fmt.Sprintf(":%d", ProxyPort), proxyMux); err != nil {
			log.Fatalf("Proxy server failed: %v", err)
		}
	}()

	// QoL: Graceful Shutdown
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)
	<-stopChan

	log.Println("\nShutting down... cleaning up active ffmpeg streams...")
	streamsLock.Lock()
	for key, stream := range streams {
		stream.Mu.Lock()
		if stream.Proc != nil && stream.Proc.Process != nil {
			log.Printf("Killing ffmpeg for stream: %s", key)
			stream.Proc.Process.Kill()
		}
		stream.Mu.Unlock()
	}
	streamsLock.Unlock()
	log.Println("Cleanup complete. Goodbye!")
}
