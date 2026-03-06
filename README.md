# IPTV Stream Proxy Manager

A production-ready IPTV proxy system with automatic failover, source recovery, and Xtream Codes support. 

> **NEW:** This project now features a high-performance **Go (Golang)** implementation alongside the original Python version. The Go version is highly recommended for production due to its massive concurrency improvements, reduced CPU/Memory usage, and seamless buffer handling.

## Features

### Core Functionality
- **Multi-Source Failover**: Automatically tries multiple IPTV sources when one fails
- **Automatic Fallback**: Shows a "channel unavailable" video when all sources fail
- **Source Recovery**: Periodically checks failed sources and automatically switches back when they recover
- **Xtream Codes Support**: Full API integration with Xtream providers
- **TVHeadend Integration**: Monitors active subscriptions and manages streams efficiently

### Stream Resilience
- **Smart Retries**: Immediately retries failed sources up to 3 times before falling back.
- **H264 Error Handling**: Gracefully handles corrupted video frames
- **Timestamp Correction**: Prevents freezing and audio/video desync
- **Packet Validation**: Discards corrupt packets instead of crashing

### Management
- **REST API**: Full control via HTTP endpoints
- **Real-time Monitoring**: Track active streams, clients, and fallback status
- **Web Interface**: Optional UI for easy management (`index.html`)
- **Configurable**: All settings adjustable via JSON config or API. Includes IP/Domain whitelist.

## Installation

### Requirements
- **FFmpeg** is required on the system for both versions.

### Go Version (Recommended)

The Go version is distributed as a single, standalone executable.

1. Download the latest release for your architecture from the [Releases](https://github.com/VlastikYoutubeKo/super-octo-fortnight/releases) page.
2. Place the binary in your preferred directory.
3. Create your configuration file:
   ```bash
   cp config.example.json config.json
   ```
4. Run the proxy:
   ```bash
   ./proxy_server_linux_amd64
   ```

*(To build from source, run `go build -o proxy_server` in the repository root.)*

### Python Version (Legacy)

1. Ensure Python 3.7+ is installed.
2. Install required packages:
   ```bash
   pip install flask requests
   ```
3. Run the proxy:
   ```bash
   python3 proxy.py
   ```

## Configuration

### config.json Structure

```json
{
  "sources": {
    "source_id": [
      "http://server:port/live/user/pass/{channel_id}.ts"
    ]
  },
  "fallback_mode": false,
  "auto_fallback": true,
  "allowed_ips": ["127.0.0.1", "192.168.1.5"],
  "allowed_domains": ["tvh.yourdomain.com"]
}
```

*Note: If both `allowed_ips` and `allowed_domains` are empty or omitted, the proxy operates in public mode and accepts all connections.*

### Configuration Options

**Network Settings:**
- Stream proxy port: `9000`
- REST API port: `9005`

**TVHeadend Settings:**
- `url` - TVHeadend server URL
- `username` - TVHeadend username
- `password` - TVHeadend password

## Usage

### Stream URLs

Access streams via:
```
http://your-server:9000/{source_id}/{channel_id}.ts
```

### API Endpoints

- `GET /api/status`: Returns active streams, clients, fallback count, and uptime
- `GET /api/config`, `POST /api/config`: Manage configuration
- `GET /api/sources`, `POST /api/sources`, `DELETE /api/sources/{source_id}`: Manage sources
- `DELETE /api/streams/{source_id}:{channel_id}`: Stop a stream
- `POST /api/fallback`: Toggle manual fallback mode
- `POST /api/auto-fallback`: Toggle automatic fallback
- `GET /api/xtream/providers...`: Various endpoints for Xtream API integration

## Running in Production

### Using systemd

Create `/etc/systemd/system/iptv-proxy.service`:
```ini
[Unit]
Description=IPTV Stream Proxy
After=network.target

[Service]
Type=simple
User=your-user
WorkingDirectory=/path/to/tvheadend
ExecStart=/path/to/tvheadend/proxy_server_linux_amd64
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Enable and start:
```bash
sudo systemctl enable iptv-proxy
sudo systemctl start iptv-proxy
```

## Contributing
Feel free to submit issues and enhancement requests!