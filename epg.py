#!/usr/bin/python3
# -*- coding: utf-8 -*-

import logging
import requests
import gzip
import sys
import os
import socket
import io
import json
import zlib
import xml.etree.ElementTree as ET
from logging.handlers import RotatingFileHandler
from datetime import datetime
from urllib.parse import quote

# --- Konfigurace ---
EPG_URLS = [
    "https://file.garden/aPznbQW_1zQPhcZE/dbepg.xml",
    "https://file.garden/ZC8Mzku7QDnuPZu9/disneychanneldb.xml",
    "https://iptv-epg.org/files/epg-cz.xml",
    "https://iptv-epg.org/files/epg-pl.xml",
    "https://epgshare01.online/epgshare01/epg_ripper_CZ1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_SK1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_PL1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_US2.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_DE1.xml.gz",
    "https://iptv-epg.org/files/epg-ru.xml",
    "https://epgshare01.online/epgshare01/epg_ripper_UK1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_FR1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_IT1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_AT1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_RS1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_IN1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_TR3.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_IN4.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_GR1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_TR1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_HR1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_ES1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_PT1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_BG1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_BE2.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_BR2.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_NL1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_ID1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_MY1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_LV1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_RO2.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_FI1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_HK1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_HU1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_VN1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_NZ1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_BR1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_SG1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_CY1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_CO1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_PH1.xml.gz",
    "https://epgshare01.online/epgshare01/epg_ripper_PH2.xml.gz",
    "https://webtv.cyn.cz/iptv/xmltv.xml"
]

# !!! ZDE VLOŽ URL SVÉHO WEBHOOKU !!!
WEBHOOK_URL = os.environ.get("EPG_DISCORD_WEBHOOK", "")  # set via env, never commit

# --- Cesty k souborům ---
HOME_DIR = os.path.expanduser('~')
LOG_FILE = os.path.join(HOME_DIR, "epg_update.log")
XMLTV_OUTPUT_FILE = "/var/lib/tvheadend/.xmltv/tv_grab_file.xmltv"
SOCKET_PATH = "/var/lib/tvheadend/epggrab/xmltv.sock"

# --- EPG od Xtream providerů (fallback) ---
# Providery bere z config.json proxy. Použije se pouze jako doplněk: kanál se přidá
# jen tehdy, když ho žádný ze zdrojů v EPG_URLS nepokrývá.
PROVIDER_EPG_ENABLED = True
PROXY_CONFIG_FILE = "/root/tvheadend_proxies/super-octo-fortnight/config.json"
PROVIDER_EPG_TIMEOUT = (15, 180)  # (connect, read) - xmltv.php má ~90 MB
# Pojistka pro pořady, které odkazují na kanál bez <channel> definice. Ve feedu jich
# je řádově tisíce; kdyby jich přišly statisíce, nebufferujeme je donekonečna.
PROVIDER_ORPHAN_LIMIT = 20000

# --- Nastavení logování ---
log_stream = io.StringIO()
try:
    log_formatter = logging.Formatter('%(asctime)s - %(levelname)s - %(message)s')

    file_handler = RotatingFileHandler(LOG_FILE, maxBytes=5*1024*1024, backupCount=1, encoding='utf-8')
    file_handler.setFormatter(log_formatter)

    console_handler = logging.StreamHandler(sys.stdout)
    console_handler.setFormatter(log_formatter)

    stream_handler = logging.StreamHandler(log_stream)
    stream_handler.setFormatter(log_formatter)

    logger = logging.getLogger()
    logger.setLevel(logging.INFO)
    logger.handlers.clear()
    logger.addHandler(file_handler)
    logger.addHandler(console_handler)
    logger.addHandler(stream_handler)
except Exception as e:
    print(f"FATAL: Nepodařilo se nastavit logování. Chyba: {e}")
    sys.exit(1)


def send_to_socket(data):
    """Odešle data do TVHeadend socketu."""
    try:
        logger.info(f"Odesílám data (velikost: {len(data) / 1024:.2f} KB) do TVHeadend socketu...")
        with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as s:
            s.connect(SOCKET_PATH)
            s.sendall(data)
            logger.info("Data byla úspěšně odeslána do socketu.")
            return True
    except socket.error as e:
        logger.error(f"Chyba socketu při odesílání: {e}")
        return False


