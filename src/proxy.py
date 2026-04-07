#!/usr/bin/env python3
"""
IPTV Stream Proxy Manager V4.2 (Ultimate Hybrid - Syntax Fixed & Redacted)
Oprava syntaxe, Anti-Ban maskování a fixace audia (PAT/PMT) pro Tvheadend.
Citlivé údaje byly odstraněny a nahrazeny zástupnými hodnotami.
"""

import subprocess
import threading
import http.server
import socketserver
import socket
import time
import sys
import requests
import json
import os
import queue
import random
import collections
import re
from urllib.parse import urlparse
from typing import Dict, List, Optional, Tuple, Set, Any
import logging
from flask import Flask, jsonify, request, send_file, Response
from requests.auth import HTTPDigestAuth
import urllib3

# Potlačení varování pro nešifrovaná IPTV připojení
urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

# ============================================================================
# CONSTANTS & CONFIGURATION
# ============================================================================

PROXY_PORT = 9000
API_PORT = 9005
FFMPEG_BUFFER = 65536
BUFFER_QUEUE_SIZE = 300
CLEANUP_DELAY = 15
COOLDOWN_TIME = 2
STARTUP_TIMEOUT = 15
STARTUP_BUFFER_CHUNKS = 2
SOURCE_RETRY_INTERVAL = 60
DATA_TIMEOUT = 30
TVH_CHECK_INTERVAL = 3
TVH_GRACE_PERIOD = 15

FALLBACK_URL = "http://example.com/fallback.mp4"
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
CONFIG_FILE = os.path.join(SCRIPT_DIR, 'config.json')
WHITELIST_DOMAINS = ["example.com", "whitelist.example.org"]

logging.basicConfig(level=logging.INFO, format='%(asctime)s [%(levelname)s] %(message)s', datefmt='%H:%M:%S')
log = logging.getLogger(__name__)

# ============================================================================
# REALISTIC USER-AGENT BANK
# ============================================================================
REAL_UAS = [
    "VLC/3.0.20 LibVLC/3.0.20",
    "VLC/3.0.18 LibVLC/3.0.18",
    "TiviMate/4.7.0",
    "TiviMate/4.6.1",
    "Kodi/20.1 (X11; Linux x86_64) App_Bitness/64 Version/20.1-Nexus",
    "Kodi/19.4 (Windows NT 10.0; Win64; x64) App_Bitness/64 Version/19.4-Matrix",
    "ExoPlayer/2.18.1",
    "ExoPlayer/2.17.1",
    "SmartIPTV",
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36"
]

# ============================================================================
# SECURITY MANAGER
# ============================================================================

class SecurityManager:
    def __init__(self):
        self.allowed_ips_cache: Set[str] = set(["127.0.0.1", "::1"])
        self.last_dns_check = 0

    def get_allowed_ips(self) -> Set[str]:
        now = time.time()
        if now - self.last_dns_check > 300:
            new_ips = set(["127.0.0.1", "::1"])
            for dom in WHITELIST_DOMAINS:
                try:
                    ips = socket.gethostbyname_ex(dom)[2]
                    new_ips.update(ips)
                except Exception:
                    pass
            self.allowed_ips_cache = new_ips
            self.last_dns_check = now
            log.info(f"Updated allowed IPs whitelist: {self.allowed_ips_cache}")
        return self.allowed_ips_cache

    def is_allowed(self, ip: str) -> bool:
        return ip in self.get_allowed_ips()

security = SecurityManager()

# ============================================================================
# APP CONFIGURATION
# ============================================================================

