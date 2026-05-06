package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type SearchResult struct {
	Type string `json:"type"`
	Data struct {
		StreamURL string `json:"stream_url"`
		Name      string `json:"stream_name"`
	} `json:"data"`
}

var (
	globalSearchCache = make(map[string][]string)
	searchCacheLock   sync.RWMutex
)

func searchGlobalAlternatives(channelName string) []string {
	if channelName == "" {
		return nil
	}

	searchCacheLock.RLock()
	if cached, ok := globalSearchCache[channelName]; ok {
		searchCacheLock.RUnlock()
		return cached
	}
	searchCacheLock.RUnlock()

	// Clean name for better search (remove common quality markers and symbols)
	cleanName := channelName
	
	// Remove country prefixes like "PL|", "CZ -", "UK:"
	rePrefix := regexp.MustCompile(`(?i)^[a-z]{2,3}\s*[:|-]\s*`)
	cleanName = rePrefix.ReplaceAllString(cleanName, "")
	
	// Remove tags in brackets/parentheses like [720p], (FHD), [CZ]
	reTags := regexp.MustCompile(`(?i)\[.*?\]|\(.*?\)|<.*?>`)
	cleanName = reTags.ReplaceAllString(cleanName, "")
	
	// Remove quality and resolution markers that might not be in brackets
	reQuality := regexp.MustCompile(`(?i)\b(FHD|HD|SD|4K|UHD|1080p|720p|480p|1080|720|HEVC)\b`)
	cleanName = reQuality.ReplaceAllString(cleanName, "")
	
	// Remove remaining extra characters and replace with space
	reChars := regexp.MustCompile(`[._|:,\-]`)
	cleanName = reChars.ReplaceAllString(cleanName, " ")
	
	// Collapse multiple spaces
	cleanName = strings.Join(strings.Fields(cleanName), " ")
	
	// If the name is too short after cleaning, use the original
	if len(cleanName) < 2 {
		cleanName = channelName
	}

	searchUrl := fmt.Sprintf("http://iptv.tutoje.cz/global-search.php?action=stream_search&q=%s&type=live", url.QueryEscape(cleanName))
	
	log.Printf("Searching global alternatives for: %s (query: %s)", channelName, cleanName)

	// Use a longer timeout for global search as it needs to scan many providers
	searchClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := searchClient.Get(searchUrl)
	if err != nil {
		log.Printf("Global search request failed: %v", err)
		return nil
	}
	defer resp.Body.Close()

	var results []string
	scanner := bufio.NewScanner(resp.Body)
	
	// API uses NDJSON (Newline Delimited JSON)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 { continue }

		var msg SearchResult
		if err := json.Unmarshal(line, &msg); err == nil {
			if msg.Type == "result" && msg.Data.StreamURL != "" {
				results = append(results, msg.Data.StreamURL)
			}
		}
	}

	searchCacheLock.Lock()
	globalSearchCache[channelName] = results
	searchCacheLock.Unlock()

	log.Printf("Found %d global alternatives for %s", len(results), channelName)
	return results
}
