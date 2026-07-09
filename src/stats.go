package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ChannelStat tracks viewing statistics for a single channel/stream.
type ChannelStat struct {
	Name         string    `json:"name"`
	Views        int       `json:"views"`
	TotalSeconds int64     `json:"total_seconds"`
	LastWatched  time.Time `json:"last_watched"`
}

var (
	channelStats     = make(map[string]*ChannelStat)
	channelStatsLock sync.RWMutex

	statsSavePending bool
	statsSaveMu      sync.Mutex
)

// InitStats loads persisted stats from disk. Call once at startup.
func InitStats() {
	loadStats()
}

// RecordStreamStart increments the view count and updates LastWatched for the given key.
func RecordStreamStart(key string) {
	channelStatsLock.Lock()
	defer channelStatsLock.Unlock()

	stat, exists := channelStats[key]
	if !exists {
		stat = &ChannelStat{
			Name: resolveChannelName(key),
		}
		channelStats[key] = stat
	}
	stat.Views++
	stat.LastWatched = time.Now()

	debouncedSaveStats()
}

// RecordStreamEnd adds the given duration to the channel's total watched time.
func RecordStreamEnd(key string, durationSeconds int64) {
	if durationSeconds <= 0 {
		return
	}

	channelStatsLock.Lock()
	defer channelStatsLock.Unlock()

	stat, exists := channelStats[key]
	if !exists {
		stat = &ChannelStat{
			Name: resolveChannelName(key),
		}
		channelStats[key] = stat
	}
	stat.TotalSeconds += durationSeconds

	debouncedSaveStats()
}

// GetChannelStats returns a copy of all stats sorted by Views descending.
func GetChannelStats() []ChannelStat {
	channelStatsLock.RLock()
	defer channelStatsLock.RUnlock()

	result := make([]ChannelStat, 0, len(channelStats))
	for _, stat := range channelStats {
		result = append(result, *stat)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Views > result[j].Views
	})

	return result
}

// ClearStats removes all channel statistics and persists the empty state.
func ClearStats() {
	channelStatsLock.Lock()
	channelStats = make(map[string]*ChannelStat)
	channelStatsLock.Unlock()

	saveStats()
}

// resolveChannelName looks up a friendly name from the ChannelNames map.
// key format is typically "providerSourceID/streamID".
func resolveChannelName(key string) string {
	channelNamesLock.RLock()
	name, exists := ChannelNames[key]
	channelNamesLock.RUnlock()
	if exists && name != "" {
		return name
	}
	return key
}

// statsFilePath returns the path to the stats JSON file.
func statsFilePath() string {
	return filepath.Join(scriptDir, "channel_stats.json")
}

// loadStats reads channel stats from disk.
func loadStats() {
	data, err := os.ReadFile(statsFilePath())
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Failed to read channel stats: %v", err)
		}
		return
	}

	channelStatsLock.Lock()
	defer channelStatsLock.Unlock()

	if err := json.Unmarshal(data, &channelStats); err != nil {
		log.Printf("Failed to decode channel stats: %v", err)
		return
	}
	log.Printf("Loaded %d channel stats", len(channelStats))
}

// saveStats writes channel stats to disk immediately.
func saveStats() {
	channelStatsLock.RLock()
	data, err := json.MarshalIndent(channelStats, "", "  ")
	channelStatsLock.RUnlock()

	if err != nil {
		log.Printf("Failed to marshal channel stats: %v", err)
		return
	}

	if err := os.WriteFile(statsFilePath(), data, 0644); err != nil {
		log.Printf("Failed to save channel stats: %v", err)
	}
}

// debouncedSaveStats ensures stats are saved at most once every 30 seconds.
// Must be called while channelStatsLock is held (it spawns a goroutine that waits).
func debouncedSaveStats() {
	statsSaveMu.Lock()
	defer statsSaveMu.Unlock()

	if statsSavePending {
		return
	}
	statsSavePending = true

	go func() {
		time.Sleep(30 * time.Second)
		statsSaveMu.Lock()
		statsSavePending = false
		statsSaveMu.Unlock()

		saveStats()
	}()
}
