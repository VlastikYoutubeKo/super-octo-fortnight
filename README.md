# IPTV Stream Proxy Manager

A production-ready, ultra-low latency IPTV proxy system acting as an intelligent middleman between TVHeadend/VLC and upstream Xtream Codes providers. Completely rewritten in Go for extreme concurrency, eliminating Python's GIL limitations, and packed with an AI-driven smart EPG mapping system.

## Key Features

- **Blazing Fast Concurrency:** Fully leverages Go routines to handle hundreds of concurrent streams without bottlenecks.
- **AI-Powered EPG Matching:** Uses Gemini 3.5-flash AI to automatically assign XMLTV EPG IDs to raw IPTV stream names (e.g. mapping `PL-[EXTRA] TVN 7` to `TVN.7.pl`).
- **Resilient Proxy Fallbacks:** Rotates HTTP requests through Webshare proxies, instantly falling back to a direct connection if proxies are blocked or rate-limited.
- **Smart Metadata Handling:** Channel names are beautifully sanitized for M3U playlists, stripping away ugly provider tags (`[1080p]`, `[HEVC]`, etc.).
- **Live Connection Hijacking:** Writes raw MPEG-TS video directly onto TCP sockets to prevent chunks from corrupting playback in TVHeadend.
- **Multi-API Key Rotation:** Supports multiple Gemini API keys to bypass rate limits (`429 Quota Exhausted`) on the free tier.

---

## 🛠 Quick Start

### Build from source
```bash
cd src
go build -o ../proxy_server *.go
```

### Run
```bash
./proxy_server
```

## 📡 Ports and Access

- **Proxy Stream Video:** `http://localhost:9000`
- **REST API & Web UI:** `http://localhost:9005`

---

## 📜 Recent Changelog

### v1.5.0 - The Smart EPG Update
- **Feature:** Introduced AI M3U Bulk Playlist generation in the UI.
- **Feature:** Gemini 3.5-flash AI integration for automatic XMLTV EPG ID matching.
- **Feature:** Added scraping support for bulk EPG text files and directories from epgshare01.
- **Feature:** Multi-API key rotation/fallback added to bypass Gemini free-tier rate limits.
- **Feature:** Added an M3U Name Sanitizer that cleans ugly prefixes and tags from generated playlists.
- **Enhancement:** UI categories are now sorted alphabetically for clean provider groupings.
- **Enhancement:** Added auto-correction and fuzzy-matching logic to prevent Gemini AI from hallucinating fake EPG IDs (e.g. auto-correcting `TVN7.pl` to `TVN.7.pl`).
- **Fix:** Redesigned the Xtream API fetcher to disable Webshare proxies for lightweight metadata calls, dropping 16-second timeouts down to instant direct connections.
- **Fix:** Optimized the channel refresh background loop to group by `SourceID`, avoiding redundant fetching.
- **Chore:** Root `index.html` updated with a unified dark mode design.

### v1.4.x - The Core Proxy Polish
- **Fix:** Completely aligned PCR and DTS in FFmpeg engine to fix TVHeadend packet drops.
- **Fix:** Removed HTTP chunked transfer boundaries, sending raw MPEG-TS over raw TCP via `http.Hijacker`.
- **Feature:** Added rapid failover functionality to blast through dead streams within 1s.
- **Feature:** Added internal fallback video when all providers are offline.