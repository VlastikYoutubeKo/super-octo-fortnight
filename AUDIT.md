# Audit & návrhy – IPTV Stream Proxy Manager

Datum auditu: 2026-08-29 · Rozsah: celý repozitář (`src/*.go`, UI `src/index.html`, `epg.py`, skripty, konfigurace)

Cílem bylo projít Xtream proxy pro TVHeadend, zjistit, co chybí, co by mohlo být součástí,
a rovnou to nejhodnotnější opravit/doplnit. Vše, co je níže v sekci **„Opraveno / přidáno“**,
je už implementované v tomto commitu.

---

## 1. Co projekt umí (rekapitulace)

- **Proxy streamů** na portu `9000`: multi-source failover, auto-fallback na náhradní video,
  hijack TCP socketu a zápis čistého MPEG-TS (obejití chunked encoding → TVHeadend se neodpojuje),
  umělý limit ~10 MB/s na klienta, detekce „idle scan burstů“ od TVH.
- **Xtream integrace** na portu `9005`: prohlížeč kategorií/kanálů, generování M3U playlistů
  (per-kategorie i bulk), filtrovaný EPG XML z `xmltv.php` providera, auto-detekce Xtream providerů
  z `sources` URL.
- **AI EPG párování**: Gemini + Ollama (multi-model fallback), přísný prompt, lokální fuzzy
  kandidáti, validace odpovědí, offline fallback.
- **EPG údržba**: denní janitor (05:15), sanitace `epg_mapping.json`, vynucené relinky kanálů
  v TVHeadend přes `/api/idnode/save`, `epgauto=false` na dotčených kanálech, re-import EPG přes
  `/api/epggrab/internal/rerun`, **piny** (`epg_pins.json`) – lidská rozhodnutí, která janitor nesmí vrátit.
- **Další**: statistiky sledování, Webshare bandwith monitoring, IP whitelist na stream portu,
  multi-API-key rotace, M3U name sanitizer, EPG grabber `epg.py` (46 zdrojů + provider EPG fallback).

---

## 2. Opraveno / přidáno v tomto commitu

### 2.1 Bezpečnost (největší díra)
- **API bez autentizace.** Kdokoli v síti, kdo dosáhl na port `9005`, si mohl přečíst
  `GET /api/config` (hesla upstream providerů, Gemini/Webshare klíče, proxy s hesly) a ovládat vše:
  měnit sources, mazat streamy, přepisovat EPG mapování, spouštět údržbu.
  → Přidán **`api_token`** do `config.json`. Když je nastaven, každý `/api/*` request z jiného
  stroje musí nést token (`X-API-Token`, `Authorization: Bearer`, nebo `?token=` pro playlist URL).
  Localhost je důvěryhodný (UI na stejném stroji, EPG janitor, TVHeadend na stejném boxu).
  Porovnání probíhá v konstantním čase (`crypto/subtle`).
- **Únik hesel přes `/api/status`** – pole `url` obsahovalo `http://user:pass@host/...`.
  → Nyní redigováno na `http://user:***@host/...`.
- **Spoofing X-Forwarded-For obcházel IP whitelist.** Klient mohl poslat
  `X-Forwarded-For: 127.0.0.1` a whitelist ho pustil.
  → Nový config **`trust_proxy_headers`** (default `false`): brát peer ze socketu; zapnout jen za
  reverzní proxy (nginx…).
- **CORS `*`** → nový config **`cors_origin`** (prázdný = `*`, zpětně kompatibilní) pro omezení,
  která webová stránka smí volat API.

### 2.2 Spolehlivost
- **Neatomické zápisy JSON.** `config.json`, `epg_mapping.json`, `epg_pins.json`,
  `names_cache.json`, `channel_stats.json` se psaly přes `os.Create` – pád (OOM kill, výpadek)
  uprostřed zápisu = zničený soubor. (CHANGELOG už jednou dokumentuje obnovu zálohy poté, co se
  mapping soubor „vyprázdnil“.)
  → Všech pět souborů se nyní píše přes **`atomicWriteFile`** (temp soubor ve stejné složce +
  fsync + rename).
