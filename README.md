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