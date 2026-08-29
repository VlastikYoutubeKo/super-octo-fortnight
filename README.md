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

## 🔐 API Security

The management API (`:9005`) used to be completely open — anyone who could reach the port could read your config (upstream credentials, API keys) and control the proxy. Three optional hardening knobs are now available in `config.json` (and in the **Settings** tab of the Web UI):

- **`api_token`** — when set, every `/api/*` request from another machine must present it via `X-API-Token` header, `Authorization: Bearer <token>`, or `?token=<token>` query param (handy for TVHeadend playlist URLs). Requests from localhost stay open (the UI served on the same host, the EPG janitor, TVHeadend running on the same box). The UI asks for the token once and stores it in `localStorage`.
- **`cors_origin`** — restrict which website may call the API from a browser. Leave empty for the backwards-compatible `*`.
- **`trust_proxy_headers`** — `false` by default. When `false`, the IP allowlist (`allowed_ips`/`allowed_domains`) uses the real socket peer, so remote clients **cannot spoof** `X-Forwarded-For: 127.0.0.1` to bypass it. Set to `true` only when the proxy sits behind a reverse proxy (nginx etc.) that sets those headers.

Additional hardening included:

- `/api/status` now **redacts upstream credentials** (`http://user:pass@host/...` → `http://user:***@host/...`).
- All JSON files (`config.json`, `epg_mapping.json`, `epg_pins.json`, `names_cache.json`, `channel_stats.json`) are now written **atomically** (temp file + rename), so a crash mid-write can never corrupt them.
- New `GET /api/health` endpoint (`status: ok`, uptime, version) for monitoring.

## 🛡️ Stream Proxies & Webshare API

To hide your server's IP address from upstream IPTV providers, you can enable **Stream Proxies**.

### How to use Stream Proxies (Optional)
1. Add your HTTP proxy URLs (e.g., `http://user:pass@host:port`) into the `"proxies"` array inside your `config.json`.
2. Open the Web UI on `http://localhost:9005`.
3. Click the **Stream Proxies** button to toggle it `ON`.
4. The proxy server will instantly start randomly routing every new channel stream request through one of your defined proxies.

### Webshare API Bandwidth Monitoring
If you use [Webshare](https://webshare.io) proxies and have a limited bandwidth plan (e.g., 1TB/month):
- Go to **Settings** in the Web UI.
- Paste your Webshare API Token into the `Webshare API Key` field.
- The Dashboard will automatically display a **Webshare Proxy Bandwidth (30 Days)** widget, updating your exact GB usage and remaining limit every 30 minutes in the background, keeping you safe from over-usage!

---

## 📜 Changelog

All notable changes, updates, and bug fixes are documented in the [CHANGELOG.md](CHANGELOG.md) file, following the *Keep a Changelog* standard.