def send_webhook(log_content, file_path, status):
    """Odešle výsledek běhu skriptu na zadaný webhook."""
    if not WEBHOOK_URL:
        logger.info("WEBHOOK_URL není nastavena, odeslání se přeskakuje.")
        return

    logger.info("Odesílám notifikaci na webhook...")

    log_limit = 1000
    if len(log_content) > log_limit:
        log_for_discord = f"{log_content[:log_limit // 2]}\n\n...\n\n{log_content[-(log_limit // 2):]}"
    else:
        log_for_discord = log_content

    try:
        payload = {
            "embeds": [{
                "title": f"EPG Update Report: {status}",
                "color": 3066993 if status == "SUCCESS" else 15158332,
                "fields": [
                    {"name": "EPG Soubor", "value": f"`{file_path}`", "inline": False},
                    {"name": "Log", "value": f"```\n{log_for_discord}\n```", "inline": False}
                ],
                "footer": {"text": "EPG Grabber"},
                "timestamp": datetime.utcnow().isoformat()
            }]
        }
        response = requests.post(WEBHOOK_URL, json=payload, timeout=10)
        response.raise_for_status()
        logger.info("Webhook notifikace úspěšně odeslána.")
    except requests.exceptions.RequestException as e:
        logger.error(f"Chyba při odesílání webhooku: {e}")


def load_provider_epg_sources(config_file=PROXY_CONFIG_FILE):
    """Načte Xtream providery z config.json proxy a vrátí [(název, xmltv.php URL)].

    Deduplikuje podle panelu (url), ne podle účtu: na jednom panelu bývá víc účtů,
    ale xmltv.php vrací stejné EPG. Bez toho bychom stahovali ~90 MB pro každý účet.
    """
    try:
        with open(config_file, encoding='utf-8') as f:
            cfg = json.load(f)
    except Exception as e:
        logger.warning(f"Nepodařilo se načíst {config_file}: {e}. Provider EPG se přeskakuje.")
        return []

    sources, seen = [], set()
    total = 0
    for prov in (cfg.get('xtream_providers') or {}).values():
        url = (prov.get('url') or '').rstrip('/')
        username = prov.get('username') or ''
        password = prov.get('password') or ''
        if not url or not username:
            continue
        total += 1
        if url in seen:
            continue
        seen.add(url)
        name = prov.get('name') or url
        sources.append((name, f"{url}/xmltv.php?username={quote(username)}&password={quote(password)}"))

    logger.info(f"Z {total} Xtream providerů zbylo po deduplikaci podle panelu {len(sources)} ke stažení.")
    return sources


def _iter_xml_chunks(response):
    """Vrací dekomprimované kusy těla odpovědi.

    Čte přes iter_content, ne přes response.raw - urllib3 raw po dosažení EOF sám
    zavře a další čtení skončí na "read of closed file", což utne konec feedu.
    """
    decompressor = None
    first = True
    for chunk in response.iter_content(65536):
        if not chunk:
            continue
        if first:
            first = False
            if chunk[:2] == b'\x1f\x8b':  # gzip tělo (ne Content-Encoding)
                decompressor = zlib.decompressobj(16 + zlib.MAX_WBITS)
        if decompressor is not None:
            chunk = decompressor.decompress(chunk)
            if not chunk:
                continue
        yield chunk
    if decompressor is not None:
        tail = decompressor.flush()
        if tail:
            yield tail


