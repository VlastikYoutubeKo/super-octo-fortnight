#!/usr/bin/env python3
"""
IPTV Stream Proxy Manager
Complete production system with Xtream Codes support
"""

import subprocess
import threading
import http.server
import socketserver
import time
import sys
import requests
import json
import os
import re
from typing import Dict, List, Optional, Tuple
import logging
from flask import Flask, jsonify, request, send_file, Response
from requests.auth import HTTPDigestAuth

# ============================================================================
# CONFIGURATION
# ============================================================================

# Server Configuration
PROXY_PORT = 9000              # Stream proxy port
API_PORT = 9005                # Web interface port

# Stream Settings
FFMPEG_BUFFER = 65536          # Buffer size
CLEANUP_DELAY = 5             # Seconds before cleanup
COOLDOWN_TIME = 2            # Cooldown after failure
STARTUP_TIMEOUT = 10           # Max wait for stream start
SOURCE_RETRY_INTERVAL = 60     # Retry failed sources every N seconds
SOURCE_CHECK_TIMEOUT = 5       # Timeout for source health checks

# TVHeadend Settings (loaded from config)
TVH_CHECK_INTERVAL = 3         # Check every N seconds
TVH_GRACE_PERIOD = 10          # Grace before cleanup

# Paths
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
CONFIG_FILE = os.path.join(SCRIPT_DIR, 'config.json')

# Fallback URL when all sources fail
FALLBACK_URL = "https://theariatv.github.io/channeldead.mp4"

# ============================================================================
# LOGGING
# ============================================================================

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s',
    datefmt='%H:%M:%S'
)
log = logging.getLogger(__name__)

# ============================================================================
# GLOBAL STATE
# ============================================================================

config = {
    'sources': {
        "1": [
            "http://tvsystem.my:80/live/cynessanya/cynessanya/{channel_id}.ts",
            "http://tv1.tvsystem.my:80/live/cynessanya/cynessanya/{channel_id}.ts",
            "http://tv2.tvsystem.my:80/live/cynessanya/cynessanya/{channel_id}.ts",
            "http://tv3.tvsystem.my:80/live/cynessanya/cynessanya/{channel_id}.ts",
            "http://line.argontv.nl:80/live/cynessanya/cynessanya/{channel_id}.ts"
        ],
        "2": [
            "http://cynessa.ottb.xyz:80/live/televizekokotu/televizekokotu/{channel_id}.ts"
        ]
    },
    'xtream_providers': {},
    'fallback_mode': False,
    'auto_fallback': True,  # Automatically use fallback video when all sources fail
    'tvheadend': {
        'url': 'http://192.168.1.135:9981',
        'username': 'temp_admin',
        'password': 'temp123'
    }
}

streams = {}
cooldowns = {}
lock = threading.RLock()
start_time = time.time()
last_ffmpeg = 0

# ============================================================================
# CONFIGURATION MANAGEMENT
# ============================================================================

def load_config():
    """Load configuration from file"""
    global config
    if os.path.exists(CONFIG_FILE):
        try:
            with open(CONFIG_FILE, 'r') as f:
                loaded = json.load(f)
                config.update(loaded)
            log.info(f"Loaded config from {CONFIG_FILE}")
        except Exception as e:
            log.error(f"Failed to load config: {e}")
    
    # Auto-detect Xtream providers
    detect_xtream()
    save_config()

def save_config():
    """Save configuration to file"""
    try:
        with open(CONFIG_FILE, 'w') as f:
            json.dump(config, f, indent=2)
        log.info("Config saved")
        return True
    except Exception as e:
        log.error(f"Failed to save config: {e}")
        return False

# ============================================================================
# XTREAM CODES
# ============================================================================

def parse_xtream(url: str) -> Optional[Tuple[str, str, str, str]]:
    """Extract Xtream credentials from URL"""
    pattern = r'https?://([^:]+):(\d+)/(?:live|movie|series)/([^/]+)/([^/]+)/\{channel_id\}'
    match = re.match(pattern, url)
    if match:
        return match.groups()
    return None

def detect_xtream():
    """Auto-detect Xtream providers from sources"""
    providers = {}
    
    for source_id, urls in config['sources'].items():
        for url in urls:
            parsed = parse_xtream(url)
            if parsed:
                server, port, user, password = parsed
                provider_id = f"{server}_{user}"
                
                if provider_id not in config.get('xtream_providers', {}):
                    providers[provider_id] = {
                        'name': server,
                        'url': f"http://{server}:{port}",
                        'username': user,
                        'password': password,
                        'source_id': source_id
                    }
    
    if providers:
        if 'xtream_providers' not in config:
            config['xtream_providers'] = {}
        config['xtream_providers'].update(providers)
        log.info(f"Detected {len(providers)} Xtream providers")