- **Zastaralý cache kanálů se nikdy nečistil.** `refreshChannelNamesLoop` jen appendoval do
  `ChannelNames`/`CurrentIDs` – kanály, které provider zrušil/přejmenoval, zůstaly v cache
  navždy a každý request zkoušel mrtvé alternativní ID („ID Discovery“).
  → Při každém úspěšném refreshe se cache daného zdroje **přestaví z živých dat**.
- **Nesprávné parsování IPv6** u whitelistu (naivní `LastIndex(":")`) → `net.SplitHostPort`.
- **Mrtvý kód**: `shuffle()` (deprecated `rand.Seed`, nikde se nevolal) a `checkUrlHealth()` odstraněny.

### 2.3 Drobnosti
- **`GET /api/health`** – `{status: ok, uptime, version}` pro monitoring (otevřený i s tokenem).
- **Validace configu při startu** – warningy: chybějící `{channel_id}` v URL source, prázdné `sources`.
- **`fmt.Printf` → `log.Printf`** v M3U API (timestampované logy).
- **UI**: pole API token / CORS origin / trust proxy v Settings; prompt pro token při 401;
  playlist downloady automaticky přidávají `?token=`.

---

## 3. Co by ještě mohlo být součástí (návrhy na příště)

Řazeno podle poměru přínos/riziko. Nic z toho není v tomto commitu, je to zásobník nápadů:

1. **Ollama/Gemini nastavení v UI.** `POST /api/config` neukládá `ollama_api_url`, `ollama_api_key`,
   `ollama_model` (a `xtream_providers`) a Settings tab je neobsahuje – dnes se dají měnit jen ručně
   v `config.json`. Dát je do UI by bylo přirozené.
2. **Konfigurovatelné porty** (9000/9005 jsou natvrdo v `main.go`) – env proměnné
   `PROXY_PORT`/`API_PORT` nebo flagy. Usnadní to provoz ve více instancích / za nginxem.
3. **VOD/Series playlisty.** `getStreams` už umí `vod`/`series`, ale M3U endpointy generují jen live.
   Stačil by parametr `?type=vod|series` (pozor na `series_id` místo `stream_id` u sérií).
4. **Automatické čištění M3U cache** – `m3u_cache_*.m3u` se maže jen při purge v rámci údržby EPG;
   přidat TTL sweep (např. mazat starší než 48 h) by zabránilo pomalému růstu disku.
5. **Log rotation / log file** místo čistého stdout (systemd uživatele to možná řeší, ale hodí se
   dokumentovat).
6. **Rate limiting na `/api/xtream/providers/.../epg.xml`** – streamování 90MB XML z providera
   bez limitu může být zneužito (i lokálně).
7. **Webshare TotalGB z API** – kód tvrdě předpokládá limit 1 TB (`TotalGB = 1024.0`); Webshare API
   umí vrátit reálný plán, šlo by to dotáhnout.
8. **Testy na stream/proxy logiku** – existují jen `fuzzy_test.go` a `epg_pins_test.go`; failover,
   cooldowny a ID discovery nejsou pokryté.
9. **HTTPS** – API port běží čistě HTTP; za nginxem stačí, ale pro přímé vystavení by se hodila
   TLS podpora nebo aspoň dokumentace reverzní proxy.
10. **`epg.py`**: cesty (`/var/lib/tvheadend/...`, `/root/tvheadend_proxies/...`) jsou natvrdo –
    zobecnit na env proměnné by usnadnilo nasazení jinde.

---

## 4. Poznámky k provozu

- Build: `cd src && go build -o ../proxy_server *.go` (Go 1.24+). Sandbox, ve kterém probíhal tento
  audit, nemá nainstalovaný Go toolchain ani přístup k internetu pro jeho stažení, takže změny
  nebyly kompilovány – doporučuji před nasazením projít `go vet ./...` a `go test ./...`.
- Po zapnutí `api_token` si nezapomeňte přidat `?token=` i k playlist URL, které používá TVHeadend
  (pokud TVH neběží na stejném stroji jako proxy).
- `trust_proxy_headers: true` zapínejte JEN když proxy reálně sedí za reverzní proxy.
