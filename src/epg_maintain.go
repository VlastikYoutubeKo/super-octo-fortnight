package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Daily EPG maintenance. The auto-EPG (AI + fuzzy) keeps adding mappings as new
// channels appear, and some of them are wrong in ways only visible against the EPG
// TVHeadend actually has loaded. This janitor runs once a day (and on demand via
// POST /api/epg/maintain) and does three things:
//
//  1. sanitize epg_mapping.json — delete/re-point mappings whose id is not loaded
//     ("dangling"), names a different more-specific channel (extra id words), or
//     belongs to a PPV/VOD/fixture listing;
//  2. relink TVHeadend channels — set each channel's EPG source to the tvg-id our
//     playlists emit (TVHeadend never re-links existing channels on its own);
//  3. clear garbage links — name-matched EPG on channels that should have none.
//
// Channels the janitor touches get epgauto=false so TVHeadend's name-matching
// cannot re-poison them.

const maintenanceHour = 5 // run daily at ~05:15 local, after the 04:00 epg.py refresh
const maintenanceMin = 15

var maintainMu sync.Mutex

// ---- TVHeadend API helpers ----

func tvhRequest(method, path string, body string) ([]byte, error) {
	configLock.RLock()
	tvh := Config.TVHeadend
	configLock.RUnlock()
	if tvh.URL == "" {
		return nil, fmt.Errorf("TVHeadend URL not configured")
	}
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	req, err := http.NewRequest(method, strings.TrimRight(tvh.URL, "/")+path, reader)
	if err != nil {
		return nil, err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if tvh.Username != "" {
		req.SetBasicAuth(tvh.Username, tvh.Password)
	}
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("TVHeadend %s returned %d", path, resp.StatusCode)
	}
	var buf strings.Builder
	if _, err := copyToBuilder(&buf, resp); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

func copyToBuilder(b *strings.Builder, resp *http.Response) (int64, error) {
	chunk := make([]byte, 1<<16)
	var n int64
	for {
		k, err := resp.Body.Read(chunk)
		if k > 0 {
			b.Write(chunk[:k])
			n += int64(k)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return n, nil
			}
			return n, err
		}
	}
}

func tvhGrid(path string, out interface{}) error {
	data, err := tvhRequest("GET", path, "")
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// ---- step 1: sanitize epg_mapping.json against the loaded EPG ----

type maintStats struct {
	MappingTotal    int      `json:"mapping_total"`
	MappingDeleted  int      `json:"mapping_deleted"`
	MappingRepoint  int      `json:"mapping_repointed"`
	PinsApplied     int      `json:"pins_applied"`
	PinsUnloaded    []string `json:"pins_unloaded,omitempty"`
	ChannelsTotal   int      `json:"tvh_channels"`
	Relinked        int      `json:"tvh_relinked"`
	Cleared         int      `json:"tvh_cleared"`
	Reimported      bool     `json:"tvh_reimported"`
	UniverseSize    int      `json:"epg_universe"`
	DurationSeconds int      `json:"duration_seconds"`
}

// planSanitize decides, for every mapping, whether to keep, re-point or delete it.
// Pinned channels are human decisions and are always kept as-is (see epg_pins.go).
// Pure so the rules can be tested without TVHeadend or the mapping file.
func planSanitize(snapshot, pins map[string]string, inUniverse map[string]bool, byCountry map[string][]string) (repoint map[string]string, del []string) {
	repoint = map[string]string{}
	for ch, id := range snapshot {
		if id == "" {
			continue
		}
		if _, pinned := pins[ch]; pinned {
			continue
		}
		if IsNonChannelName(ch) {
			del = append(del, ch)
			continue
		}
		bad := !inUniverse[id] || len(ExtraIDWords(ch, id)) > 0
		if !bad {
			continue
		}
		if best := CleanReplacement(ch, byCountry, inUniverse); best != "" && best != id {
			repoint[ch] = best
		} else if best == "" {
			del = append(del, ch)
		}
	}
	return repoint, del
}

func sanitizeMappings(inUniverse map[string]bool, byCountry map[string][]string, st *maintStats) {
	epgMappingLock.RLock()
	snapshot := make(map[string]string, len(EPGMapping))
	for k, v := range EPGMapping {
		snapshot[k] = v
	}
	epgMappingLock.RUnlock()
	st.MappingTotal = len(snapshot)

	repoint, del := planSanitize(snapshot, pinsSnapshot(), inUniverse, byCountry)

	if len(repoint) == 0 && len(del) == 0 {
		return
	}
	// keep one rolling backup per day before touching the file
	if data, err := json.Marshal(snapshot); err == nil {
		_ = writeBackup("epg_mapping.json.pre-maintenance", data)
	}
	epgMappingLock.Lock()
	for ch, id := range repoint {
		EPGMapping[ch] = id
	}
	for _, ch := range del {
		delete(EPGMapping, ch)
	}
	epgMappingLock.Unlock()
	saveEpgMapping()
	st.MappingRepoint = len(repoint)
	st.MappingDeleted = len(del)
	for ch, id := range repoint {
		log.Printf("EPG maintenance: re-pointed %q -> %s", ch, id)
	}
}

// ---- step 2+3: force-correct TVHeadend channel links ----

var reTvgID = regexp.MustCompile(`tvg-id="([^"]*)"`)

// fetchOwnPlaylist rewrites a network URL that points at this proxy to localhost and
// returns displayName -> tvg-id. Non-proxy playlists are fetched as-is.
func fetchOwnPlaylist(rawURL string) map[string]string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	if strings.Contains(u.Path, "/api/playlist/") {
		u.Scheme = "http"
		u.Host = fmt.Sprintf("127.0.0.1:%d", APIPort)
	}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "epg-maintenance")
	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var buf strings.Builder
	if _, err := copyToBuilder(&buf, resp); err != nil {
		return nil
	}
	out := map[string]string{}
	for _, line := range strings.Split(buf.String(), "\n") {
		if !strings.HasPrefix(line, "#EXTINF") {
			continue
		}
		comma := strings.Index(line, ",")
		if comma < 0 {
			continue
		}
		name := strings.TrimSpace(line[comma+1:])
		id := ""
		if m := reTvgID.FindStringSubmatch(line); m != nil {
			id = m[1]
		}
		out[name] = id
	}
	return out
}

