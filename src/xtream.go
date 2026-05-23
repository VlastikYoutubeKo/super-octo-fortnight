package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"math/rand"
	"time"
)

var (
	xtreamClient = &http.Client{
		Timeout: 300 * time.Second,
	}
	
	// Map: SourceID:NormalizedName -> CurrentStreamID
	CurrentIDs      = make(map[string]string)
	currentIDsLock  sync.RWMutex
)

func refreshChannelNamesLoop() {
	loadNamesCache()
	for {
		// Try to refresh from any available provider
		configLock.RLock()
		providers := make([]Provider, 0)
		for _, p := range Config.XtreamProviders {
			providers = append(providers, p)
		}
		configLock.RUnlock()

		if len(providers) == 0 {
			time.Sleep(1 * time.Minute)
			continue
		}

		anySuccess := false
		for _, p := range providers {
			log.Printf("Refreshing channel names from provider: %s", p.Name)
			names, err := fetchLiveStreams(p)
			if err == nil && len(names) > 0 {
				channelNamesLock.Lock()
				currentIDsLock.Lock()
				for id, name := range names {
					key := fmt.Sprintf("%s:%s", p.SourceID, id)
					ChannelNames[key] = name
					
					norm := NormalizeChannelName(name)
					if norm != "" {
						mappingKey := fmt.Sprintf("%s:%s", p.SourceID, norm)
						CurrentIDs[mappingKey] = id
					}
				}
				currentIDsLock.Unlock()
				channelNamesLock.Unlock()
				log.Printf("Successfully cached %d channel names from %s", len(names), p.Name)
				anySuccess = true
				saveNamesCache() // Save immediately
			} else if err != nil {
				log.Printf("Failed to refresh names from %s: %v", p.Name, err)
			}
			time.Sleep(2 * time.Second)
		}

		if anySuccess {
			time.Sleep(1 * time.Hour) // Refresh every hour
		} else {
			time.Sleep(5 * time.Minute) // Retry sooner if all failed
		}
	}
}

func saveNamesCache() {
	channelNamesLock.RLock()
	currentIDsLock.RLock()
	defer currentIDsLock.RUnlock()
	defer channelNamesLock.RUnlock()

	data := map[string]interface{}{
		"names": ChannelNames,
		"ids":   CurrentIDs,
	}
	
	cacheFile := filepath.Join(scriptDir, "names_cache.json")
	file, err := os.Create(cacheFile)
	if err != nil {
		log.Printf("Failed to create names cache file: %v", err)
		return
	}
	defer file.Close()
	
	json.NewEncoder(file).Encode(data)
}

func loadNamesCache() {
	cacheFile := filepath.Join(scriptDir, "names_cache.json")
	file, err := os.Open(cacheFile)
	if err != nil {
		return
	}
	defer file.Close()

	var data struct {
		Names map[string]string `json:"names"`
		IDs   map[string]string `json:"ids"`
	}
	
	if err := json.NewDecoder(file).Decode(&data); err == nil {
		channelNamesLock.Lock()
		for k, v := range data.Names {
			ChannelNames[k] = v
		}
		channelNamesLock.Unlock()
		
		currentIDsLock.Lock()
		for k, v := range data.IDs {
			CurrentIDs[k] = v
		}
		currentIDsLock.Unlock()
		log.Printf("Loaded %d names and %d ID mappings from cache", len(data.Names), len(data.IDs))
	}
}