class AppConfig:
    def __init__(self):
        self.data = {
            'sources': {
                "source1": ["http://provider1.example.com/live/username/password/{channel_id}.ts"],
                "source2": ["http://provider2.example.com/live/username/password/{channel_id}.ts"]
            },
            'xtream_providers': {},
            'fallback_mode': False,
            'auto_fallback': True,
            'tvheadend': {
                'url': 'http://127.0.0.1:9981',
                'username': 'admin',
                'password': 'password'
            }
        }
        self.load()

    def load(self):
        if os.path.exists(CONFIG_FILE):
            try:
                with open(CONFIG_FILE, 'r') as f:
                    self.data.update(json.load(f))
            except Exception as e:
                log.error(f"Failed to load config: {e}")
        self.detect_xtream()
        self.save()

    def save(self):
        try:
            with open(CONFIG_FILE, 'w') as f:
                json.dump(self.data, f, indent=2)
        except Exception:
            pass

    def update(self, new_data: Dict):
        self.data.update(new_data)
        self.detect_xtream()
        self.save()

    def get_random_proxy(self) -> Optional[str]:
        proxies = self.data.get('proxies', [])
        return random.choice(proxies) if proxies else None

    def detect_xtream(self):
        providers = {}
        pattern = r'https?://([^:]+):?(\d+)?/(?:live|movie|series)/([^/]+)/([^/]+)/\{channel_id\}'

        for source_id, urls in self.data['sources'].items():
            for url in urls:
                match = re.match(pattern, url)
                if match:
                    groups = match.groups()
                    server = groups[0]
                    port = groups[1] or "80"
                    user = groups[2]
                    password = groups[3]
                    provider_id = f"{server}_{user}"
                    if provider_id not in self.data.get('xtream_providers', {}):
                        providers[provider_id] = {
                            'name': server,
                            'url': f"http://{server}:{port}",
                            'username': user,
                            'password': password,
                            'source_id': source_id
                        }
        if providers:
            self.data.setdefault('xtream_providers', {}).update(providers)

config = AppConfig()

# ============================================================================
# XTREAM API
# ============================================================================

class XtreamAPI:
    @staticmethod
    def call(provider: dict, action: str = None, **params) -> Optional[Any]:
        try:
            url = f"{provider['url']}/player_api.php"
            p = {'username': provider['username'], 'password': provider['password']}
            if action: p['action'] = action
            p.update(params)

            proxy_url = config.get_random_proxy()
            req_proxies = {"http": proxy_url, "https": proxy_url} if proxy_url else None

            r = requests.get(url, params=p, timeout=10, proxies=req_proxies, verify=False)
            r.raise_for_status()
            return r.json()
        except Exception:
            return None

    @staticmethod
    def get_categories(provider: dict, cat_type: str = 'live') -> List[dict]:
        actions = {'live': 'get_live_categories', 'vod': 'get_vod_categories', 'series': 'get_series_categories'}
        res = XtreamAPI.call(provider, actions.get(cat_type))
        if isinstance(res, list):
            return [{'id': str(c.get('category_id')), 'name': c.get('category_name')} for c in res]
        return []

    @staticmethod
    def get_streams(provider: dict, cat_id: str = None, stream_type: str = 'live') -> List[dict]:
        actions = {'live': 'get_live_streams', 'vod': 'get_vod_streams', 'series': 'get_series'}
        params = {'category_id': cat_id} if cat_id else {}
        res = XtreamAPI.call(provider, actions.get(stream_type), **params)
        return res if isinstance(res, list) else []

    @staticmethod
    def get_epg(provider: dict, stream_id: str) -> dict:
        res = XtreamAPI.call(provider, 'get_short_epg', stream_id=stream_id, limit=10)
        return res if res else {}

    @staticmethod
    def get_vod_info(provider: dict, vod_id: str) -> dict:
        res = XtreamAPI.call(provider, 'get_vod_info', vod_id=vod_id)
        return res if res else {}

    @staticmethod
    def get_series_info(provider: dict, series_id: str) -> dict:
        res = XtreamAPI.call(provider, 'get_series_info', series_id=series_id)
        return res if res else {}

    @staticmethod
    def get_full_epg(provider: dict, stream_id: str) -> dict:
        res = XtreamAPI.call(provider, 'get_simple_data_table', stream_id=stream_id)
        return res if res else {}

# ============================================================================
# HYBRID STREAM MANAGER (FFmpeg Core)
# ============================================================================

