package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
    "os"
    "log"
)

func sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	json.NewEncoder(w).Encode(data)
}

func setupAPIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		streamsLock.RLock()
		defer streamsLock.RUnlock()
		
		totalClients := 0
		fallbackCount := 0
		var streamList []map[string]interface{}
		
		for key, s := range streams {
			s.Mu.RLock()
			clients := len(s.Clients)
			urlStr := "N/A"
			if len(s.Urls) > 0 && s.CurrentUrlIdx < len(s.Urls) {
				urlStr = s.Urls[s.CurrentUrlIdx]
			}
			onFallback := s.OnFallback
			age := time.Since(s.Created).Seconds()
			lastRetry := s.LastRetry

			bytesRead := s.CurrentBytesRead
			processAge := time.Since(s.CurrentProcessStart).Seconds()

			mbps := s.CurrentBitrate
			s.Mu.RUnlock()

			kbps := 0
			if processAge > 0 {
				kbps = int((float64(bytesRead) * 8.0 / 1000.0) / processAge)
			}

			totalClients += clients
			if onFallback {
				fallbackCount++
			}

			var nextRetry interface{} = nil
			if onFallback && !lastRetry.IsZero() {
			nr := int(lastRetry.Add(SourceRetryInterval).Sub(time.Now()).Seconds())
			if nr < 0 { nr = 0 }
			nextRetry = nr
			}

			streamList = append(streamList, map[string]interface{}{
				"key":         key,
				"clients":     clients,
				"age":         int(age),
				"url":         urlStr,
				"on_fallback": onFallback,
			"next_retry":  nextRetry,
				"kbps":        kbps,
				"mbps":        mbps,
			})		}

        if streamList == nil {
            streamList = make([]map[string]interface{}, 0)
        }

        cooldownsLock.RLock()
        var cdList []map[string]interface{}
        now := time.Now()
        for k, v := range cooldowns {
            if v.After(now) {
                cdList = append(cdList, map[string]interface{}{
                    "key": k,
                    "remaining": int(v.Sub(now).Seconds()),
                })
            }
        }
        cooldownsLock.RUnlock()

        if cdList == nil {
            cdList = make([]map[string]interface{}, 0)
        }

        configLock.RLock()
        fm := Config.FallbackMode
        af := Config.AutoFallback
        rm := Config.RedirectMode
        im := Config.InternalM3u8
        eng := Config.Engine
        if eng == "" { eng = "http" }
        configLock.RUnlock()

        sendJSON(w, map[string]interface{}{
        "streams":          streamList,
        "total_streams":    len(streamList),
        "total_clients":    totalClients,
        "fallback_streams": fallbackCount,
        "fallback_mode":    fm,
            "auto_fallback":    af,
            "redirect_mode":    rm,
            "internal_m3u8":    im,
            "engine":           eng,
            "retry_interval":   int(SourceRetryInterval.Seconds()),			"uptime":           int(time.Since(startTime).Seconds()),
            "cooldowns":        cdList,
		})
	})

	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
        configLock.RLock()
		sendJSON(w, Config)
        configLock.RUnlock()
	})

	mux.HandleFunc("POST /api/config", func(w http.ResponseWriter, r *http.Request) {
		var data AppConfig
		if err := json.NewDecoder(r.Body).Decode(&data); err == nil {
            configLock.Lock()
            if data.Sources != nil { Config.Sources = data.Sources }
            Config.FallbackMode = data.FallbackMode
            Config.AutoFallback = data.AutoFallback
            Config.RedirectMode = data.RedirectMode
            Config.InternalM3u8 = data.InternalM3u8
            if data.Engine != "" { Config.Engine = data.Engine } else { Config.Engine = "http" }
            if data.TVHeadend.URL != "" { Config.TVHeadend = data.TVHeadend }
            if data.Proxies != nil { Config.Proxies = data.Proxies }
            if data.Ppproxies != nil { Config.Ppproxies = data.Ppproxies }
            if data.AllowedIPs != nil { Config.AllowedIPs = data.AllowedIPs }
            if data.AllowedDomains != nil { Config.AllowedDomains = data.AllowedDomains }
            configLock.Unlock()
            
			detectXtream()
			saveConfig()
		}
		sendJSON(w, map[string]bool{"success": true})
	})

	mux.HandleFunc("GET /api/sources", func(w http.ResponseWriter, r *http.Request) {
        configLock.RLock()
		sendJSON(w, map[string]interface{}{"sources": Config.Sources})
        configLock.RUnlock()
	})

	mux.HandleFunc("POST /api/sources", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Sources map[string][]string `json:"sources"`
		}
		if err := json.NewDecoder(r.Body).Decode(&data); err == nil && data.Sources != nil {
            configLock.Lock()
			Config.Sources = data.Sources
            configLock.Unlock()
			detectXtream()
			saveConfig()
		}
		sendJSON(w, map[string]bool{"success": true})
	})

	mux.HandleFunc("DELETE /api/sources/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
        configLock.Lock()
		if _, exists := Config.Sources[id]; exists {
			delete(Config.Sources, id)
            configLock.Unlock()
			saveConfig()
		} else {
            configLock.Unlock()
        }
		sendJSON(w, map[string]bool{"success": true})
	})

	mux.HandleFunc("DELETE /api/streams/{key...}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		cleanupStream(key)
		sendJSON(w, map[string]bool{"success": true})
	})

	mux.HandleFunc("POST /api/fallback", func(w http.ResponseWriter, r *http.Request) {
        configLock.Lock()
		Config.FallbackMode = !Config.FallbackMode
        fm := Config.FallbackMode
        configLock.Unlock()
		saveConfig()
		log.Printf("Fallback mode: %v", fm)
		sendJSON(w, map[string]bool{"fallback_mode": fm})
	})

	mux.HandleFunc("POST /api/auto-fallback", func(w http.ResponseWriter, r *http.Request) {
        configLock.Lock()
		Config.AutoFallback = !Config.AutoFallback
        af := Config.AutoFallback
        configLock.Unlock()
		saveConfig()
		log.Printf("Auto-fallback: %v", af)
		sendJSON(w, map[string]bool{"auto_fallback": af})
	})

	mux.HandleFunc("POST /api/redirect", func(w http.ResponseWriter, r *http.Request) {
        configLock.Lock()
		Config.RedirectMode = !Config.RedirectMode
        rm := Config.RedirectMode
        configLock.Unlock()
		saveConfig()
		log.Printf("External Redirect mode: %v", rm)
		sendJSON(w, map[string]bool{"redirect_mode": rm})
	})

	mux.HandleFunc("POST /api/internal_m3u8", func(w http.ResponseWriter, r *http.Request) {
        configLock.Lock()
		Config.InternalM3u8 = !Config.InternalM3u8
        im := Config.InternalM3u8
        configLock.Unlock()
		saveConfig()
		log.Printf("Internal M3U8 mode: %v", im)
		sendJSON(w, map[string]bool{"internal_m3u8": im})
	})

	mux.HandleFunc("POST /api/engine", func(w http.ResponseWriter, r *http.Request) {
		var data map[string]string
		if err := json.NewDecoder(r.Body).Decode(&data); err == nil && data["engine"] != "" {
			configLock.Lock()
			Config.Engine = data["engine"]
			eng := Config.Engine
			configLock.Unlock()
			saveConfig()
			log.Printf("Engine set to: %s", eng)
			sendJSON(w, map[string]string{"engine": eng})
		} else {
			http.Error(w, "Bad request", 400)
		}
	})

	mux.HandleFunc("GET /api/xtream/providers", func(w http.ResponseWriter, r *http.Request) {
        configLock.RLock()
		sendJSON(w, map[string]interface{}{"providers": Config.XtreamProviders})
        configLock.RUnlock()
	})

	mux.HandleFunc("GET /api/xtream/providers/{id}/info", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
        configLock.RLock()
		provider, exists := Config.XtreamProviders[id]
        configLock.RUnlock()
		if !exists {
			http.Error(w, `{"error":"Not found"}`, 404)
			return
		}
		info := xtreamAPI(provider, "", nil)
		if info != nil {
			sendJSON(w, map[string]interface{}{"info": info})
		} else {
			sendJSON(w, map[string]interface{}{"error": "Failed"})
		}
	})

	mux.HandleFunc("GET /api/xtream/providers/{id}/categories", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
        configLock.RLock()
		provider, exists := Config.XtreamProviders[id]
        configLock.RUnlock()
		if !exists {
			http.Error(w, `{"error":"Not found"}`, 404)
			return
		}
		catType := r.URL.Query().Get("type")
		if catType == "" {
			catType = "live"
		}
		cats := getCategories(provider, catType)
        if cats == nil {
            cats = make([]map[string]interface{}, 0)
        }
		sendJSON(w, map[string]interface{}{"categories": cats})
	})

	mux.HandleFunc("GET /api/xtream/providers/{id}/streams", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
        configLock.RLock()
		provider, exists := Config.XtreamProviders[id]
        configLock.RUnlock()
		if !exists {
			http.Error(w, `{"error":"Not found"}`, 404)
			return
		}
		catID := r.URL.Query().Get("category_id")
		streamType := r.URL.Query().Get("type")
		if streamType == "" {
			streamType = "live"
		}
		streamsData := getStreams(provider, catID, streamType)
        if streamsData == nil {
            streamsData = make([]interface{}, 0)
        }
		sendJSON(w, map[string]interface{}{"streams": streamsData})
	})

	mux.HandleFunc("GET /api/xtream/providers/{id}/epg/{stream_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		streamID := r.PathValue("stream_id")
        configLock.RLock()
		provider, exists := Config.XtreamProviders[id]
        configLock.RUnlock()
		if !exists {
			http.Error(w, `{"error":"Not found"}`, 404)
			return
		}
		epg := getEpg(provider, streamID)
		sendJSON(w, map[string]interface{}{"epg": epg})
	})

	mux.HandleFunc("GET /api/xtream/providers/{id}/epg.xml", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		categoryID := r.URL.Query().Get("category_id")
		
		configLock.RLock()
		provider, exists := Config.XtreamProviders[id]
		configLock.RUnlock()
		
		if !exists {
			http.Error(w, "Provider not found", 404)
			return
		}

		filterEpg(w, provider, categoryID)
	})

	mux.HandleFunc("POST /api/xtream/test", func(w http.ResponseWriter, r *http.Request) {
		var data map[string]string
		if err := json.NewDecoder(r.Body).Decode(&data); err == nil {
			provider := Provider{
				URL:      data["url"],
				Username: data["username"],
				Password: data["password"],
			}
			res := xtreamAPI(provider, "", nil)
			if res != nil {
				sendJSON(w, map[string]interface{}{"success": true, "data": res})
				return
			}
		}
		sendJSON(w, map[string]interface{}{"error": "Failed"})
	})

	mux.HandleFunc("GET /api/xtream/providers/{id}/category/{category_id}/playlist.m3u", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		categoryID := r.PathValue("category_id")
        configLock.RLock()
		provider, exists := Config.XtreamProviders[id]
        configLock.RUnlock()
		if !exists {
			http.Error(w, `{"error":"Provider not found"}`, 404)
			return
		}

		if provider.SourceID == "" {
			http.Error(w, `{"error":"Provider has no source_id configured"}`, 500)
			return
		}

		streamsData := getStreams(provider, categoryID, "live")
		if streamsData == nil {
			http.Error(w, `{"error":"No streams found in this category"}`, 404)
			return
		}

		proxyHost := strings.Split(r.Host, ":")[0]
		if proxyHost == "" {
			proxyHost = "localhost"
		}

		var m3uLines []string
		m3uLines = append(m3uLines, "#EXTM3U")

		for _, item := range streamsData {
			if m, ok := item.(map[string]interface{}); ok {
				var streamID string
				if val, ok := m["stream_id"]; ok && val != nil {
					if f, isFloat := val.(float64); isFloat {
						streamID = fmt.Sprintf("%.0f", f)
					} else {
						streamID = fmt.Sprintf("%v", val)
					}
				} else if val, ok := m["id"]; ok && val != nil {
					if f, isFloat := val.(float64); isFloat {
						streamID = fmt.Sprintf("%.0f", f)
					} else {
						streamID = fmt.Sprintf("%v", val)
					}
				}
				
				streamName := fmt.Sprintf("%v", m["name"])
				epgID := fmt.Sprintf("%v", m["epg_channel_id"])
                if epgID == "<nil>" { epgID = "" }

				extinf := fmt.Sprintf(`#EXTINF:-1 tvg-id="%s" tvg-name="%s",%s`, epgID, streamName, streamName)
				m3uLines = append(m3uLines, extinf)

				proxyURL := fmt.Sprintf("http://%s:%d/%s/%s.ts", proxyHost, ProxyPort, provider.SourceID, streamID)
				m3uLines = append(m3uLines, proxyURL)
			}
		}

		m3uContent := strings.Join(m3uLines, "\n")
		w.Header().Set("Content-Type", "audio/mpegurl")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="playlist_%s.m3u"`, categoryID))
		w.Write([]byte(m3uContent))
	})

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		// QoL: Disable caching for index.html so updates are immediate
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		// 1. Try local file in src directory (allows manual UI overrides)
		uiFile := "src/index.html"
		if _, err := os.Stat(uiFile); err == nil {
			http.ServeFile(w, r, uiFile)
			return
		}
		
		// 2. Try embedded HTML (the new redesign)
		if len(defaultHTML) > 0 {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(defaultHTML)
			return
		}

		// 3. Fallback to basic JSON if everything else fails
		sendJSON(w, map[string]string{"status": "running", "version": "3.0 (Go Production)"})
	})
}