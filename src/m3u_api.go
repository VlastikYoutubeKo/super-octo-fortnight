package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

		var allStreams []map[string]interface{}
		catIDs := strings.Split(categories, ",")
		for _, catID := range catIDs {
			catID = strings.TrimSpace(catID)
			if catID == "" {
				continue
			}

			streamsData := getStreams(provider, catID, "live")
			if streamsData != nil {
				for _, item := range streamsData {
					if m, ok := item.(map[string]interface{}); ok {
						allStreams = append(allStreams, m)
					}
				}
			}
		}

		// Collect unmapped
		var unmapped []string
		epgMappingLock.RLock()
		for _, m := range allStreams {
			streamName := fmt.Sprintf("%v", m["name"])
			if _, ok := EPGMapping[streamName]; !ok {
				unmapped = append(unmapped, streamName)
			}
		}
		epgMappingLock.RUnlock()

		// Run Auto-EPG if requested
		if epgUrl != "" && len(unmapped) > 0 {
			configLock.RLock()
			apiKey := Config.GeminiAPIKey
			configLock.RUnlock()

			if apiKey != "" {
				epgChannels, err := FetchEPGChannelIDs(epgUrl)
				if err == nil {
					// We might have many unmapped, Gemini might have limits. 
					// For safety, we match all of them, but in production we'd chunk them.
					matched, err := MatchWithGemini(apiKey, unmapped, epgChannels)
					if err == nil {
						epgMappingLock.Lock()
						for k, v := range matched {
							if v != "" {
								EPGMapping[k] = v
							}
						}
						epgMappingLock.Unlock()
						saveEpgMapping()
					} else {
						// log error but continue generating M3U
						fmt.Printf("Auto-EPG Gemini match failed: %v\n", err)
					}
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
			epgID := fmt.Sprintf("%v", m["epg_channel_id"])
			if epgID == "<nil>" {
				epgID = ""
			}

			epgMappingLock.RLock()
			if mappedID, ok := EPGMapping[streamName]; ok && mappedID != "" {
				epgID = mappedID
			}
			epgMappingLock.RUnlock()

			displayName := CleanChannelNameForM3U(streamName)
			extinf := fmt.Sprintf(`#EXTINF:-1 tvg-id="%s" tvg-name="%s",%s`, epgID, displayName, displayName)
			m3uLines = append(m3uLines, extinf)

			proxyURL := fmt.Sprintf("http://%s:%d/%s/%s.ts", proxyHost, ProxyPort, provider.SourceID, streamID)
			m3uLines = append(m3uLines, proxyURL)
		}

		m3uContent := strings.Join(m3uLines, "\n")
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
		configLock.RUnlock()

		if apiKey == "" {
			http.Error(w, `{"error":"Gemini API Key is not configured in config.json"}`, 400)
			return
		}

		epgChannels, err := FetchEPGChannelIDs(req.EpgUrl)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Failed to fetch EPG: %v"}`, err), 500)
			return
		}

		matched, err := MatchWithGemini(apiKey, req.Channels, epgChannels)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"AI Matching failed: %v"}`, err), 500)
			return
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
