package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

func setupM3URoutes(mux *http.ServeMux) {
	// 1. Generate M3U for multiple categories
	mux.HandleFunc("GET /api/playlist/bulk.m3u", func(w http.ResponseWriter, r *http.Request) {
		providerID := r.URL.Query().Get("provider")
		categories := r.URL.Query().Get("categories") // comma separated
		if providerID == "" || categories == "" {
			http.Error(w, "provider and categories are required", 400)
			return
		}

		// Check Cache
		hashKey := md5.Sum([]byte(r.URL.RawQuery))
		cacheFile := fmt.Sprintf("m3u_cache_%x.m3u", hashKey)
		if info, err := os.Stat(cacheFile); err == nil && r.URL.Query().Get("force") != "1" {
			if time.Since(info.ModTime()) < 12*time.Hour {
				http.ServeFile(w, r, cacheFile)
				return
			}
		}

		configLock.RLock()
		provider, exists := Config.XtreamProviders[providerID]
		configLock.RUnlock()
		if !exists {
			http.Error(w, "Provider not found", 404)
			return
		}

		proxyHost := strings.Split(r.Host, ":")[0]
		if proxyHost == "" {
			proxyHost = "localhost"
		}

		epgUrl := r.URL.Query().Get("epg_url")
		stripCountry := r.URL.Query().Get("strip_country") == "1"

		var allStreams []map[string]interface{}
		catIDs := strings.Split(categories, ",")
		seenNames := make(map[string]bool)
		
		for _, catID := range catIDs {
			catID = strings.TrimSpace(catID)
			if catID == "" {
				continue
			}

			streamsData := getStreams(provider, catID, "live")
			if streamsData != nil {
				for _, item := range streamsData {
					if m, ok := item.(map[string]interface{}); ok {
						streamName := fmt.Sprintf("%v", m["name"])
						norm := NormalizeChannelName(streamName)
						
						if !seenNames[norm] {
							seenNames[norm] = true
							allStreams = append(allStreams, m)
						}
					}
				}
			}
		}

		// Collect unmapped
		var unmapped []string
		epgMappingLock.RLock()
		for _, m := range allStreams {
			streamName := fmt.Sprintf("%v", m["name"])
			if IsCategorySeparator(streamName) {
				continue
			}
			if _, ok := EPGMapping[streamName]; !ok {
				unmapped = append(unmapped, streamName)
			}
		}
		epgMappingLock.RUnlock()

		// Run Auto-EPG if requested
		if epgUrl != "" && len(unmapped) > 0 {
			configLock.RLock()
			apiKey := Config.GeminiAPIKey
			ollamaUrl := Config.OllamaAPIUrl
			ollamaKey := Config.OllamaAPIKey
			ollamaModel := Config.OllamaModel
			configLock.RUnlock()

			if ollamaUrl != "" || apiKey != "" {
				epgChannels, err := FetchEPGChannelIDs(epgUrl)
				if err == nil {
					var matched map[string]string
					var matchErr error

					if ollamaUrl != "" {
						matched, matchErr = MatchWithOllama(ollamaUrl, ollamaKey, ollamaModel, unmapped, epgChannels)
					} else {
						apiKeys := strings.Split(apiKey, ",")
						for i := range apiKeys {
							apiKeys[i] = strings.TrimSpace(apiKeys[i])
						}
						matched, matchErr = MatchWithGemini(apiKeys, unmapped, epgChannels)
					}

					if matchErr != nil {
						fmt.Printf("Auto-EPG AI match failed: %v\nFalling back to purely offline fuzzy matching...\n", matchErr)
						matched = make(map[string]string)
						var allIDs []string
						for id := range epgChannels {
							allIDs = append(allIDs, id)
						}
						for _, ch := range unmapped {
							top := GetTopCandidates(ch, allIDs, 1, 10)
							if len(top) > 0 {
								// Basic safety check: don't randomly assign if name is too short
								if len(ch) > 2 {
									matched[ch] = top[0]
								}
							}
						}
					}

					epgMappingLock.Lock()
					for k, v := range matched {
						if v != "" {
							EPGMapping[k] = v
						}
					}
					epgMappingLock.Unlock()
					saveEpgMapping()
				} else {
					fmt.Printf("Auto-EPG Fetch EPG failed: %v\n", err)
				}
			}
		}

		// Generate output
		var m3uLines []string
		m3uLines = append(m3uLines, "#EXTM3U")

		for _, m := range allStreams {
			var streamID string
			if val, ok := m["stream_id"]; ok && val != nil {
				if f, isFloat := val.(float64); isFloat {
					streamID = fmt.Sprintf("%.0f", f)
				} else {
					streamID = fmt.Sprintf("%v", val)
				}
			}

			streamName := fmt.Sprintf("%v", m["name"])
			
			if IsCategorySeparator(streamName) {
				continue
			}

			epgID := fmt.Sprintf("%v", m["epg_channel_id"])
			if epgID == "<nil>" {
				epgID = ""
			}

			epgMappingLock.RLock()
			if mappedID, ok := EPGMapping[streamName]; ok && mappedID != "" {
				epgID = mappedID
			}
			epgMappingLock.RUnlock()

			displayName := CleanChannelNameForM3U(streamName, stripCountry)
			extinf := fmt.Sprintf(`#EXTINF:-1 tvg-id="%s" tvg-name="%s",%s`, epgID, displayName, displayName)
			m3uLines = append(m3uLines, extinf)

			proxyURL := fmt.Sprintf("http://%s:%d/%s/%s.ts", proxyHost, ProxyPort, provider.SourceID, streamID)
			m3uLines = append(m3uLines, proxyURL)
		}

		m3uContent := strings.Join(m3uLines, "\n")
		os.WriteFile(cacheFile, []byte(m3uContent), 0644)

		w.Header().Set("Content-Type", "audio/mpegurl")
		w.Header().Set("Content-Disposition", `attachment; filename="bulk_playlist.m3u"`)
		w.Write([]byte(m3uContent))
	})

	// 2. EPG Matcher with Gemini
	mux.HandleFunc("POST /api/epg/match", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			EpgUrl   string   `json:"epg_url"`
			Channels []string `json:"channels"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", 400)
			return
		}

		configLock.RLock()
		apiKey := Config.GeminiAPIKey
		ollamaUrl := Config.OllamaAPIUrl
		ollamaKey := Config.OllamaAPIKey
		ollamaModel := Config.OllamaModel
		configLock.RUnlock()

		if apiKey == "" && ollamaUrl == "" {
			http.Error(w, `{"error":"No AI API Key or Ollama URL is configured in config.json"}`, 400)
			return
		}

		epgChannels, err := FetchEPGChannelIDs(req.EpgUrl)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Failed to fetch EPG: %v"}`, err), 500)
			return
		}

		var matched map[string]string
		var matchErr error

		if ollamaUrl != "" {
			matched, matchErr = MatchWithOllama(ollamaUrl, ollamaKey, ollamaModel, req.Channels, epgChannels)
		} else {
			apiKeys := strings.Split(apiKey, ",")
			for i := range apiKeys {
				apiKeys[i] = strings.TrimSpace(apiKeys[i])
			}
			matched, matchErr = MatchWithGemini(apiKeys, req.Channels, epgChannels)
		}

		if matchErr != nil {
			fmt.Printf("Manual AI Match failed: %v. Using purely offline fallback.\n", matchErr)
			matched = make(map[string]string)
			var allIDs []string
			for id := range epgChannels {
				allIDs = append(allIDs, id)
			}
			for _, ch := range req.Channels {
				top := GetTopCandidates(ch, allIDs, 1, 10)
				if len(top) > 0 && len(ch) > 2 {
					matched[ch] = top[0]
				}
			}
		}

		epgMappingLock.Lock()
		for k, v := range matched {
			if v != "" {
				EPGMapping[k] = v
			}
		}
		epgMappingLock.Unlock()

		saveEpgMapping()

		sendJSON(w, map[string]interface{}{
			"status": "success",
			"matched_count": len(matched),
			"matches": matched,
		})
	})

	// 3. Get EPG Mapping
	mux.HandleFunc("GET /api/epg/mapping", func(w http.ResponseWriter, r *http.Request) {
		epgMappingLock.RLock()
		defer epgMappingLock.RUnlock()
		sendJSON(w, EPGMapping)
	})

	// 4. Update EPG Mapping manually
	mux.HandleFunc("POST /api/epg/mapping", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", 400)
			return
		}

		epgMappingLock.Lock()
		for k, v := range req {
			EPGMapping[k] = v
		}
		epgMappingLock.Unlock()

		saveEpgMapping()
		sendJSON(w, map[string]string{"status": "saved"})
	})
}
