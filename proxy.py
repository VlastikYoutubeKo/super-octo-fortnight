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
import queue
import random
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
FFMPEG_BUFFER = 65536          # Buffer size (64KB chunks for queue)
BUFFER_QUEUE_SIZE = 200        # Queue size: 200 * 64KB = ~12MB buffer
CLEANUP_DELAY = 5              # Seconds before cleanup
COOLDOWN_TIME = 2              # Cooldown after failure
STARTUP_TIMEOUT = 10           # Max wait for stream start
SOURCE_RETRY_INTERVAL = 60     # Retry failed sources every N seconds
SOURCE_CHECK_TIMEOUT = 5       # Timeout for source health checks
NETWORK_TIMEOUT = 10000000     # 10s timeout for FFMPEG (microseconds)

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
url_counters = {}  # Track active streams per source URL for load balancing

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

def select_best_url(source_id: str, urls: list) -> list:
    """
    Randomized load balancing: shuffle URLs to distribute load across servers.
    Returns URLs in random order to avoid all streams hitting the same server.
    """
    if len(urls) <= 1:
        return urls

    # Shuffle URLs for random distribution - prevents thundering herd
    shuffled_urls = urls[:]
    random.shuffle(shuffled_urls)

    log.info(f"Load balancing {source_id}: Randomized {len(shuffled_urls)} URLs (first: {shuffled_urls[0].split('/')[2] if shuffled_urls else 'none'})")

    return shuffled_urls

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

        # Kill current ffmpeg process (producer will restart with new URLs)
        if stream.get('proc'):
            try:
                stream['proc'].terminate()
                stream['proc'].wait(timeout=1)
            except:
                try:
                    stream['proc'].kill()
                except:
                    pass

        # Update to use only the working source (plus fallback at end)
        working_url = stream['urls'][source_idx]
        stream['urls'] = [working_url]
        if FALLBACK_URL not in stream['urls']:
            stream['urls'].append(FALLBACK_URL)

        stream['proc'] = None
        stream['on_fallback'] = False
        stream['last_retry'] = time.time()

        log.info(f"Recovering {key} - switching from fallback to source [{source_idx+1}]")

    # Producer thread will automatically pick up new URLs and restart
    return True

def stream_producer(key: str):
    """
    Producer thread: Manages FFMPEG process and feeds data into queue.
    If FFMPEG dies, restarts it while keeping queue alive (no client disconnect).
    """
    log.info(f"Producer started: {key}")

    while True:
        # Check stream still exists and has clients
        with lock:
            if key not in streams:
                break
            stream = streams[key]
            if stream['clients'] <= 0:
                break

        urls = stream['urls']

        # Try each URL (failover)
        for idx, url in enumerate(urls):
            is_fallback = (url == FALLBACK_URL)

            with lock:
                if key not in streams:
                    break
                streams[key]['current_url_idx'] = idx
                streams[key]['on_fallback'] = is_fallback

            log.info(f"Starting {key} [{idx+1}/{len(urls)}]" + (" - Fallback" if is_fallback else ""))

            # Build FFMPEG command
            cmd = ["ffmpeg"]

            if is_fallback:
                cmd.extend(["-stream_loop", "-1", "-re"])

            cmd.extend([
                "-loglevel", "error", "-user_agent", "VLC/3.0.20",
                # Network reconnection
                "-reconnect", "1", "-reconnect_streamed", "1",
                "-reconnect_delay_max", "10",
            ])

            cmd.extend([
                # Balanced stream analysis - enough for audio detection
                "-analyzeduration", "7000000",   # 7M - sweet spot
                "-probesize", "15000000",        # 15M - reasonable
                # Input flags - forgiving but not excessive
                "-fflags", "+genpts+igndts+discardcorrupt",
                "-err_detect", "ignore_err",
                "-i", url,
                # Map streams
                "-map", "0:v?",  # All video streams
                "-map", "0:a?",  # All audio streams
                "-map", "0:s?",  # All subtitles
                # Copy codecs
                "-c", "copy",
                # Fix timestamp issues
                "-avoid_negative_ts", "make_zero",
                "-start_at_zero",
                # Audio/video sync
                "-async", "1",
                "-vsync", "passthrough",
                # Output format
                "-f", "mpegts",
                "-flush_packets", "1",
                "pipe:1"
            ])

            proc = None
            try:
                proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, bufsize=FFMPEG_BUFFER)

                with lock:
                    if key in streams:
                        streams[key]['proc'] = proc

                # Error logging thread
                def log_err():
                    for line in iter(proc.stderr.readline, b''):
                        pass  # Suppress for performance (already working)
                threading.Thread(target=log_err, daemon=True).start()

                # Read from FFMPEG and feed queue
                while True:
                    # Check if stream still has clients
                    with lock:
                        if key not in streams or streams[key]['clients'] <= 0:
                            if proc:
                                try:
                                    proc.terminate()
                                    proc.wait(timeout=1)
                                except:
                                    try:
                                        proc.kill()
                                    except:
                                        pass
                            return

                    chunk = proc.stdout.read(FFMPEG_BUFFER)
                    if not chunk:
                        break  # FFMPEG ended, restart

                    try:
                        # Put data in queue (with timeout for backpressure)
                        stream['buffer'].put(chunk, timeout=5)
                    except queue.Full:
                        continue  # Queue full, try again

            except Exception as e:
                log.error(f"Producer error {key}: {e}")
            finally:
                if proc:
                    try:
                        proc.terminate()
                        proc.wait(timeout=1)
                    except:
                        try:
                            proc.kill()
                        except:
                            pass

            # FFMPEG died, log and restart
            log.warning(f"Source {idx+1} for {key} ended, restarting...")
            time.sleep(1)  # Brief delay before restart

            # Check if we should continue
            with lock:
                if key not in streams or streams[key]['clients'] <= 0:
                    return

        # All URLs failed, wait before retry
        time.sleep(1)

    log.info(f"Producer ended: {key}")

