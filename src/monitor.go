package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
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

	var cancelFunc context.CancelFunc
	if s.CancelFunc != nil {
		cancelFunc = s.CancelFunc
	}
	s.CancelFunc = nil

	s.Mu.Unlock()
	streamsLock.Unlock()

	if cancelFunc != nil {
		cancelFunc()
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
        Channel   string `json:"channel"`
    } `json:"entries"`
}

func monitorTVH() {
	log.Println("TVHeadend monitor started")
	for {
		configLock.RLock()
		tvhUrl := Config.TVHeadend.URL
		tvhUser := Config.TVHeadend.Username
		tvhPass := Config.TVHeadend.Password
		configLock.RUnlock()

		if tvhUrl == "" {
			time.Sleep(TVHCheckInterval)
			continue
		}

		client := &http.Client{Timeout: 5 * time.Second}
		req, err := http.NewRequest("GET", tvhUrl+"/api/status/subscriptions", nil)
		if err == nil {
			req.SetBasicAuth(tvhUser, tvhPass)
			resp, err := client.Do(req)
			if err != nil {
				log.Printf("TVH Monitor: API request failed: %v", err)
			} else {
				if resp.StatusCode == 200 {
					var data tvhSubResponse
					bodyCopy, _ := io.ReadAll(resp.Body)
					resp.Body.Close()

					if err := json.Unmarshal(bodyCopy, &data); err != nil {
						log.Printf("TVH Monitor: JSON decode failed: %v | Body: %s", err, string(bodyCopy))
					} else {
						active := make(map[string]bool)
						activeNames := make(map[string]bool)
						for _, sub := range data.Entries {
							u := sub.ServerURL
							activeNames[sub.Channel] = true
							log.Printf("TVH Active Channel Name: '%s'", sub.Channel)
							
							parsed, err := url.Parse(u)
							if err == nil && parsed.Path != "" {
								path := strings.TrimPrefix(parsed.Path, "/")
								parts := strings.Split(path, "/")
								if len(parts) == 2 {
									channelID := strings.Split(parts[1], ".")[0]
									key := fmt.Sprintf("%s:%s", parts[0], channelID)
									active[key] = true
									log.Printf("TVH Active Sub (URL): %s", key)
								}
							}
						}

						streamsLock.Lock()
						channelNamesLock.RLock()
						for key, s := range streams {
							name := strings.ToLower(ChannelNames[key])
							isActuallyActive := active[key]
							
							if !isActuallyActive && name != "" {
								for activeName := range activeNames {
									an := strings.ToLower(activeName)
									if strings.Contains(name, an) || strings.Contains(an, name) {
										isActuallyActive = true
										break
									}
								}
							}
							
							if !isActuallyActive {
								s.Mu.RLock()
								clients := len(s.Clients)
								age := time.Since(s.Created)
								stillActive := s.Active
								s.Mu.RUnlock()

								if clients == 0 && age >= TVHGracePeriod && !stillActive {
									log.Printf("TVH: cleanup %s (Name: '%s', Clients: 0, Age: %v, Active: %v)", key, ChannelNames[key], age, stillActive)
									go cleanupStream(key)
								}
							}
						}
						channelNamesLock.RUnlock()
						streamsLock.Unlock()
					}
				} else {
					log.Printf("TVH Monitor: API returned status %d", resp.StatusCode)
					resp.Body.Close()
				}
			}
		}

		cooldownsLock.Lock()
		now := time.Now()
		for k, v := range cooldowns {
			if now.After(v) {
				delete(cooldowns, k)
			}
		}
		cooldownsLock.Unlock()

		time.Sleep(TVHCheckInterval)
	}
}