// linkCoverage scores how well an epg id fits a channel name (for keep-vs-replace).
func linkCoverage(name, id string) int {
	if id == "" {
		return -1
	}
	cw, _ := tokenize(CleanChannelNameForMatching(name))
	_, idJ := tokenize(id)
	if c := EPGIDCountry(id); c != "" {
		idJ = strings.TrimSuffix(idJ, c)
	}
	n := 0
	hasWords := false
	for _, w := range cw {
		if len(w) < 3 || genericWords[w] || isNumeric(w) || qualityWords[w] {
			continue
		}
		hasWords = true
		if strings.Contains(idJ, w) {
			n++
		}
	}
	if !hasWords {
		return 0
	}
	return n
}

func relinkTVHChannels(st *maintStats) error {
	var nets struct {
		Entries []struct {
			UUID string `json:"uuid"`
			URL  string `json:"url"`
		} `json:"entries"`
	}
	if err := tvhGrid("/api/mpegts/network/grid?limit=100", &nets); err != nil {
		return err
	}
	want := map[string]map[string]string{}
	for _, n := range nets.Entries {
		if n.URL == "" {
			continue
		}
		if pl := fetchOwnPlaylist(n.URL); pl != nil {
			want[n.UUID] = pl
		}
	}
	if len(want) == 0 {
		return fmt.Errorf("no network playlists fetched")
	}

	var muxes struct {
		Entries []struct {
			UUID    string `json:"uuid"`
			Network string `json:"network_uuid"`
			SName   string `json:"iptv_sname"`
		} `json:"entries"`
	}
	if err := tvhGrid("/api/mpegts/mux/grid?limit=10000", &muxes); err != nil {
		return err
	}
	muxByUUID := map[string]struct{ net, sname string }{}
	for _, m := range muxes.Entries {
		muxByUUID[m.UUID] = struct{ net, sname string }{m.Network, m.SName}
	}

	var svcs struct {
		Entries []struct {
			UUID string `json:"uuid"`
			Mux  string `json:"multiplex_uuid"`
		} `json:"entries"`
	}
	if err := tvhGrid("/api/mpegts/service/grid?limit=20000", &svcs); err != nil {
		return err
	}
	svcMux := map[string]string{}
	for _, s := range svcs.Entries {
		svcMux[s.UUID] = s.Mux
	}

	var eg struct {
		Entries []struct {
			UUID string `json:"uuid"`
			ID   string `json:"id"`
		} `json:"entries"`
	}
	if err := tvhGrid("/api/epggrab/channel/grid?limit=100000", &eg); err != nil {
		return err
	}
	id2uuid := map[string]string{}
	uuid2id := map[string]string{}
	for _, e := range eg.Entries {
		if _, ok := id2uuid[e.ID]; !ok {
			id2uuid[e.ID] = e.UUID
		}
		uuid2id[e.UUID] = e.ID
	}

	var chans struct {
		Entries []struct {
			UUID     string   `json:"uuid"`
			Name     string   `json:"name"`
			EPGGrab  []string `json:"epggrab"`
			Services []string `json:"services"`
		} `json:"entries"`
	}
	if err := tvhGrid("/api/channel/grid?limit=100000&all=1", &chans); err != nil {
		return err
	}
	st.ChannelsTotal = len(chans.Entries)

	type node struct {
		UUID    string   `json:"uuid"`
		EPGGrab []string `json:"epggrab"`
		EPGAuto bool     `json:"epgauto"`
	}
	var actions []node
	for _, ch := range chans.Entries {
		var netUUID, sname string
		for _, su := range ch.Services {
			mu, ok := muxByUUID[svcMux[su]]
			if !ok {
				continue
			}
			if _, isOurs := want[mu.net]; isOurs {
				netUUID, sname = mu.net, mu.sname
				break
			}
		}
		expected := ""
		if pl, ok := want[netUUID]; ok {
			expected = pl[sname]
			if expected == "" {
				expected = pl[ch.Name]
			}
		}
		links := make([]string, 0, len(ch.EPGGrab))
		for _, u := range ch.EPGGrab {
			links = append(links, uuid2id[u])
		}

		if expected != "" {
			expUUID := id2uuid[expected]
			if expUUID == "" || containsStr(links, expected) {
				continue
			}
			bestCur := -1
			for _, l := range links {
				if c := linkCoverage(ch.Name, l); c > bestCur {
					bestCur = c
				}
			}
			// tie goes to the curated playlist id; strictly-better current links stay
			if bestCur <= linkCoverage(ch.Name, expected) {
				actions = append(actions, node{ch.UUID, []string{expUUID}, false})
				st.Relinked++
			}
		} else if len(links) > 0 {
			bestCur := -1
			for _, l := range links {
				if c := linkCoverage(ch.Name, l); c > bestCur {
					bestCur = c
				}
			}
			if bestCur == 0 { // has distinguishing words, none matched: garbage link
				actions = append(actions, node{ch.UUID, []string{}, false})
				st.Cleared++
			}
		}
	}

	for i := 0; i < len(actions); i += 100 {
		end := i + 100
		if end > len(actions) {
			end = len(actions)
		}
		payload, _ := json.Marshal(actions[i:end])
		if _, err := tvhRequest("POST", "/api/idnode/save", "node="+url.QueryEscape(string(payload))); err != nil {
			return err
		}
	}
	return nil
}

