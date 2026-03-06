# Project Context: IPTV Stream Proxy Manager

## Architecture & Workflows
- **Purpose**: A production-ready IPTV proxy system acting as a middleman between TVHeadend and upstream Xtream Codes sources.
- **Language**: Initially written in Python (`proxy.py`), later fully rewritten and migrated to Go for high concurrency and performance without the GIL limitations.
- **Ports**: Runs a stream proxy on `9000` and a REST API / Web UI on `9005`.
- **Stream Producer Strategy**: Uses `ffmpeg` to pull upstream sources, copy the codecs (`-c copy`), correct timestamps (`-avoid_negative_ts make_zero`), and pipe raw MPEG-TS data.
- **Client Consumer Strategy**: The Go proxy uses a memory buffer queue (chan `[]byte` of size 2000 per client) combined with a rolling history (`recentChunks`) to allow instant playback and protect against slow clients.
- **Protocol Details**: To prevent IPTV clients (like TVHeadend or VLC) from disconnecting after a few minutes, the proxy utilizes Go's `http.Hijacker` to directly write raw MPEG-TS bytes onto the TCP connection. This avoids HTTP/1.1 `Transfer-Encoding: chunked` boundaries which are prone to corrupting raw media streams for naive parsers.
- **Resiliency**: Implements an automatic 3-retry limit per source for fast `ffmpeg` drops, and falling back to a "channel unavailable" local video when all upstream links are dead.

## Known Configuration Requirements
- FFmpeg must be installed and available in the system PATH.
- `config.json` stores all streams, user credentials, failover configurations, TVH details, proxies, and a security whitelist for access control (`allowed_ips` and `allowed_domains`).

## Release Strategy
- Binaries are compiled for `linux/amd64`, `linux/arm64`, `windows/amd64`, and `windows/arm64`.
- They are automatically built and published to GitHub Releases via a `.github/workflows/release.yml` GitHub Action whenever a new tag (e.g., `v1.x.x`) is pushed to the repository.