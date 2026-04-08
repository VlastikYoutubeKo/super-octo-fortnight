package main

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
    "math/rand"
    "os"
)

func checkSourceHealth(testUrl string) bool {
	if testUrl == FallbackURL {
		return false
	}

	testUrl = strings.Replace(testUrl, "{channel_id}", "1", -1)
	
	client := &http.Client{
		Timeout: SourceCheckTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow to avoid downloading
		},
	}
    
    proxies := getProxies()
    if len(proxies) > 0 {
        proxyUrl, err := url.Parse(proxies[rand.Intn(len(proxies))])
        if err == nil {
            client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
        }
    }

	req, err := http.NewRequest("HEAD", testUrl, nil)
	if err != nil {
		return false
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	
	return resp.StatusCode == 200 || resp.StatusCode == 302 || resp.StatusCode == 404
}

func restartStreamWithSource(key string, sourceIdx int) bool {
	streamsLock.Lock()
	s, exists := streams[key]
	if !exists {
		streamsLock.Unlock()
		return false
	}

	s.Mu.Lock()
	onFallback := s.OnFallback
	clients := len(s.Clients)
	if !onFallback || clients == 0 {
		s.Mu.Unlock()
		streamsLock.Unlock()
		return false
	}
    
    if sourceIdx >= len(s.Urls) {
        s.Mu.Unlock()
		streamsLock.Unlock()
		return false
    }

	workingUrl := s.Urls[sourceIdx]
	newUrls := []string{workingUrl}
	hasFallback := false
	for _, u := range s.Urls {
		if u == FallbackURL {
			hasFallback = true
			break
		}
	}
	if hasFallback {
		newUrls = append(newUrls, FallbackURL)
	}

	s.Urls = newUrls
	s.OnFallback = false
	s.LastRetry = time.Now()
    
    var procToKill *os.Process
    if s.Proc != nil && s.Proc.Process != nil {
        procToKill = s.Proc.Process
    }
    s.Proc = nil

	s.Mu.Unlock()
	streamsLock.Unlock()

    if procToKill != nil {
        procToKill.Kill()
    }

	log.Printf("Recovering %s - switching from fallback to source", key)
	return true
}

func monitorSourceRecovery() {
	log.Println("Source recovery monitor started")
	for {
		time.Sleep(SourceRetryInterval)

		type fallbackStream struct {
			key  string
			urls []string
		}
		var targets []fallbackStream

		streamsLock.RLock()
		for key, s := range streams {
			s.Mu.RLock()
			onFallback := s.OnFallback
			clients := len(s.Clients)
			lastRetry := s.LastRetry
			s.Mu.RUnlock()

			if onFallback && clients > 0 {
				if time.Since(lastRetry) >= SourceRetryInterval {
					urlsCopy := make([]string, len(s.Urls))
					copy(urlsCopy, s.Urls)
					targets = append(targets, fallbackStream{key, urlsCopy})
				}
			}
		}
		streamsLock.RUnlock()

		for _, t := range targets {
			log.Printf("Checking source recovery for %s", t.key)
			recovered := false
			for idx, u := range t.urls {
				if u == FallbackURL {
					continue
				}
				if checkSourceHealth(u) {
					log.Printf("Source recovered for %s at URL index %d", t.key, idx)
					restartStreamWithSource(t.key, idx)
					recovered = true
					break
				}
			}
			
			if !recovered {
				streamsLock.RLock()
				if s, exists := streams[t.key]; exists {
					s.Mu.Lock()
					s.LastRetry = time.Now()
					s.Mu.Unlock()
					log.Printf("No sources available yet for %s, will retry later", t.key)
				}
				streamsLock.RUnlock()
			}
		}
	}
}

type tvhSubResponse struct {
    Entries []struct {
        ServerURL string `json:"server_url"`
    } `json:"entries"`
}

func monitorTVH() {
	// Disabled to prevent aggressive cleanup
    log.Println("TVHeadend monitor disabled (stability mode)")
    for {
        time.Sleep(1 * time.Hour)
    }
}