class ActiveFFmpegStream:
    def __init__(self, key: str, urls: List[str]):
        self.key = key
        self.urls = urls
        self.clients = 0
        self.client_queues: List[queue.Queue] = []
        self.recent_chunks = collections.deque(maxlen=BUFFER_QUEUE_SIZE)
        self.running = True
        self.created = time.time()
        self.last_client_time = time.time()
        self.last_data_time = time.time()
        self.proc: Optional[subprocess.Popen] = None
        self.user_agent = random.choice(REAL_UAS)
        
        self.thread = threading.Thread(target=self._producer_loop, daemon=True)
        self.thread.start()

    def add_client(self) -> queue.Queue:
        q = queue.Queue(maxsize=500)
        with stream_mgr.lock:
            for chunk in list(self.recent_chunks):
                try: q.put_nowait(chunk)
                except queue.Full: pass
        
        self.client_queues.append(q)
        self.clients += 1
        self.last_client_time = time.time()
        return q

    def remove_client(self, q: queue.Queue):
        if q in self.client_queues:
            self.client_queues.remove(q)
            self.clients -= 1
            self.last_client_time = time.time()

    def stop(self):
        self.running = False
        self.recent_chunks.clear()
        for q in self.client_queues:
            try: q.put_nowait(None)
            except queue.Full: pass
        if self.proc:
            try: self.proc.kill()
            except: pass

    def _producer_loop(self):
        log.info(f" FFmpeg startuje stream {self.key} (Masking as: {self.user_agent.split('/')[0]})")
        
        url_idx = 0
        while self.running:
            url = self.urls[url_idx]
            is_fallback = (url == FALLBACK_URL)

            cmd = ["ffmpeg", "-hide_banner"]
            if is_fallback: cmd.extend(["-stream_loop", "-1", "-re"])
            
            cmd.extend([
                "-loglevel", "fatal", 
                "-user_agent", self.user_agent,
                "-reconnect", "1", 
                "-reconnect_streamed", "1",
                "-reconnect_delay_max", "5",
                "-reconnect_on_http_error", "4xx,5xx",
                
                "-analyzeduration", "15000000",
                "-probesize", "50000000",
                
                "-fflags", "+genpts+igndts+discardcorrupt",
                "-i", url,
                
                "-map", "0:v:0?", "-map", "0:a:0?", "-map", "0:s?",
                "-c", "copy",
                "-avoid_negative_ts", "make_zero",
                
                "-mpegts_flags", "initial_discontinuity+resend_headers",
                
                "-f", "mpegts",
                "-flush_packets", "1",
                "pipe:1"
            ])
            
            try:
                self.proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, bufsize=FFMPEG_BUFFER)
                self.last_data_time = time.time()

                def log_err():
                    try:
                        for line in iter(self.proc.stderr.readline, b''):
                            line_str = line.decode('utf-8', 'ignore').strip()
                            if line_str: log.error(f"FFMPEG {self.key}: {line_str}")
                    except: pass
                threading.Thread(target=log_err, daemon=True).start()

                while self.running:
                    if time.time() - self.last_data_time > DATA_TIMEOUT:
                        log.warning(f" FFmpeg u {self.key} zamrzl. Zkouším jiný zdroj.")
                        break

                    chunk = self.proc.stdout.read(FFMPEG_BUFFER)
                    if not chunk: 
                        break

                    self.last_data_time = time.time()
                    
                    with stream_mgr.lock:
                        self.recent_chunks.append(chunk)
                        for q in list(self.client_queues):
                            try:
                                q.put_nowait(chunk)
                            except queue.Full:
                                try: q.get_nowait(); q.put_nowait(chunk)
                                except: pass

            except Exception as e:
                log.error(f" FFmpeg chyba {self.key}: {e}")
            finally:
                if self.proc:
                    try: self.proc.kill()
                    except: pass
            
            if self.running:
                log.warning(f" Zdroj {url.split('://')[0]} vypadl. Restart...")
                time.sleep(1)
                url_idx = (url_idx + 1) % len(self.urls)
                with stream_mgr.lock:
                    self.recent_chunks.clear()

class StreamManager:
    def __init__(self):
        self.streams: Dict = {}
        self.cooldowns: Dict[str, float] = {}
        self.lock = threading.RLock()
        self.start_time = time.time()
        threading.Thread(target=self._janitor_loop, daemon=True).start()

    def get_best_urls(self, source_id: str, channel_id: str) -> List[str]:
        if config.data.get('fallback_mode'):
            return [FALLBACK_URL]
        source_urls = [u.format(channel_id=channel_id) for u in config.data['sources'].get(source_id, [])]
        if len(source_urls) > 1:
            random.shuffle(source_urls)
        if config.data.get('auto_fallback', True) and FALLBACK_URL not in source_urls:
            source_urls.append(FALLBACK_URL)
        return source_urls

    def get_or_create_stream(self, key: str, source_id: str, channel_id: str) -> Tuple:
        with self.lock:
            if key in self.streams:
                return self.streams[key], False
                
            urls = self.get_best_urls(source_id, channel_id)
            stream = ActiveFFmpegStream(key, urls)
            self.streams[key] = stream
            return stream, True

    def _janitor_loop(self):
        while True:
            time.sleep(3)
            now = time.time()
            with self.lock:
                for key in list(self.streams.keys()):
                    stream = self.streams[key]
                    if stream.clients == 0 and (now - stream.last_client_time) > CLEANUP_DELAY:
                        log.info(f" Úklid. Vypínám FFmpeg pro {key} (bez klientů {CLEANUP_DELAY}s).")
                        stream.stop()
                        del self.streams[key]

    def cleanup_stream(self, key: str, delay: int = 0):
        with self.lock:
            if key in self.streams:
                self.streams[key].stop()
                del self.streams[key]