def merge_provider_epg(main_root, known_ids):
    """Doplní do main_root EPG z Xtream providerů.

    Provider EPG je záměrně až poslední v pořadí: přidá se pouze kanál, jehož id
    zatím žádný jiný zdroj nedodal, a k němu jeho pořady. Díky sdílenému known_ids
    tak druhý a třetí panel se stejným obsahem nepřidá nic navíc.

    Parsuje se průběžně přes XMLPullParser - xmltv.php má ~90 MB a nikdy se
    nenačítá do paměti celé.
    """
    if not PROVIDER_EPG_ENABLED:
        return 0

    sources = load_provider_epg_sources()
    if not sources:
        return 0

    ok_count = 0
    for name, url in sources:
        response = None
        try:
            logger.info(f"--- Stahuji provider EPG: {name} ---")
            response = requests.get(url, timeout=PROVIDER_EPG_TIMEOUT, stream=True)
            response.raise_for_status()

            added_channels = 0
            added_programmes = 0
            taken = set()  # id kanálů, které jsme z TOHOTO providera převzali
            orphans = []   # pořady, jejichž kanál zatím nikdo nedeklaroval
            orphans_dropped = 0
            source_root = None

            parser = ET.XMLPullParser(events=('start', 'end'))
            for chunk in _iter_xml_chunks(response):
                parser.feed(chunk)
                for event, element in parser.read_events():
                    if event == 'start':
                        if source_root is None:
                            source_root = element  # kořenový <tv>
                        continue

                    if element.tag == 'channel':
                        channel_id = element.get('id')
                        if channel_id and channel_id not in known_ids:
                            known_ids.add(channel_id)
                            taken.add(channel_id)
                            main_root.append(element)
                            added_channels += 1
                        else:
                            # Feed opakuje stejné kanály několikrát; duplicitu zahodíme.
                            element.clear()
                    elif element.tag == 'programme':
                        channel_id = element.get('channel')
                        if channel_id in taken:
                            main_root.append(element)
                            added_programmes += 1
                        elif channel_id and channel_id not in known_ids:
                            # Kanál zatím nikdo nedeklaroval - může přijít později ve feedu,
                            # nebo nepřijde vůbec. Rozhodneme se až na konci.
                            if len(orphans) < PROVIDER_ORPHAN_LIMIT:
                                orphans.append(element)
                            else:
                                orphans_dropped += 1
                                element.clear()
                        else:
                            # Kanál pokrývá jiný zdroj - jeho pořady nepřebíráme.
                            element.clear()
                    else:
                        continue

                    # Uvolní zpracované prvky. Co jsme si nechali, drží už main_root.
                    if source_root is not None:
                        source_root.clear()
            parser.close()

            # Pořady na kanály, které feed nikdy nedeklaroval: kanál dogenerujeme,
            # jinak by je TVHeadend zahodil. Pokud kanál mezitím dorazil, prostě ho použijeme.
            synthesized = 0
            for element in orphans:
                channel_id = element.get('channel')
                if channel_id not in taken:
                    if channel_id in known_ids:
                        element.clear()
                        continue
                    channel_el = ET.Element('channel', {'id': channel_id})
                    ET.SubElement(channel_el, 'display-name').text = channel_id
                    main_root.append(channel_el)
                    known_ids.add(channel_id)
                    taken.add(channel_id)
                    added_channels += 1
                    synthesized += 1
                main_root.append(element)
                added_programmes += 1

            ok_count += 1
            extra = f", z toho {synthesized} kanálů dogenerováno z osiřelých pořadů" if synthesized else ""
            if orphans_dropped:
                extra += f" ({orphans_dropped} osiřelých pořadů zahozeno, limit {PROVIDER_ORPHAN_LIMIT})"
            logger.info(
                f"Provider {name}: doplněno {added_channels} kanálů "
                f"a {added_programmes} pořadů, které jiné zdroje nemají{extra}."
            )
        except Exception as e:
            logger.error(f"Chyba při zpracování provider EPG {name}: {e}")
        finally:
            if response is not None:
                response.close()

    return ok_count