def xtream_api(provider: dict, action: str = None, **params) -> Optional[dict]:
    """Call Xtream Codes API"""
    try:
        url = f"{provider['url']}/player_api.php"
        p = {'username': provider['username'], 'password': provider['password']}
        if action:
            p['action'] = action
        p.update(params)
        
        r = requests.get(url, params=p, timeout=10)
        r.raise_for_status()
        return r.json()
    except Exception as e:
        log.error(f"Xtream API error: {e}")
        return None

def get_categories(provider: dict, cat_type: str = 'live') -> List[dict]:
    """Get Xtream categories"""
    actions = {'live': 'get_live_categories', 'vod': 'get_vod_categories', 'series': 'get_series_categories'}
    result = xtream_api(provider, actions.get(cat_type))
    if result and isinstance(result, list):
        return [{'id': str(c.get('category_id')), 'name': c.get('category_name')} for c in result]
    return []

def get_streams(provider: dict, cat_id: str = None, stream_type: str = 'live') -> List[dict]:
    """Get Xtream streams"""
    actions = {'live': 'get_live_streams', 'vod': 'get_vod_streams', 'series': 'get_series'}
    params = {'category_id': cat_id} if cat_id else {}
    result = xtream_api(provider, actions.get(stream_type), **params)
    return result if isinstance(result, list) else []

def get_epg(provider: dict, stream_id: str) -> dict:
    """Get EPG data"""
    result = xtream_api(provider, 'get_short_epg', stream_id=stream_id, limit=10)
    return result if result else {}

# ============================================================================
# STREAM MANAGEMENT
# ============================================================================

def check_source_health(url: str) -> bool:
    """Check if a source URL is available"""
    if url == FALLBACK_URL:
        return False  # Don't check fallback URL

    try:
        # Quick HEAD request to check if source is available
        response = requests.head(url.replace('{channel_id}', '1'), timeout=SOURCE_CHECK_TIMEOUT, allow_redirects=True)
        # Consider 200, 302, 404 as "server is up" - 404 just means channel doesn't exist
        # We care about connectivity, not specific channel availability
        return response.status_code in [200, 302, 404]
    except:
        return False

def restart_stream_with_source(key: str, source_idx: int):
    """Restart a stream with a specific source URL"""
    with lock:
        if key not in streams:
            return False

        stream = streams[key]
        if not stream.get('on_fallback'):
            return False  # Not on fallback, no need to restart

        if stream['clients'] == 0:
            return False  # No active clients

        # Kill current ffmpeg process
        if stream.get('proc'):
            try:
                stream['proc'].kill()
                stream['proc'].wait(timeout=2)
            except:
                pass

        # Update to use only the working source (plus fallback at end)
        working_url = stream['urls'][source_idx]
        stream['urls'] = [working_url]
        if FALLBACK_URL not in stream['urls']:
            stream['urls'].append(FALLBACK_URL)

        stream['proc'] = None
        stream['pipe'] = None
        stream['on_fallback'] = False
        stream['last_retry'] = time.time()

        log.info(f"Recovering {key} - switching from fallback to source [{source_idx+1}]")

    # Start with the working source
    start_stream(key)
    return True

