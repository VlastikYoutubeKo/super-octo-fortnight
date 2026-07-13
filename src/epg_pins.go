package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// Pinned EPG mappings: human decisions the janitor must never undo.
//
// sanitizeMappings deletes or re-points any mapping whose id carries words the
// channel name lacks (ExtraIDWords), on the theory that such an id names a
// different, more specific channel. That heuristic is right almost always, but it
// is wrong for channels that share one broadcast slot under a combined id — e.g.
// "CS FILM" airs CS Horror overnight and the only 24h guide for it is
// "CS.Film-CS.Horror.HD.sk". Mapping it there is correct and the janitor would
// still revert it every night.
//
// A pin is that override: channel name -> epg id, stored in epg_pins.json, applied
// at startup and at every maintenance run, and skipped by the sanitizer.
//
// A pin is NOT a licence to break invariant 2 (never map to an id TVHeadend has not
// loaded): a pin whose id is missing from the universe is reported and ignored, not
// written into the mapping.

var (
	EPGPins     = make(map[string]string)
	epgPinsLock sync.RWMutex
)

func epgPinsPath() string { return filepath.Join(scriptDir, "epg_pins.json") }

func loadEPGPins() {
	file, err := os.Open(epgPinsPath())
	if err != nil {
		return
	}
	defer file.Close()

	epgPinsLock.Lock()
	defer epgPinsLock.Unlock()
	if err := json.NewDecoder(file).Decode(&EPGPins); err != nil {
		log.Printf("Failed to parse epg_pins.json: %v", err)
		return
	}
	log.Printf("Loaded %d pinned EPG mappings", len(EPGPins))
}

func saveEPGPins() {
	file, err := os.Create(epgPinsPath())
	if err != nil {
		log.Printf("Failed to create epg pins file: %v", err)
		return
	}
	defer file.Close()

	epgPinsLock.RLock()
	defer epgPinsLock.RUnlock()
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	enc.Encode(EPGPins)
}

func pinsSnapshot() map[string]string {
	epgPinsLock.RLock()
	defer epgPinsLock.RUnlock()
	out := make(map[string]string, len(EPGPins))
	for k, v := range EPGPins {
		out[k] = v
	}
	return out
}

// applyPins forces every pinned channel back to its pinned id. Pins whose id is not
// loaded in TVHeadend are left unapplied and returned, so a stale pin surfaces in the
// maintenance report instead of silently poisoning the mapping.
func applyPins(inUniverse map[string]bool) (applied int, unloaded []string) {
	pins := pinsSnapshot()
	if len(pins) == 0 {
		return 0, nil
	}
	fix := map[string]string{}
	for ch, id := range pins {
		if id == "" {
			continue
		}
		if len(inUniverse) > 0 && !inUniverse[id] {
			unloaded = append(unloaded, ch)
			continue
		}
		epgMappingLock.RLock()
		cur, ok := EPGMapping[ch]
		epgMappingLock.RUnlock()
		if !ok || cur != id {
			fix[ch] = id
		}
	}
	if len(fix) == 0 {
		return 0, unloaded
	}
	epgMappingLock.Lock()
	for ch, id := range fix {
		EPGMapping[ch] = id
	}
	epgMappingLock.Unlock()
	saveEpgMapping()
	for ch, id := range fix {
		log.Printf("EPG pin applied: %q -> %s", ch, id)
	}
	return len(fix), unloaded
}

func setupPinRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/epg/pins", func(w http.ResponseWriter, r *http.Request) {
		sendJSON(w, pinsSnapshot())
	})

	mux.HandleFunc("PUT /api/epg/pin", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Channel string `json:"channel"`
			EpgID   string `json:"epg_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, 400)
			return
		}
		if data.Channel == "" || data.EpgID == "" {
			http.Error(w, `{"error":"channel and epg_id are required"}`, 400)
			return
		}
		epgPinsLock.Lock()
		EPGPins[data.Channel] = data.EpgID
		epgPinsLock.Unlock()
		saveEPGPins()

		epgMappingLock.Lock()
		EPGMapping[data.Channel] = data.EpgID
		epgMappingLock.Unlock()
		saveEpgMapping()

		log.Printf("EPG pin set: %q -> %s", data.Channel, data.EpgID)
		sendJSON(w, map[string]bool{"ok": true})
	})

	mux.HandleFunc("DELETE /api/epg/pin", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Channel string `json:"channel"`
		}
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, 400)
			return
		}
		if data.Channel == "" {
			http.Error(w, `{"error":"channel is required"}`, 400)
			return
		}
		epgPinsLock.Lock()
		delete(EPGPins, data.Channel)
		epgPinsLock.Unlock()
		saveEPGPins()
		sendJSON(w, map[string]bool{"ok": true})
	})
}