def main():
    """Hlavní funkce skriptu."""
    status = "SUCCESS"
    try:
        logger.info("=== Spouštím kompletní EPG refresh ===")
        is_root = (os.geteuid() == 0)
        if is_root:
            logger.info("Skript je spuštěn jako root. Data budou také odeslána do socketu.")
        else:
            logger.info("Skript není spuštěn jako root. Odeslání do socketu bude přeskočeno.")

        main_root = ET.Element('tv')
        success_count = 0

        prefix_to_remove = 'https://sledovanitv.cz'

        logger.info("Zpracovávám jednotlivé zdroje...")
        for url in EPG_URLS:
            try:
                logger.info(f"--- Stahuji: {url} ---")
                response = requests.get(url, timeout=30)
                response.raise_for_status()

                content = response.content

                # Detekce gzip podle magic bytes
                if content[:2] == b'\x1f\x8b':
                    xml_data = gzip.decompress(content)
                    logger.info(f"Zdroj je gzip archiv, úspěšně rozbaleno ({len(xml_data) / 1024:.2f} KB).")
                else:
                    xml_data = content
                    logger.info(f"Zdroj je běžný XML (ne gzip). Velikost: {len(xml_data) / 1024:.2f} KB.")

                source_root = ET.fromstring(xml_data)
                for element in source_root:
                    if element.tag in ['channel', 'programme']:
                        # Oprava URL ikon
                        for icon in element.findall('.//icon'):
                            src = icon.get('src')
                            if src and src.startswith(prefix_to_remove):
                                new_src = src.replace(prefix_to_remove, '', 1)
                                icon.set('src', new_src)
                                logger.info(f"Opravena URL ikony z '{src}' na '{new_src}'")
                        main_root.append(element)
                success_count += 1
            except Exception as e:
                logger.error(f"Chyba při zpracování zdroje {url}: {e}")

        
        # --- UK TV GUIDE API INJECTION ---
        logger.info("--- Stahuji: api-2.tvguide.co.uk ---")
        try:
            import datetime
            import json
            import urllib.request
            today = datetime.datetime.now()
            dates = [(today + datetime.timedelta(days=i)).strftime("%Y-%m-%d") for i in range(-1, 3)]
            channels_added = set()
            for d in dates:
                url = f"https://api-2.tvguide.co.uk/listings?platform=popular&region=&view=grid&date={d}&hour=0&details=false"
                req = urllib.request.Request(url, headers={'User-Agent': 'Mozilla/5.0'})
                try:
                    with urllib.request.urlopen(req) as response:
                        data = json.loads(response.read().decode())
                        for ch in data:
                            ch_id = ch.get('slug', '')
                            if not ch_id: continue
                            if ch_id not in channels_added:
                                channel_el = ET.SubElement(main_root, "channel", {"id": ch_id})
                                display_name = ET.SubElement(channel_el, "display-name")
                                display_name.text = ch.get('title', ch_id)
                                if 'logo_url' in ch:
                                    ET.SubElement(channel_el, "icon", {"src": ch['logo_url']})
                                channels_added.add(ch_id)
                            for prog in ch.get('schedules', []):
                                try:
                                    start_dt = datetime.datetime.strptime(prog['start_at'], "%Y-%m-%dT%H:%M:%S.%fZ")
                                    duration_mins = prog.get('duration', 0)
                                    stop_dt = start_dt + datetime.timedelta(minutes=duration_mins)
                                    start_str = start_dt.strftime("%Y%m%d%H%M%S +0000")
                                    stop_str = stop_dt.strftime("%Y%m%d%H%M%S +0000")
                                    prog_el = ET.SubElement(main_root, "programme", {"start": start_str, "stop": stop_str, "channel": ch_id})
                                    title_el = ET.SubElement(prog_el, "title")
                                    title_el.text = prog.get('title', 'Unknown')
                                    if 'image_url' in prog and prog['image_url']:
                                        ET.SubElement(prog_el, "icon", {"src": prog['image_url']})
                                except Exception as inner_e:
                                    continue
                except Exception as inner_req_e:
                    logger.error(f"Chyba při stahování {url}: {inner_req_e}")
            success_count += 1
            logger.info("UK TV Guide API (api-2) data byla úspěšně přidána.")
        except Exception as api_e:
            logger.error(f"Chyba při zpracování UK TV Guide API: {api_e}")
        # --- END UK TV GUIDE API INJECTION ---

        
        # --- SKY UK API INJECTION ---
        logger.info("--- Stahuji: awk.epgsky.com (Sky UK) ---")
        try:
            import datetime
            import json
            import urllib.request
            import concurrent.futures
            
            def fetch_json(url):
                req = urllib.request.Request(url, headers={'User-Agent': 'Mozilla/5.0'})
                try:
                    with urllib.request.urlopen(req, timeout=10) as response:
                        return json.loads(response.read().decode())
                except:
                    return None

            channels_data = fetch_json("https://awk.epgsky.com/hawk/linear/services/4101/1")
            if channels_data and 'services' in channels_data:
                today = datetime.datetime.utcnow()
                dates = [(today + datetime.timedelta(days=i)).strftime("%Y%m%d") for i in range(-1, 3)]
                
                def fetch_channel_schedule(ch):
                    sid = ch.get('sid')
                    name = ch.get('t', sid)
                    if not sid: return None
                    
                    channel_node = ET.Element("channel", {"id": name})
                    display_name = ET.SubElement(channel_node, "display-name")
                    display_name.text = name
                    
                    programs = []
                    for d in dates:
                        sched_url = f"https://awk.epgsky.com/hawk/linear/schedule/{d}/{sid}"
                        sched_data = fetch_json(sched_url)
                        if sched_data and 'schedule' in sched_data:
                            for schedule in sched_data['schedule']:
                                if schedule.get('sid') == sid and 'events' in schedule:
                                    for event in schedule['events']:
                                        try:
                                            start_ts = event.get('st')
                                            duration = event.get('d', 0)
                                            if not start_ts: continue
                                            
                                            start_dt = datetime.datetime.utcfromtimestamp(start_ts)
                                            stop_dt = start_dt + datetime.timedelta(seconds=duration)
                                            
                                            start_str = start_dt.strftime("%Y%m%d%H%M%S +0000")
                                            stop_str = stop_dt.strftime("%Y%m%d%H%M%S +0000")
                                            
                                            prog_el = ET.Element("programme", {"start": start_str, "stop": stop_str, "channel": name})
                                            title_el = ET.SubElement(prog_el, "title")
                                            title_el.text = event.get('t', 'Unknown')
                                            
                                            desc = event.get('sy')
                                            if desc:
                                                desc_el = ET.SubElement(prog_el, "desc")
                                                desc_el.text = desc
                                                
                                            if 'programmeuuid' in event:
                                                img_url = f"https://images.metadata.sky.com/pd-image/{event['programmeuuid']}/16-9/640"
                                                ET.SubElement(prog_el, "icon", {"src": img_url})
                                                
                                            programs.append(prog_el)
                                        except:
                                            continue
                    return (channel_node, programs)

                logger.info(f"Nalezeno {len(channels_data['services'])} Sky UK kanálů, stahuji programy asynchronně...")
                
                with concurrent.futures.ThreadPoolExecutor(max_workers=20) as executor:
                    results = executor.map(fetch_channel_schedule, channels_data['services'])
                    
                for result in results:
                    if result:
                        ch_node, progs = result
                        main_root.append(ch_node)
                        for p in progs:
                            main_root.append(p)
                            
                success_count += 1
                logger.info("Sky UK API data byla úspěšně přidána.")
        except Exception as api_e:
            logger.error(f"Chyba při zpracování Sky UK API: {api_e}")
        # --- END SKY UK API INJECTION ---

        # --- PROVIDER (XTREAM) EPG INJECTION ---
        # Až tady, aby šlo jen o doplnění kanálů, které předchozí zdroje nepokrývají.
        known_ids = {el.get('id') for el in main_root if el.tag == 'channel' and el.get('id')}
        logger.info(f"Po hlavních zdrojích máme {len(known_ids)} kanálů. Doplňuji EPG od providerů...")
        success_count += merge_provider_epg(main_root, known_ids)
        # --- END PROVIDER EPG INJECTION ---

        if success_count == 0:
            logger.error("Nepodařilo se stáhnout žádná EPG data. Soubor nebude aktualizován.")
            status = "FAILURE"
            return

        logger.info(f"Spojeno EPG z {success_count} zdrojů ({len(EPG_URLS)} XMLTV URL + API + providery). Připravuji finální soubor...")

        final_tree = ET.ElementTree(main_root)
        output_dir = os.path.dirname(XMLTV_OUTPUT_FILE)
        os.makedirs(output_dir, exist_ok=True)

        final_tree.write(XMLTV_OUTPUT_FILE, encoding='UTF-8', xml_declaration=True)
        logger.info(f"Finální EPG soubor byl úspěšně uložen do: {XMLTV_OUTPUT_FILE}")

        if is_root:
            with open(XMLTV_OUTPUT_FILE, 'rb') as f:
                final_xml_bytes = f.read()
            send_to_socket(final_xml_bytes)

    except Exception as e:
        logger.error(f"Došlo k závažné chybě v průběhu skriptu: {e}", exc_info=True)
        status = "FAILURE"
    finally:
        logger.info(f"=== EPG refresh dokončen se statusem: {status}. ===")
        send_webhook(log_stream.getvalue(), XMLTV_OUTPUT_FILE, status)


if __name__ == "__main__":
    main()