def start_stream(key: str):
    """Start producer thread for stream"""
    threading.Thread(target=stream_producer, args=(key,), daemon=True).start()

def cleanup_stream(key: str, delay: int = 0):
    """Clean up stream and stop producer"""
    def do_cleanup():
        with lock:
            if key not in streams:
                return
            s = streams[key]
            if delay > 0 and s['clients'] > 0:
                return
            # Setting clients to 0 will signal producer to stop
            s['clients'] = 0
            if s.get('proc'):
                try:
                    s['proc'].terminate()
                    s['proc'].wait(timeout=1)
                except:
                    try:
                        s['proc'].kill()
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
                    # Get all URLs for this source
                    source_urls = [u.format(channel_id=channel_id) for u in config['sources'].get(source_id, [])]

                    # Load balance: reorder URLs to use least-busy server first
                    urls = select_best_url(source_id, source_urls)

                    # Automatically append fallback video as last resort (if enabled)
                    if config.get('auto_fallback', True) and urls and FALLBACK_URL not in urls:
                        urls.append(FALLBACK_URL)
                        log.info(f"Auto-fallback enabled for {key}")

                streams[key] = {
                    'buffer': queue.Queue(maxsize=BUFFER_QUEUE_SIZE),  # Persistent buffer
                    'proc': None, 'clients': 0,
                    'urls': urls, 'created': time.time(),
                    'current_url_idx': None,  # Track which URL is currently playing
                    'on_fallback': False,      # Track if using fallback video
                    'last_retry': 0            # Last time we checked sources
                }
                log.info(f"Created stream: {key}")
                start_stream(key)
            
            streams[key]['clients'] += 1
            stream_buffer = streams[key]['buffer']  # Get buffer reference
            log.info(f"Client connected: {key} (total: {streams[key]['clients']})")

        # Wait for first data in buffer
        wait = 0
        while wait < STARTUP_TIMEOUT:
            with lock:
                s = streams.get(key)
                if s and s.get('proc'):
                    break
            time.sleep(0.1)
            wait += 0.1

        # Send stream
        self.send_response(200)
        self.send_header("Content-Type", "video/MP2T")
        self.end_headers()

        # Read from queue (consumer)
        empty_reads = 0
        try:
            while True:
                try:
                    # Wait for data in queue (timeout 10s)
                    # Producer may restart FFMPEG, causing brief empty periods
                    # We wait instead of closing connection
                    chunk = stream_buffer.get(timeout=10)
                    self.wfile.write(chunk)
                    empty_reads = 0
                except queue.Empty:
                    # No data for 10s - producer might be struggling
                    empty_reads += 1
                    if empty_reads > 3:  # 30s without data = give up
                        log.warning(f"Timeout waiting for data: {key}")
                        break
                    continue
                except (BrokenPipeError, ConnectionResetError):
                    # Client disconnected
                    break
                except Exception as e:
                    log.error(f"Stream error: {e}")
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
                'queue_size': stream['buffer'].qsize() if stream.get('buffer') else 0,
                'age': int(time.time() - stream['created']),
                'url': stream['urls'][stream.get('current_url_idx', 0)] if stream.get('urls') else 'N/A',
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