def start_stream(key: str):
    """Start FFMPEG for stream"""
    def run():
        global last_ffmpeg
        
        # Rate limit
        with lock:
            elapsed = time.time() - last_ffmpeg
            if elapsed < 1.0:
                time.sleep(1.0 - elapsed)
            last_ffmpeg = time.time()
        
        with lock:
            if key not in streams:
                return
            stream = streams[key]
        
        urls = stream['urls']
        for idx, url in enumerate(urls):
            is_fallback = (url == FALLBACK_URL)
            source_type = "FALLBACK" if is_fallback else f"{idx+1}/{len(urls)}"
            log.info(f"Starting {key} [{source_type}]" + (" - Using fallback video" if is_fallback else ""))

            # Update stream tracking
            with lock:
                if key in streams:
                    streams[key]['current_url_idx'] = idx
                    streams[key]['on_fallback'] = is_fallback

            cmd = ["nice", "-n", "10", "ffmpeg"]
            if is_fallback:
                cmd.extend(["-stream_loop", "-1", "-re"])
            
            cmd.extend([
            "-loglevel", "error", "-user_agent", "VLC/3.0.20",
            # Aggressive reconnection (like VLC)
            "-reconnect", "1", "-reconnect_streamed", "1",
            "-reconnect_delay_max", "10", "-reconnect_on_network_error", "1",
            # Very aggressive stream analysis (VLC-style)
            "-analyzeduration", "20000000",  # 20M - analyze more data
            "-probesize", "30000000",        # 30M - probe more data
            # Input flags - be very forgiving like VLC
            "-fflags", "+genpts+igndts+discardcorrupt",
            "-err_detect", "ignore_err",
            "-max_error_rate", "1.0",
            # Use wallclock timestamps for broken streams
            "-use_wallclock_as_timestamps", "1",
            "-i", url,
            # Increase thread queue size for stability
            "-thread_queue_size", "512",
            # Map all streams explicitly
            "-map", "0:v:0?",  # First video stream
            "-map", "0:a?",    # All audio streams
            "-map", "0:s?",    # All subtitle streams
            # Copy codecs without re-encoding
            "-c:v", "copy",
            "-c:a", "copy",
            "-c:s", "copy",
            # Fix timestamp issues (VLC does this automatically)
            "-avoid_negative_ts", "make_zero",
            "-start_at_zero",
            # Audio sync with higher tolerance
            "-async", "1",
            "-vsync", "cfr",
            "-copyts",
            # Longer max delay for bad streams
            "-max_delay", "5000000",
            "-max_interleave_delta", "0",
            # Output format
            "-f", "mpegts",
            "-mpegts_copyts", "1",
            "-muxdelay", "0",
            "-muxpreload", "0",
            "pipe:1"
            ])
            
            try:
                proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, bufsize=FFMPEG_BUFFER)
            except Exception as e:
                log.error(f"FFMPEG failed: {e}")
                with lock:
                    if key in streams:
                        streams[key]['error'] = str(e)
                break
            
            with lock:
                if key in streams:
                    streams[key]['proc'] = proc
                    streams[key]['pipe'] = proc.stdout
                    streams[key]['error'] = None
                else:
                    proc.kill()
                    return
            
            # Monitor errors
            def log_errors():
                errors = []
                for line in iter(proc.stderr.readline, b''):
                    s = line.decode('utf-8', errors='ignore').strip()
                    log.warning(f"FFMPEG_STDERR: {key} - {s}") # <-- ADD THIS LINE
                    if 'error' in s.lower():
                        errors.append(s)
                if errors:
                    with lock:
                        if key in streams:
                            streams[key]['error'] = errors[-1]
            
            t = threading.Thread(target=log_errors, daemon=True)
            t.start()
            
            ret = proc.wait()
            t.join(timeout=2)

            with lock:
                if key in streams:
                    streams[key]['proc'] = None
                    streams[key]['pipe'] = None

            # Exit code 0 = normal end, -9 = SIGKILL (TVH disconnect)
            if ret == 0 or ret == -9:
                log.info(f"Stream {key} ended normally (code {ret})")
                return

            log.warning(f"Stream {key} failed (code {ret})")
            time.sleep(0.5)
        
        # All failed
        with lock:
            cooldowns[key] = time.time() + COOLDOWN_TIME
        cleanup_stream(key)
    
    threading.Thread(target=run, daemon=True).start()

def cleanup_stream(key: str, delay: int = 0):
    """Clean up stream"""
    def do_cleanup():
        with lock:
            if key not in streams:
                return
            s = streams[key]
            if delay > 0 and s['clients'] > 0:
                return
            if s.get('proc'):
                try:
                    s['proc'].kill()
                    s['proc'].wait(timeout=2)
                except:
                    pass
            log.info(f"Cleaned up: {key}")
            del streams[key]
    
    if delay > 0:
        threading.Timer(delay, do_cleanup).start()
    else:
        do_cleanup()

# ============================================================================
# HTTP PROXY
# ============================================================================