// triggerEPGReimport re-runs TVHeadend's internal (cron) grabbers.
//
// Relinking a channel to a different EPG source does NOT move any events: TVHeadend
// attaches broadcasts to channels at import time, so until a grabber re-reads the
// XMLTV the channel keeps showing the old source's guide (or nothing). Its cron is
// "4 */12 * * *", i.e. a relink made at 05:15 would otherwise stay invisible until
// 12:04. Whenever we change a link, we force the re-import.
func triggerEPGReimport() error {
	_, err := tvhRequest("POST", "/api/epggrab/internal/rerun", "rerun=1")
	return err
}

func writeBackup(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ---- orchestration ----

func RunEPGMaintenance() (maintStats, error) {
	maintainMu.Lock()
	defer maintainMu.Unlock()
	start := time.Now()
	var st maintStats

	// the resolvable universe = the EPG channels TVHeadend actually has
	universe, err := FetchTVHeadendEPGChannels()
	if err != nil {
		return st, fmt.Errorf("fetch TVH EPG channels: %v", err)
	}
	inUniverse := make(map[string]bool, len(universe))
	byCountry := map[string][]string{}
	for id := range universe {
		inUniverse[id] = true
		if c := EPGIDCountry(id); c != "" {
			byCountry[c] = append(byCountry[c], id)
		}
	}
	st.UniverseSize = len(inUniverse)

	// Safety rail: a suspiciously small universe means the grid fetch is broken or
	// TVHeadend is mid-restart. Deleting mappings against it would gut the file.
	if st.UniverseSize < 5000 {
		return st, fmt.Errorf("EPG universe suspiciously small (%d); refusing to sanitize", st.UniverseSize)
	}

	// re-assert human overrides first, so the sanitizer sees the pinned ids in place
	st.PinsApplied, st.PinsUnloaded = applyPins(inUniverse)
	for _, ch := range st.PinsUnloaded {
		log.Printf("EPG pin NOT applied (id not loaded in TVHeadend): %q", ch)
	}

	sanitizeMappings(inUniverse, byCountry, &st)

	if err := relinkTVHChannels(&st); err != nil {
		return st, fmt.Errorf("relink: %v", err)
	}

	// a changed link only reaches the guide after a grabber re-import
	if st.Relinked+st.Cleared > 0 {
		if err := triggerEPGReimport(); err != nil {
			log.Printf("EPG maintenance: re-import trigger failed: %v", err)
		} else {
			st.Reimported = true
		}
	}

	st.DurationSeconds = int(time.Since(start).Seconds())
	log.Printf("EPG maintenance done: universe=%d mappings=%d pinned=%d repointed=%d deleted=%d channels=%d relinked=%d cleared=%d (%ds)",
		st.UniverseSize, st.MappingTotal, st.PinsApplied, st.MappingRepoint, st.MappingDeleted,
		st.ChannelsTotal, st.Relinked, st.Cleared, st.DurationSeconds)
	return st, nil
}

func setupMaintainRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/epg/maintain", func(w http.ResponseWriter, r *http.Request) {
		st, err := RunEPGMaintenance()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), 500)
			return
		}
		sendJSON(w, st)
	})
}

// epgMaintenanceLoop runs maintenance daily at maintenanceHour:maintenanceMin local
// time — one hour after the 04:00 epg.py refresh, so it audits fresh data.
func epgMaintenanceLoop() {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), maintenanceHour, maintenanceMin, 0, 0, now.Location())
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		time.Sleep(time.Until(next))
		if _, err := RunEPGMaintenance(); err != nil {
			log.Printf("EPG maintenance failed: %v", err)
		}
	}
}