stream_mgr = StreamManager()

# ============================================================================
# BACKGROUND MONITORS (TVH CHECK)
# ============================================================================

def monitor_tvh():
    log.info("TVHeadend monitor started")
    missing_since = {}
    
    while True:
        try:
            tvh = config.data.get('tvheadend', {})
            active = set()
            if tvh.get('url'):
                r = requests.get(f"{tvh['url']}/api/status/subscriptions",
                               auth=HTTPDigestAuth(tvh.get('username', ''), tvh.get('password', '')), timeout=5)
                if r.status_code == 200:
                    for sub in r.json().get('entries', []):
                        url = sub.get('server_url', '')
                        if url:
                            parsed_url = urlparse(url)
                            path = parsed_url.path.strip('/')
                            parts = path.split('/')
                            if len(parts) >= 2: 
                                source_id = parts[-2]
                                channel_id = parts[-1].rsplit('.', 1)[0]
                                active.add(f"{source_id}:{channel_id}")
                            
            with stream_mgr.lock:
                for key in list(stream_mgr.streams.keys()):
                    stream = stream_mgr.streams[key]
                    
                    if stream.clients > 0:
                        if key in missing_since:
                            del missing_since[key]
                        continue

                    if key not in active:
                        if key not in missing_since:
                            missing_since[key] = time.time()
                        elif (time.time() - missing_since[key]) > TVH_GRACE_PERIOD:
                            log.info(f" TVH Monitor: Záchranný úklid pro mrtvý stream {key}.")
                            stream_mgr.cleanup_stream(key)
                    else:
                        if key in missing_since:
                            del missing_since[key]
                            
                for k in list(missing_since.keys()):
                    if k not in stream_mgr.streams:
                        del missing_since[k]
                        
        except Exception:
            pass 
            
        with stream_mgr.lock:
            expired = [k for k, v in stream_mgr.cooldowns.items() if time.time() > v]
            for k in expired: del stream_mgr.cooldowns[k]
            
        time.sleep(TVH_CHECK_INTERVAL)

# ============================================================================
# HTTP PROXY SERVER
# ============================================================================

class ProxyHandler(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args): pass
    def log_error(self, format, *args): pass 

    def do_GET(self):
        client_ip = self.client_address[0]
        if not security.is_allowed(client_ip):
            self.send_error(403, "Forbidden")
            return
            
        parts = self.path.strip("/").split("/")
        if not parts or parts == [""]:
            self.send_response(200); self.end_headers(); self.wfile.write(b"IPTV Proxy V4.2")
            return
            
        if len(parts) < 2:
            self.send_error(400); return
            
        source_id = parts[0]
        channel_id = parts[1].rsplit('.', 1)[0]
        key = f"{source_id}:{channel_id}"
        
        with stream_mgr.lock:
            if key in stream_mgr.cooldowns:
                rem = int(stream_mgr.cooldowns[key] - time.time())
                if rem > 0: self.send_error(503, f"Cooldown: {rem}s"); return

        if source_id not in config.data['sources'] and not config.data.get('fallback_mode'):
            self.send_error(404); return
            
        stream, _ = stream_mgr.get_or_create_stream(key, source_id, channel_id)
        with stream_mgr.lock:
            client_queue = stream.add_client()
            
        wait = 0
        while wait < STARTUP_TIMEOUT:
            if not stream.running:
                break
            if client_queue.qsize() > 0:
                break
            time.sleep(0.1)
            wait += 0.1
            
        if not stream.running or client_queue.empty():
            self.send_error(503, "Zdroj neposkytl data.")
            with stream_mgr.lock:
                stream.remove_client(client_queue)
            return

        self.send_response(200)
        self.send_header("Content-Type", "video/MP2T")
        self.send_header("Connection", "keep-alive")
        self.end_headers()
        
        try:
            while True:
                try:
                    chunk = client_queue.get(timeout=30)
                except queue.Empty:
                    break 
                
                if chunk is None: 
                    break 
                    
                self.wfile.write(chunk)
                
        except Exception: 
            pass 
        finally:
            with stream_mgr.lock:
                if key in stream_mgr.streams:
                    stream_mgr.streams[key].remove_client(client_queue)

