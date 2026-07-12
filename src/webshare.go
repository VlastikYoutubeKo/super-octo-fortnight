package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type WebshareStatResponse struct {
	BandwidthTotal int64 `json:"bandwidth_total"`
	IsProjected    bool  `json:"is_projected"`
}

func monitorWebshareStats() {
	for {
		configLock.RLock()
		apiKey := Config.WebshareAPIKey
		configLock.RUnlock()

		if apiKey != "" {
			end := time.Now().UTC()
			start := end.AddDate(0, 0, -30)

			url := "https://proxy.webshare.io/api/v2/stats/?timestamp__gte=" + start.Format(time.RFC3339) + "&timestamp__lte=" + end.Format(time.RFC3339)

			req, err := http.NewRequest("GET", url, nil)
			if err == nil {
				req.Header.Set("Authorization", "Token "+apiKey)

				client := &http.Client{Timeout: 10 * time.Second}
				resp, err := client.Do(req)
				if err == nil && resp.StatusCode == 200 {
					var stats []WebshareStatResponse
					if err := json.NewDecoder(resp.Body).Decode(&stats); err == nil {
						var totalBytes int64 = 0
						for _, s := range stats {
							if !s.IsProjected {
								totalBytes += s.BandwidthTotal
							}
						}

						usedGB := float64(totalBytes) / (1024 * 1024 * 1024)

						wsStatsLock.Lock()
						wsStats.UsedGB = usedGB
						wsStats.TotalGB = 1024.0 // Assuming 1TB limit
						wsStats.LastUpdate = time.Now().Unix()
						wsStatsLock.Unlock()
					}
					resp.Body.Close()
				} else if err != nil {
					log.Printf("Webshare API error: %v", err)
				} else {
					resp.Body.Close()
				}
			}
		}

		time.Sleep(30 * time.Minute)
	}
}
