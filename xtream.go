package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
    "math/rand"
    "time"
)

func parseXtream(u string) (server, port, user, password string, ok bool) {
	re := regexp.MustCompile(`https?://([^:]+):(\d+)/(?:live|movie|series)/([^/]+)/([^/]+)/\{channel_id\}`)
	matches := re.FindStringSubmatch(u)
	if len(matches) == 5 {
		return matches[1], matches[2], matches[3], matches[4], true
	}
	return "", "", "", "", false
}

func detectXtream() {
	configLock.Lock()
	defer configLock.Unlock()

	providers := make(map[string]Provider)
	for k, v := range Config.XtreamProviders {
		providers[k] = v // Keep existing
	}

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