class ProxyHandler(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass
    
    def do_GET(self):
        parts = self.path.strip("/").split("/")
        if len(parts) < 2:
            self.send_error(400)
            return
        
        source_id = parts[0]
        channel_id = parts[1].rsplit('.', 1)[0]
        key = f"{source_id}:{channel_id}"
        
        # Check cooldown
        with lock:
            if key in cooldowns:
                remaining = int(cooldowns[key] - time.time())
                if remaining > 0:
                    self.send_error(503, f"Cooldown: {remaining}s")
                    return
        
        # Check source exists
        if source_id not in config['sources'] and not config['fallback_mode']:
            self.send_error(404)
            return
        
        # Create stream
        with lock:
            if key not in streams:
                if config['fallback_mode']:
                    urls = [FALLBACK_URL]
                else:
                    urls = [u.format(channel_id=channel_id) for u in config['sources'].get(source_id, [])]
                    # Automatically append fallback video as last resort (if enabled)
                    if config.get('auto_fallback', True) and urls and FALLBACK_URL not in urls:
                        urls.append(FALLBACK_URL)
                        log.info(f"Auto-fallback enabled for {key}")

                streams[key] = {
                    'proc': None, 'pipe': None, 'clients': 0,
                    'urls': urls, 'created': time.time(), 'error': None,
                    'current_url_idx': None,  # Track which URL is currently playing
                    'on_fallback': False,      # Track if using fallback video
                    'last_retry': 0            # Last time we checked sources
                }
                log.info(f"Created stream: {key}")
                start_stream(key)
            
            streams[key]['clients'] += 1
            log.info(f"Client connected: {key} (total: {streams[key]['clients']})")
        
        # Wait for start
        wait = 0
        while wait < STARTUP_TIMEOUT:
            with lock:
                s = streams.get(key)
                if s and s.get('proc') and s.get('pipe'):
                    break
                if s and s.get('error'):
                    with lock:
                        streams[key]['clients'] -= 1
                    self.send_error(503)
                    return
            time.sleep(0.1)
            wait += 0.1
        
        with lock:
            s = streams.get(key)
            if not s or not s.get('pipe'):
                if s:
                    s['clients'] -= 1
                self.send_error(503)
                return
            pipe = s['pipe']
            proc = s['proc']
        
        # Send stream
        self.send_response(200)
        self.send_header("Content-Type", "video/MP2T")
        self.end_headers()
        
        try:
            while True:
                if proc.poll() is not None or pipe.closed:
                    break
                data = pipe.read(FFMPEG_BUFFER)
                if not data:
                    break
                try:
                    self.wfile.write(data)
                except:
                    break
        except:
            pass
        finally:
            with lock:
                if key in streams:
                    streams[key]['clients'] -= 1
                    log.info(f"Client disconnected: {key} (remaining: {streams[key]['clients']})")
                    if streams[key]['clients'] == 0:
                        cleanup_stream(key, delay=CLEANUP_DELAY)

# ============================================================================
# SOURCE RECOVERY MONITOR
# ============================================================================

def monitor_source_recovery():
    """Periodically check if failed sources have recovered"""
    log.info("Source recovery monitor started")
    while True:
        try:
            time.sleep(SOURCE_RETRY_INTERVAL)

            fallback_streams = []
            with lock:
                for key, stream in list(streams.items()):
                    if stream.get('on_fallback') and stream.get('clients', 0) > 0:
                        # Check if enough time has passed since last retry
                        if time.time() - stream.get('last_retry', 0) >= SOURCE_RETRY_INTERVAL:
                            fallback_streams.append((key, stream['urls'][:]))  # Copy URLs

            # Check sources outside of lock to avoid blocking
            for key, urls in fallback_streams:
                log.info(f"Checking source recovery for {key}")

                # Try each non-fallback URL
                for idx, url in enumerate(urls):
                    if url == FALLBACK_URL:
                        continue

                    if check_source_health(url):
                        log.info(f"Source recovered for {key} at URL index {idx}")
                        restart_stream_with_source(key, idx)
                        break  # Stop checking once we find a working source
                else:
                    # No sources recovered, update last_retry time
                    with lock:
                        if key in streams:
                            streams[key]['last_retry'] = time.time()
                            log.info(f"No sources available yet for {key}, will retry later")

        except Exception as e:
            log.error(f"Source recovery error: {e}")

# ============================================================================
# TVHeadend MONITOR
# ============================================================================

def monitor_tvh():
    """Monitor TVHeadend subscriptions"""
    log.info("TVHeadend monitor started")
    while True:
        try:
            tvh = config.get('tvheadend', {})
            if not tvh.get('url'):
                time.sleep(TVH_CHECK_INTERVAL)
                continue

            r = requests.get(f"{tvh['url']}/api/status/subscriptions",
                           auth=HTTPDigestAuth(tvh.get('username', ''), tvh.get('password', '')), timeout=5)
            r.raise_for_status()
            
            active = set()
            for sub in r.json().get('entries', []):
                url = sub.get('server_url', '')
                if url:
                    parts = url.strip('/').split('/')
                    if len(parts) > 1:
                        active.add(f"{parts[0]}:{parts[1].rsplit('.', 1)[0]}")
            
            with lock:
                for key in list(streams.keys()):
                    if key not in active:
                        s = streams[key]
                        if s['clients'] == 0 and (time.time() - s['created']) >= TVH_GRACE_PERIOD:
                            log.info(f"TVH: cleanup {key}")
                            cleanup_stream(key)
        except:
            pass
        
        with lock:
            expired = [k for k, v in cooldowns.items() if time.time() > v]
            for k in expired:
                del cooldowns[k]
        
        time.sleep(TVH_CHECK_INTERVAL)

# ============================================================================
# REST API
# ============================================================================

app = Flask(__name__)
app.config['JSON_SORT_KEYS'] = False
logging.getLogger('werkzeug').setLevel(logging.ERROR)

@app.route('/api/status')
def api_status():
    with lock:
        s = []
        total = 0
        fallback_count = 0
        for key, stream in streams.items():
            c = stream['clients']
            total += c

            on_fallback = stream.get('on_fallback', False)
            if on_fallback:
                fallback_count += 1

            # Calculate next retry time for fallback streams
            next_retry = None
            if on_fallback and stream.get('last_retry', 0) > 0:
                next_retry = int(stream['last_retry'] + SOURCE_RETRY_INTERVAL - time.time())
                if next_retry < 0:
                    next_retry = 0

            s.append({
                'key': key,
                'clients': c,
                'age': int(time.time() - stream['created']),
                'url': stream['urls'][stream.get('current_url_idx', 0)] if stream.get('urls') else 'N/A',
                'error': stream.get('error'),
                'on_fallback': on_fallback,
                'next_retry': next_retry
            })
        c = [{'key': k, 'remaining': int(v - time.time())} for k, v in cooldowns.items() if v > time.time()]

    return jsonify({
        'streams': s,
        'total_streams': len(s),
        'total_clients': total,
        'fallback_streams': fallback_count,
        'fallback_mode': config['fallback_mode'],
        'auto_fallback': config.get('auto_fallback', True),
        'retry_interval': SOURCE_RETRY_INTERVAL,
        'uptime': int(time.time() - start_time),
        'cooldowns': c
    })

@app.route('/api/config')
def api_get_config():
    return jsonify(config)

@app.route('/api/config', methods=['POST'])
def api_update_config():
    global config
    data = request.get_json()
    if data:
        config.update(data)
        detect_xtream()
        save_config()
    return jsonify({'success': True})

@app.route('/api/sources')
def api_get_sources():
    return jsonify({'sources': config['sources']})

@app.route('/api/sources', methods=['POST'])
def api_update_sources():
    data = request.get_json()
    if data and 'sources' in data:
        config['sources'] = data['sources']
        detect_xtream()
        save_config()
    return jsonify({'success': True})

@app.route('/api/sources/<source_id>', methods=['DELETE'])
def api_delete_source(source_id):
    if source_id in config['sources']:
        del config['sources'][source_id]
        save_config()
    return jsonify({'success': True})

@app.route('/api/streams/<path:key>', methods=['DELETE'])
def api_kill_stream(key):
    cleanup_stream(key)
    return jsonify({'success': True})

@app.route('/api/fallback', methods=['POST'])
def api_toggle_fallback():
    config['fallback_mode'] = not config['fallback_mode']
    save_config()
    log.warning(f"Fallback mode: {config['fallback_mode']}")
    return jsonify({'fallback_mode': config['fallback_mode']})

@app.route('/api/auto-fallback', methods=['POST'])
def api_toggle_auto_fallback():
    config['auto_fallback'] = not config.get('auto_fallback', True)
    save_config()
    log.info(f"Auto-fallback: {config['auto_fallback']}")
    return jsonify({'auto_fallback': config['auto_fallback']})

@app.route('/api/xtream/providers')
def api_xtream_providers():
    return jsonify({'providers': config.get('xtream_providers', {})})

@app.route('/api/xtream/providers/<provider_id>/info')
def api_xtream_info(provider_id):
    providers = config.get('xtream_providers', {})
    if provider_id not in providers:
        return jsonify({'error': 'Not found'}), 404
    info = xtream_api(providers[provider_id])
    return jsonify({'info': info} if info else {'error': 'Failed'})

@app.route('/api/xtream/providers/<provider_id>/categories')
def api_xtream_categories(provider_id):
    providers = config.get('xtream_providers', {})
    if provider_id not in providers:
        return jsonify({'error': 'Not found'}), 404
    cat_type = request.args.get('type', 'live')
    cats = get_categories(providers[provider_id], cat_type)
    return jsonify({'categories': cats})

@app.route('/api/xtream/providers/<provider_id>/streams')
def api_xtream_streams(provider_id):
    providers = config.get('xtream_providers', {})
    if provider_id not in providers:
        return jsonify({'error': 'Not found'}), 404
    cat_id = request.args.get('category_id')
    stream_type = request.args.get('type', 'live')
    streams_data = get_streams(providers[provider_id], cat_id, stream_type)
    return jsonify({'streams': streams_data})

@app.route('/api/xtream/providers/<provider_id>/epg/<stream_id>')
def api_xtream_epg(provider_id, stream_id):
    providers = config.get('xtream_providers', {})
    if provider_id not in providers:
        return jsonify({'error': 'Not found'}), 404
    epg = get_epg(providers[provider_id], stream_id)
    return jsonify({'epg': epg})

@app.route('/api/xtream/test', methods=['POST'])
def api_xtream_test():
    data = request.get_json()
    provider = {'url': data.get('url'), 'username': data.get('username'), 'password': data.get('password')}
    result = xtream_api(provider)
    return jsonify({'success': True, 'data': result} if result else {'error': 'Failed'})

@app.route('/api/xtream/providers/<provider_id>/category/<category_id>/playlist.m3u')
def api_xtream_playlist(provider_id, category_id):
    providers = config.get('xtream_providers', {})
    if provider_id not in providers:
        return jsonify({'error': 'Provider not found'}), 404
    
    provider = providers[provider_id]
    source_id = provider.get('source_id')
    if not source_id:
            return jsonify({'error': 'Provider has no source_id configured'}), 500

    # Get all streams for this category
    streams_data = get_streams(provider, category_id, 'live')
    if not streams_data:
        return jsonify({'error': 'No streams found in this category'}), 404

    # Get the host (e.g., 192.168.1.135) from the request
    # This avoids hardcoding the IP and uses whatever host the client is using
    proxy_host = request.host.split(':')[0]
    
    m3u_lines = ["#EXTM3U"]
    
    for stream in streams_data:
        stream_id = stream.get('stream_id') or stream.get('id')
        stream_name = stream.get('name', f'Stream {stream_id}')
        epg_id = stream.get('epg_channel_id', '') # The EPG ID you wanted

        # Build the #EXTINF line with EPG tags
        extinf = f'#EXTINF:-1 tvg-id="{epg_id}" tvg-name="{stream_name}",{stream_name}'
        m3u_lines.append(extinf)
        
        # Build the dynamic proxy URL
        proxy_url = f"http://{proxy_host}:{PROXY_PORT}/{source_id}/{stream_id}.ts"
        m3u_lines.append(proxy_url)

    # Join all lines with a newline
    m3u_content = "\n".join(m3u_lines)
    
    # Return as a downloadable M3U file
    return Response(m3u_content, mimetype='audio/mpegurl', headers={
        "Content-Disposition": f"attachment; filename=\"playlist_{category_id}.m3u\""
    })


@app.route('/')
def serve_ui():
    ui_file = os.path.join(SCRIPT_DIR, 'index.html')
    if os.path.exists(ui_file):
        return send_file(ui_file)
    return jsonify({'status': 'running', 'version': '2.0'})

# ============================================================================
# MAIN
# ============================================================================

def run_proxy():
    socketserver.TCPServer.allow_reuse_address = True
    with socketserver.ThreadingTCPServer(("", PROXY_PORT), ProxyHandler) as server:
        log.info(f"Proxy running on :{PROXY_PORT}")
        server.serve_forever()

def run_api():
    log.info(f"API running on :{API_PORT}")
    app.run(host='0.0.0.0', port=API_PORT, debug=False, threaded=True)

def main():
    print("=" * 60)
    print("IPTV Stream Proxy Manager")
    print("=" * 60)
    print(f"Directory: {SCRIPT_DIR}")

    load_config()

    threading.Thread(target=monitor_tvh, daemon=True).start()
    threading.Thread(target=monitor_source_recovery, daemon=True).start()
    threading.Thread(target=run_api, daemon=True).start()

    try:
        run_proxy()
    except KeyboardInterrupt:
        log.info("Stopped")
    except Exception as e:
        log.error(f"Fatal: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