# ============================================================================
# FLASK API
# ============================================================================

app = Flask(__name__)
app.config['JSON_SORT_KEYS'] = False
logging.getLogger('werkzeug').setLevel(logging.ERROR)

@app.before_request
def restrict_ips():
    if not security.is_allowed(request.remote_addr):
        return jsonify({'error': 'Forbidden'}), 403

@app.route('/api/status')
def api_status():
    with stream_mgr.lock:
        s = []
        for key, stream in stream_mgr.streams.items():
            s.append({
                'key': key,
                'clients': stream.clients,
                'age': int(time.time() - stream.created),
                'url': stream.urls[0] if stream.urls else 'N/A'
            })

    return jsonify({
        'streams': s,
        'total_streams': len(s),
        'total_clients': sum(st.clients for st in stream_mgr.streams.values()),
        'uptime': int(time.time() - stream_mgr.start_time)
    })

@app.route('/api/config', methods=['GET', 'POST'])
def api_config():
    if request.method == 'POST':
        config.update(request.get_json() or {})
        return jsonify({'success': True})
    return jsonify(config.data)

@app.route('/api/sources', methods=['GET', 'POST'])
def api_sources():
    if request.method == 'POST':
        data = request.get_json()
        if data and 'sources' in data:
            config.update({'sources': data['sources']})
            return jsonify({'success': True})
    return jsonify({'sources': config.data.get('sources', {})})

@app.route('/api/sources/<source_id>', methods=['DELETE'])
def api_delete_source(source_id):
    if source_id in config.data['sources']:
        del config.data['sources'][source_id]
        config.save()
    return jsonify({'success': True})

@app.route('/api/streams/<path:key>', methods=['DELETE'])
def api_kill_stream(key):
    stream_mgr.cleanup_stream(key)
    return jsonify({'success': True})

@app.route('/api/fallback', methods=['POST'])
def api_toggle_fallback():
    config.data['fallback_mode'] = not config.data.get('fallback_mode', False)
    config.save()
    return jsonify({'fallback_mode': config.data['fallback_mode']})

@app.route('/api/auto-fallback', methods=['POST'])
def api_toggle_auto_fallback():
    config.data['auto_fallback'] = not config.data.get('auto_fallback', True)
    config.save()
    return jsonify({'auto_fallback': config.data['auto_fallback']})

@app.route('/api/xtream/providers')
def api_xtream_providers():
    return jsonify({'providers': config.data.get('xtream_providers', {})})

@app.route('/api/xtream/providers/<provider_id>/info')
def api_xtream_info(provider_id):
    providers = config.data.get('xtream_providers', {})
    if provider_id not in providers: return jsonify({'error': 'Not found'}), 404
    info = XtreamAPI.call(providers[provider_id])
    return jsonify({'info': info} if info else {'error': 'Failed'})

@app.route('/api/xtream/providers/<provider_id>/categories')
def api_xtream_categories(provider_id):
    providers = config.data.get('xtream_providers', {})
    if provider_id not in providers: return jsonify({'error': 'Not found'}), 404
    cat_type = request.args.get('type', 'live')
    return jsonify({'categories': XtreamAPI.get_categories(providers[provider_id], cat_type)})

@app.route('/api/xtream/providers/<provider_id>/streams')
def api_xtream_streams(provider_id):
    providers = config.data.get('xtream_providers', {})
    if provider_id not in providers: return jsonify({'error': 'Not found'}), 404
    cat_id = request.args.get('category_id')
    stream_type = request.args.get('type', 'live')
    return jsonify({'streams': XtreamAPI.get_streams(providers[provider_id], cat_id, stream_type)})