func fetchLiveStreams(p Provider) (map[string]string, error) {
	apiUrl := fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=get_live_streams", p.URL, p.Username, p.Password)
	
	req, _ := http.NewRequest("GET", apiUrl, nil)
	// Use the same reliable User-Agent as FFmpeg
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")

	resp, err := xtreamClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var streams []struct {
		ID   interface{} `json:"stream_id"`
		Name string      `json:"name"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&streams); err != nil {
		return nil, err
	}

	names := make(map[string]string)
	for _, s := range streams {
		idStr := fmt.Sprintf("%v", s.ID)
		names[idStr] = s.Name
	}
	return names, nil
}

func parseXtream(u string) (server, port, user, password string, ok bool) {
	re := regexp.MustCompile(`https?://([^:]+)(?::(\d+))?/(?:live|movie|series)/([^/]+)/([^/]+)/\{channel_id\}`)
	matches := re.FindStringSubmatch(u)
	if len(matches) == 5 {
		port := matches[2]
		if port == "" {
			if strings.HasPrefix(u, "https") {
				port = "443"
			} else {
				port = "80"
			}
		}
		return matches[1], port, matches[3], matches[4], true
	}
	return "", "", "", "", false
}

func detectXtream() {
	configLock.Lock()
	defer configLock.Unlock()

	providers := make(map[string]Provider)

	for sourceID, urls := range Config.Sources {
		for _, u := range urls {
			server, port, user, password, ok := parseXtream(u)
			if ok {
				providerID := fmt.Sprintf("%s_%s", server, user)
				if _, exists := providers[providerID]; !exists {
					providers[providerID] = Provider{
						Name:     server,
						URL:      fmt.Sprintf("http://%s:%s", server, port),
						Username: user,
						Password: password,
						SourceID: sourceID,
					}
				}
			}
		}
	}

	if len(providers) > 0 {
		Config.XtreamProviders = providers
		log.Printf("Detected %d Xtream providers", len(providers))
	}
}

func xtreamAPI(provider Provider, action string, params map[string]string) interface{} {
	apiURL := provider.URL + "/player_api.php"
	req, _ := http.NewRequest("GET", apiURL, nil)
	q := req.URL.Query()
	q.Add("username", provider.Username)
	q.Add("password", provider.Password)
	if action != "" {
		q.Add("action", action)
	}
	for k, v := range params {
		q.Add(k, v)
	}
	req.URL.RawQuery = q.Encode()

	proxies := getProxies()
	maxRetries := 3
	if len(proxies) == 0 {
		maxRetries = 1
	}

	for i := 0; i < maxRetries; i++ {
		client := &http.Client{Timeout: 10 * time.Second}
		if len(proxies) > 0 {
			proxyUrl, err := url.Parse(proxies[rand.Intn(len(proxies))])
			if err == nil {
				client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Xtream API error (attempt %d): %v", i+1, err)
			continue
		}
		
		if resp.StatusCode != 200 {
			resp.Body.Close()
			log.Printf("Xtream API returned status %d (attempt %d)", resp.StatusCode, i+1)
			continue
		}

		var result interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			log.Printf("Xtream API JSON decode error: %v", err)
			continue
		}
		resp.Body.Close()
		return result
	}

	return nil
}

func getCategories(provider Provider, catType string) []map[string]interface{} {
	actions := map[string]string{
		"live":   "get_live_categories",
		"vod":    "get_vod_categories",
		"series": "get_series_categories",
	}
	action := actions[catType]
	if action == "" {
		return nil
	}
	res := xtreamAPI(provider, action, nil)
	
	var result []map[string]interface{}
	if arr, ok := res.([]interface{}); ok {
		for _, item := range arr {
			if m, ok := item.(map[string]interface{}); ok {
				id := fmt.Sprintf("%v", m["category_id"])
				name := fmt.Sprintf("%v", m["category_name"])
				result = append(result, map[string]interface{}{
					"id":   id,
					"name": name,
				})
			}
		}
	}
	return result
}

func getStreams(provider Provider, catID string, streamType string) []interface{} {
	actions := map[string]string{
		"live":   "get_live_streams",
		"vod":    "get_vod_streams",
		"series": "get_series",
	}
	action := actions[streamType]
	if action == "" {
		return nil
	}
	params := make(map[string]string)
	if catID != "" {
		params["category_id"] = catID
	}
	
	res := xtreamAPI(provider, action, params)
	if arr, ok := res.([]interface{}); ok {
		return arr
	}
	return nil
}

func getEpg(provider Provider, streamID string) interface{} {
	params := map[string]string{
		"stream_id": streamID,
		"limit":     "10",
	}
	res := xtreamAPI(provider, "get_short_epg", params)
	if res != nil {
		return res
	}
	return map[string]interface{}{}
}

func filterEpg(w http.ResponseWriter, provider Provider, categoryID string) {
	// 1. Get channel IDs for this category
	streams := getStreams(provider, categoryID, "live")
	allowedIDs := make(map[string]bool)
	for _, s := range streams {
		if m, ok := s.(map[string]interface{}); ok {
			if epgID, ok := m["epg_channel_id"].(string); ok && epgID != "" {
				allowedIDs[epgID] = true
			}
		}
	}

	if len(allowedIDs) == 0 && categoryID != "" {
		log.Printf("No EPG IDs found for category %s", categoryID)
		http.Error(w, "No channels with EPG found in this category", 404)
		return
	}

	// 2. Fetch full XMLTV
	epgURL := provider.URL + "/xmltv.php"
	req, _ := http.NewRequest("GET", epgURL, nil)
	q := req.URL.Query()
	q.Add("username", provider.Username)
	q.Add("password", provider.Password)
	req.URL.RawQuery = q.Encode()

	proxies := getProxies()
	maxRetries := 3
	if len(proxies) == 0 {
		maxRetries = 1
	}

	var resp *http.Response
	var err error

	for i := 0; i < maxRetries; i++ {
		client := &http.Client{Timeout: 60 * time.Second}
		if len(proxies) > 0 {
			proxyUrl, parseErr := url.Parse(proxies[rand.Intn(len(proxies))])
			if parseErr == nil {
				client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
			}
		}

		resp, err = client.Do(req)
		if err != nil {
			log.Printf("EPG fetch error (attempt %d): %v", i+1, err)
			continue
		}
		
		if resp.StatusCode != 200 {
			resp.Body.Close()
			log.Printf("EPG fetch returned status %d (attempt %d)", resp.StatusCode, i+1)
			continue
		}
		break // Success
	}

	if resp == nil || resp.StatusCode != 200 {
		http.Error(w, "Failed to fetch EPG from provider", 502)
		return
	}
	defer resp.Body.Close()

	// 3. Filter and stream back
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(200)

	// XMLTV is huge, so we use a simple scanner to filter line by line if possible,
	// but <channel> and <programme> tags can span multiple lines.
	// For now, let's use a more robust approach:
	// We'll output the header, then filter the elements.
	
	fmt.Fprintf(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	fmt.Fprintf(w, "<!DOCTYPE tv SYSTEM \"xmltv.dtd\">\n")
	fmt.Fprintf(w, "<tv>\n")

	decoder := xml.NewDecoder(resp.Body)
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch se := token.(type) {
		case xml.StartElement:
			if se.Name.Local == "channel" {
				var ch struct {
					ID string `xml:"id,attr"`
					Inner []byte `xml:",innerxml"`
				}
				if err := decoder.DecodeElement(&ch, &se); err == nil {
					if allowedIDs[ch.ID] || categoryID == "" {
						fmt.Fprintf(w, "  <channel id=\"%s\">%s</channel>\n", ch.ID, string(ch.Inner))
					}
				}
			} else if se.Name.Local == "programme" {
				var pr struct {
					Channel string `xml:"channel,attr"`
					Inner []byte `xml:",innerxml"`
					Start string `xml:"start,attr"`
					Stop string `xml:"stop,attr"`
				}
				if err := decoder.DecodeElement(&pr, &se); err == nil {
					if allowedIDs[pr.Channel] || categoryID == "" {
						fmt.Fprintf(w, "  <programme start=\"%s\" stop=\"%s\" channel=\"%s\">%s</programme>\n", 
							pr.Start, pr.Stop, pr.Channel, string(pr.Inner))
					}
				}
			}
		}
	}

	fmt.Fprintf(w, "</tv>\n")
}