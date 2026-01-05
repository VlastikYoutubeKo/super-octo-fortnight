# IPTV Stream Proxy Manager

A production-ready IPTV proxy system with automatic failover, source recovery, and Xtream Codes support.

## Features

### Core Functionality
- **Multi-Source Failover**: Automatically tries multiple IPTV sources when one fails
- **Automatic Fallback**: Shows a "channel unavailable" video when all sources fail
- **Source Recovery**: Periodically checks failed sources and automatically switches back when they recover
- **Xtream Codes Support**: Full API integration with Xtream providers
- **TVHeadend Integration**: Monitors active subscriptions and manages streams efficiently

### Stream Resilience
- **H264 Error Handling**: Gracefully handles corrupted video frames
- **Timestamp Correction**: Prevents freezing and audio/video desync
- **Packet Validation**: Discards corrupt packets instead of crashing
- **Smart Reconnection**: Automatic retry logic with configurable intervals

### Management
- **REST API**: Full control via HTTP endpoints
- **Real-time Monitoring**: Track active streams, clients, and fallback status
- **Web Interface**: Optional UI for easy management (index.html)
- **Configurable**: All settings adjustable via JSON config or API

## Installation

### Requirements
- Python 3.7+
- FFmpeg
- Required Python packages:
  ```bash
  pip install flask requests
  ```

### Setup

1. Clone the repository:
   ```bash
   git clone <your-repo-url>
   cd tvheadend
   ```

2. Create your configuration:
   ```bash
   cp config.example.json config.json
   ```

3. Edit `config.json` with your IPTV source URLs

4. Run the proxy:
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
  "auto_fallback": true
}
```

### Configuration Options

**In proxy.py (lines 27-36):**
- `PROXY_PORT = 9000` - Stream proxy port
- `API_PORT = 9005` - REST API port
- `FFMPEG_BUFFER = 65536` - Buffer size for streams
- `CLEANUP_DELAY = 5` - Seconds before cleaning up inactive streams
- `COOLDOWN_TIME = 2` - Cooldown period after source failure
- `STARTUP_TIMEOUT = 10` - Max wait for stream startup
- `SOURCE_RETRY_INTERVAL = 60` - Check failed sources every N seconds
- `SOURCE_CHECK_TIMEOUT = 5` - Timeout for source health checks

**TVHeadend Settings (lines 38-42):**
- `TVH_URL` - TVHeadend server URL
- `TVH_USER` - TVHeadend username
- `TVH_PASS` - TVHeadend password
- `TVH_CHECK_INTERVAL = 3` - Check subscriptions every N seconds
- `TVH_GRACE_PERIOD = 10` - Grace period before cleanup

**Fallback URL (line 48):**
- `FALLBACK_URL` - Video to show when all sources fail

## Usage

### Stream URLs

Access streams via:
```
http://your-server:9000/{source_id}/{channel_id}.ts
```

Example:
```
http://localhost:9000/1/12345.ts
```

### API Endpoints

**Status**
```bash
GET /api/status
```
Returns active streams, clients, fallback count, and uptime

**Configuration**
```bash
GET /api/config
POST /api/config
```

**Sources Management**
```bash
GET /api/sources
POST /api/sources
DELETE /api/sources/{source_id}
```

**Stream Control**
```bash
DELETE /api/streams/{source_id}:{channel_id}
```

**Fallback Control**
```bash
POST /api/fallback          # Toggle manual fallback mode
POST /api/auto-fallback     # Toggle automatic fallback
```

**Xtream Codes**
```bash
GET /api/xtream/providers
GET /api/xtream/providers/{id}/info
GET /api/xtream/providers/{id}/categories?type=live
GET /api/xtream/providers/{id}/streams?category_id=X
GET /api/xtream/providers/{id}/epg/{stream_id}
GET /api/xtream/providers/{id}/category/{cat_id}/playlist.m3u
```

## How It Works

### Source Failover
1. Client requests stream
2. Proxy tries first source URL
3. If fails, tries next source in list
4. If all sources fail, automatically serves fallback video
5. Every 60 seconds, checks if original sources recovered
6. Automatically switches back to real source when available

### Stream Recovery
- Background monitor checks fallback streams every 60 seconds
- Uses HTTP HEAD requests to test source availability
- Gracefully switches from fallback to real source when recovered
- Only checks streams with active viewers

### Logging

Watch logs for:
```
[INFO] Auto-fallback enabled for 2:17575
[INFO] Starting 2:17575 [FALLBACK] - Using fallback video
[INFO] Checking source recovery for 2:17575
[INFO] Source recovered for 2:17575 at URL index 0
[INFO] Recovering 2:17575 - switching from fallback to source [1]
```

## Running in Production

### Using tmux (recommended)
```bash
tmux new-session -s proxy -d "python3 /path/to/proxy.py"
```

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
ExecStart=/usr/bin/python3 /path/to/tvheadend/proxy.py
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

## Troubleshooting

### H264 Errors
The proxy automatically handles:
- `mmco: unref short failure`
- `number of reference frames exceeds max`
- `non-existing SPS/PPS referenced`

These are logged but don't interrupt streaming.

### Source 404 Errors
When sources return 404:
1. Proxy tries all configured sources
2. Falls back to "channel unavailable" video
3. Automatically retries sources every 60 seconds
4. Switches back when source recovers

### No Audio / Video Desync
Fixed via:
- `-fflags +genpts+igndts+discardcorrupt`
- `-avoid_negative_ts make_zero`
- `-async 1` for audio sync

## Files

- `proxy.py` - Main application
- `config.json` - Configuration (not in git - contains credentials)
- `config.example.json` - Template configuration
- `index.html` - Optional web UI
- `.gitignore` - Git exclusions

## License

This project is provided as-is for personal use.

## Contributing

Feel free to submit issues and enhancement requests!