@app.route('/api/xtream/providers/<provider_id>/epg/<stream_id>')
def api_xtream_epg(provider_id, stream_id):
    providers = config.data.get('xtream_providers', {})
    if provider_id not in providers: return jsonify({'error': 'Not found'}), 404
    epg_type = request.args.get('type', 'short')
    if epg_type == 'full':
        return jsonify({'epg': XtreamAPI.get_full_epg(providers[provider_id], stream_id)})
    return jsonify({'epg': XtreamAPI.get_epg(providers[provider_id], stream_id)})

@app.route('/api/xtream/providers/<provider_id>/vod/<vod_id>/info')
def api_xtream_vod_info(provider_id, vod_id):
    providers = config.data.get('xtream_providers', {})
    if provider_id not in providers: return jsonify({'error': 'Not found'}), 404
    return jsonify({'info': XtreamAPI.get_vod_info(providers[provider_id], vod_id)})

@app.route('/api/xtream/providers/<provider_id>/series/<series_id>/info')
def api_xtream_series_info(provider_id, series_id):
    providers = config.data.get('xtream_providers', {})
    if provider_id not in providers: return jsonify({'error': 'Not found'}), 404
    return jsonify({'info': XtreamAPI.get_series_info(providers[provider_id], series_id)})

@app.route('/api/xtream/providers/<provider_id>/xmltv.php')
def api_xtream_xmltv(provider_id):
    providers = config.data.get('xtream_providers', {})
    if provider_id not in providers: return jsonify({'error': 'Not found'}), 404
    provider = providers[provider_id]

    xmltv_url = f"{provider['url']}/xmltv.php?username={provider['username']}&password={provider['password']}"
    proxy_url = config.get_random_proxy()
    req_proxies = {"http": proxy_url, "https": proxy_url} if proxy_url else None

    try:
        r = requests.get(xmltv_url, stream=True, proxies=req_proxies, verify=False)
        return Response(r.iter_content(chunk_size=8192), content_type=r.headers.get('Content-Type', 'text/xml'))
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/xtream/providers/<provider_id>/category/<category_id>/playlist.m3u')
def api_xtream_playlist(provider_id, category_id):
    providers = config.data.get('xtream_providers', {})
    if provider_id not in providers: return jsonify({'error': 'Not found'}), 404

    provider = providers[provider_id]
    source_id = provider.get('source_id')
    if not source_id: return jsonify({'error': 'Missing source_id'}), 500

    url = f"{provider['url']}/player_api.php"
    p = {'username': provider['username'], 'password': provider['password'], 'action': 'get_live_streams', 'category_id': category_id}
    try:
        r = requests.get(url, params=p, timeout=10, verify=False)
        streams_data = r.json() if r.status_code == 200 else []
    except:
        streams_data = []

    if not streams_data: return jsonify({'error': 'No streams found'}), 404

    proxy_host = request.host.split(':')[0]
    m3u_lines = ["#EXTM3U"]

    for stream in streams_data:
        stream_id = stream.get('stream_id') or stream.get('id')
        stream_name = stream.get('name', f'Stream {stream_id}')
        epg_id = stream.get('epg_channel_id', '')

        m3u_lines.append(f'#EXTINF:-1 tvg-id="{epg_id}" tvg-name="{stream_name}",{stream_name}')
        m3u_lines.append(f"http://{proxy_host}:{PROXY_PORT}/{source_id}/{stream_id}.ts")

    return Response("\n".join(m3u_lines), mimetype='audio/mpegurl', headers={
        "Content-Disposition": f"attachment; filename=\"playlist_{category_id}.m3u\""
    })

@app.route('/')
def serve_ui():
    return jsonify({'status': 'Proxy V4.2 running', 'version': '4.2.0'})

# ============================================================================
# MAIN
# ============================================================================

def run_proxy():
    socketserver.TCPServer.allow_reuse_address = True
    with socketserver.ThreadingTCPServer(("", PROXY_PORT), ProxyHandler) as server:
        log.info(f"Proxy V4.2 running on :{PROXY_PORT}")
        server.serve_forever()

def main():
    print("=" * 60)
    print("IPTV Stream Proxy Manager V4.2")
    print("=" * 60)
    threading.Thread(target=monitor_tvh, daemon=True).start()
    threading.Thread(target=app.run, kwargs={'host':'0.0.0.0', 'port':API_PORT, 'debug':False, 'threaded':True}, daemon=True).start()
    try: run_proxy()
    except KeyboardInterrupt: pass
    except Exception as e: log.error(f"Fatal: {e}"); sys.exit(1)

if __name__ == "__main__":
    main()
