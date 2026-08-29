# Changelog

All notable changes to the **IPTV Stream Proxy Manager** project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- **Optional API authentication (`api_token` in config.json):** the management API (`:9005`) used to be wide open — anyone who could reach the port could read `GET /api/config` (upstream credentials, Gemini/Webshare keys) and flip settings. When a token is configured, every `/api/*` request from another machine must present it (`X-API-Token` header, `Authorization: Bearer <token>`, or `?token=<token>` for playlist URLs). Loopback requests stay trusted (local UI, EPG janitor, TVHeadend on the same box). The UI prompts for the token once and stores it in localStorage. Constant-time comparison; `/api/health` stays open for monitoring.
- **`cors_origin` config:** restrict which website may call the API from a browser (empty = `*`, backwards compatible).
- **`trust_proxy_headers` config (default `false`):** the IP allowlist previously trusted `X-Forwarded-For` from any client, so a remote attacker could spoof `X-Forwarded-For: 127.0.0.1` and bypass `allowed_ips` entirely. Now the socket peer is authoritative unless the proxy is explicitly behind a reverse proxy.
- **`GET /api/health`:** `status: ok` + uptime + version, for uptime monitors.
- **Startup config validation:** warns about sources whose URLs lack the `{channel_id}` placeholder and about an empty `sources` block.

### Fixed
- **Upstream credentials leaked via `/api/status`:** the `url` field of each stream carried `http://user:pass@host/...`; it is now redacted to `http://user:***@host/...`.
- **Crash could corrupt JSON state files:** `config.json`, `epg_mapping.json`, `epg_pins.json`, `names_cache.json` and `channel_stats.json` were written in place (`os.Create`), so an OOM-kill or power loss mid-write destroyed them (the EPG mapping file was gutted once before and had to be restored from backup). All five now go through `atomicWriteFile` (same-dir temp + fsync + rename).
- **Stale channel-ID cache never pruned:** `refreshChannelNamesLoop` only appended to `ChannelNames`/`CurrentIDs`, so streams dropped or renamed by the provider left dead IDs in the cache forever and every request probed them via "ID Discovery". Each successful refresh now rebuilds the source's cache entries from live data.
- **Broken IPv6 client parsing:** the allowlist stripped the client port with a naive `LastIndex(":")`, which mangles bare IPv6 addresses; `net.SplitHostPort` is used instead.
- **Deprecated `rand.Seed` dead code** (`shuffle()` in stream.go, `checkUrlHealth()` in proxy.go) removed.

### Changed
- The Settings tab of the Web UI gained fields for API token, CORS origin and trust-proxy headers; playlist download links append `?token=` when a token is stored.
- **Pinned EPG mappings (`epg_pins.json`, `src/epg_pins.go`):** curated channel→ID overrides that the daily janitor must never undo. `sanitizeMappings` deletes or re-points any mapping whose ID carries words the channel name lacks (`ExtraIDWords`), which is right almost always but wrong for channels that share one broadcast slot under a combined ID. Pins are re-asserted at startup and at every maintenance run, and skipped by the sanitizer. A pin whose ID is not loaded in TVHeadend is reported and ignored, never written into the mapping (invariant 2 still holds). API: `GET /api/epg/pins`, `PUT /api/epg/pin`, `DELETE /api/epg/pin`.
- **`tools/epg_gap_audit.py`:** finds channels mapped to a guide that only covers part of the day while another ID carries the same schedule for a fuller day. Uses evidence, not name heuristics: intra-day gaps (<20 h/day) plus ≥60% exact `(start, title)` agreement with the fuller ID. Found 57 candidates, incl. the near-empty Austrian ripper (~3-4 h/day of guide on ~24 `AT:` channels) and `Strike.TV.cz` (14.5 h/day → `STRIKE.TV.HD.cz`, 24 h/day, 99% identical).

### Fixed
- **CS FILM showed no overnight guide:** CS Film timeshares its slot with CS Horror (22:00-06:00), but it was mapped to `CS.Film.cz`, whose guide only covers 06:00-22:00 — the night was simply blank. Re-pointed (and pinned) to `CS.Film-CS.Horror.HD.sk`, the combined 24 h feed already present in the EPG universe, which carries the same schedule on the same `+0200` clock plus the horror block. `CSFILM,MINI.cz` was rejected as an alternative: it fills the night with one 6-hour filler block.
- **TVHeadend relinks stayed invisible for up to 12 hours:** TVHeadend attaches broadcasts to channels *at import time*, so changing a channel's EPG source does not move any events — the guide keeps showing the old source until a grabber re-reads the XMLTV (its cron is `4 */12 * * *`). Maintenance now calls `POST /api/epggrab/internal/rerun` (`rerun=1`) whenever it changed a link, so a relink reaches the guide immediately instead of at the next 00:04/12:04 run.
- **Wrong EPG IDs assigned to channels:** Reworked candidate generation in `fuzzy.go`, which was handing the AI garbage options and mapping channels to unrelated programme data. An audit found 3,666 of 5,045 stored mappings were wrong. Four independent causes were fixed:
  - **Country crossing:** nothing tied a channel's country prefix to the EPG ID's country suffix, so `CZ: AMC` was mapped to `AMC.pl` and `DE: 4 BLOCKS` to `Canal.4.de.Costa.Rica.cr` (the German `DE` prefix matching the Spanish word "de"). Candidates from a foreign country are now dropped whenever the channel's own country has a match.
  - **Brand bleed:** a brand token could match any word it was a prefix of, mapping `DISCO POLO` to `Discovery` and `HAUS DES GELDES` to `Schaffhauser Fernsehen`. A brand may now extend by at most 2 characters.
  - **Diacritics:** `TVN FABULA` resolved to `TVN.24.pl` and `TVP3 WROCLAW` to `TVP3.Opole.cz`, because the EPG spells them `Fabuła` and `Wrocław`. Both sides are now diacritic-folded, and fused tokens are matched in either direction (`4 FUN`↔`4FUN`, `TVP3`↔`TVP.3`, `SKY SHOWTIME`↔`SkyShowtime`, `N-TV`↔`n-tv`).
  - **Non-channels:** VOD titles, PPV slots, 24/7 loops and fixtures were given real channels' EPG IDs (`TNT SPORTS: Event 12` → `Sky.Sports.Main.Event.HD.ie`, `DE: THE BEAR` → `The.HISTORY.Channel.de`). `IsNonChannelName` now rejects them on the raw name, before bracket tags are stripped, and names with no brand to anchor on are left unmapped.
- **AI could return an ID it was never offered:** `parseJSONResponse` now validates each returned EPG ID against the exact option set that specific channel was shown, closing the path where the model borrowed another channel's candidate. Near-miss auto-correction (`TVN7.pl` → `TVN.7.pl`) still applies, scoped to that channel's options.
- **Offline fallback guessed blindly:** when the AI errors, the fallback took the top fuzzy candidate unconditionally. It now uses `GetBestCandidateStrict`, which only accepts an unambiguous same-country brand match.
- **Stricter AI prompt:** added a rule requiring the channel's country prefix to agree with the EPG ID's country suffix, and a rule that VOD/PPV/fixtures must return an empty string.
- **"+1" timeshift feeds matched the wrong channel:** a base channel could be handed its one-hour-delayed feed and vice versa — "ITV 1" resolved to `ITV1+1.uk` instead of `ITV1.uk`, and "ITV 2+1" to `ITV.Quiz.uk` instead of `ITV2+1.uk` — because the extra "1" in the "+1" id coincidentally matched the channel's own digit. `hasTimeshift` now detects the `+1`/`plus 1` marker and penalizes any candidate that disagrees with the channel on it, so base and timeshift feeds can no longer swap. Corrected the affected UK ITV mappings in `epg_mapping.json` (base→base, `+1`→`+1`, regionals→their regional id, ITV1 regional feeds→`ITV1.uk`, ITV Box Office→unmapped as PPV).
- **A plain channel could grab a more-specific variant's EPG:** "ITV LONDON"→`ITV.Quiz.uk`, "HBO"→`HBO.Comedy`, "Discovery"→`Discovery.Science` — the shared brand token was enough to win even though the candidate is a different, more specific channel. `extraSpecificWords` now penalizes any candidate carrying a distinguishing word the channel lacks, and drops it entirely when it is the only option, leaving the channel unmapped rather than wrongly assigned. A correct match is never penalized (it shares the channel's words). Validated over ~2,000 real channel names: every resulting change was a wrong assignment being dropped or corrected, none broke a correct mapping.

### Changed
- `epg_mapping.json` purged of the 3,666 mappings the new matcher rejects (backup written alongside it); stale `m3u_cache_*.m3u` files cleared so corrected IDs are served immediately.
- Added `src/fuzzy_test.go` covering brand anchoring, country agreement, diacritic folding, fused tokens, non-channel rejection and AI response validation.
- **Coverage pass:** matched every live channel across all providers against only the EPG ids TVHeadend actually has loaded, and applied the high-confidence results, taking `epg_mapping.json` from ~2,100 to ~4,700 entries with **zero** dangling (non-resolvable) ids remaining. Only assignments where the id covers the channel's distinguishing words were applied; genuine regional feeds (Australian city feeds, Austrian ORF regionals) were mapped to their national EPG, and everything ambiguous was left unmapped rather than guessed. Also repointed/removed the 585 pre-existing mappings that pointed at ids not in the loaded EPG (a dangling id overrides and hides the provider's own `epg_channel_id`).

### Added
- **Native-country id recognition:** `EPGIDCountry` now reads a known-country prefix on TVHeadend-native ids (`uk.ITV London`, `UK| ITV 1`) in addition to the `.uk` suffix, so those resolve and pass the country filter.
- **Number-agreement rule:** a channel's standalone number must match the candidate's — `Eurosport 7` no longer falls onto `Eurosport 1`, and `France 24` no longer onto `France 2`, when the channel's own number has no EPG id. Numbers fused with a quality marker count too (`Cytavision.Sports.1HD` carries the number 1), disjoint numbers on both sides disqualify the candidate outright, and a numbered id offered to a number-less channel is penalized (`RTL` is not `RTL 2`).
- **31 more epgshare rippers in `epg.py`:** the grabber previously loaded only 6 countries (CZ/SK/PL/US/DE/UK); channels from 30+ other countries could never resolve. Measured each of epgshare01's 102 rippers for how many correct assignments it would unlock per megabyte, and added the 31 worthwhile ones (FR, IT, AT, RS, IN, TR, GR, RO, HR, ES, PT, BG, BE, BR, NL, ID, MY, LV, FI, HK, HU, VN, NZ, SG, CY, CO, PH). The merged XMLTV grew from 488 MB to 820 MB and the resolvable EPG universe from ~11,000 to ~18,300 ids.
- **Ollama multi-model fallback:** `ollama_model` now accepts a comma-separated preference list; models are tried in order so a rate-limited or failing model rolls to the next instead of dropping to the offline matcher. Benchmarked all 34 Ollama-cloud models against the free key: 20 usable, 9 scored 5/5 on an EPG-matching trap set. Configured `minimax-m3` (perfect score, 3× faster than gpt-oss:120b) as primary with 5 ranked fallbacks.
- **Daily automatic EPG maintenance** (`src/epg_maintain.go`): runs at 05:15 (after the 04:00 XMLTV refresh) and via `POST /api/epg/maintain`. Sanitizes `epg_mapping.json` against the EPG TVHeadend actually has loaded (deletes/re-points dangling or over-specific ids, drops PPV/VOD entries), then force-corrects TVHeadend channel links through `/api/idnode/save` and clears name-matched garbage links. Idempotent; writes `epg_mapping.json.pre-maintenance` before any destructive change; refuses to run if the EPG universe looks broken (<5000 ids).
- **Fixed `FetchTVHeadendEPGChannels` fetching only 50 channels:** TVHeadend grid APIs default to 50 rows without an explicit `limit` — the AI matching pool had silently been capped at 50 TVHeadend channels since the feature was added. Now `limit=100000`. The bug also caused the first maintenance run to gut the mapping file (recovered from backup; safety rail added).
- **Reverse-direction sweep:** a mapping could point at a *more specific* channel than the stream ("5 Select" → `Sky.Cinema.Select.uk`, "Sky Arts 1" → Sky Sports Ultra, "Quest" → `Quest.Red+1.uk`) because earlier audits only checked brand and country, not id words the channel lacks. Swept all ~7,300 mappings for ids carrying uncovered distinguishing words: 111 re-pointed to their plain correct id, 698 deleted where no clean id exists. Matcher hardening along the way: parentheses treated as token separators (`(HD)`, `(RS)`), digits recognized inside fused tokens in both directions (`R4`→4, `5select`→5), dash-spelled brands collapsed (`E-X-X-E-N`→EXXEN), and "action" removed from the generic-word list.
- **TVHeadend channel links forced via API:** TVHeadend never re-links existing channels when playlist tvg-ids change, so wrong EPG (High Street TV on ITV 1 +1, Channel 4 on ITV 4+1, Playboy on a movie loop) persisted regardless of playlist fixes. Audited all 3,552 channels against the live playlists and set each channel's `epggrab` link directly through `/api/idnode/save` (~1,790 relinked, 432 garbage links cleared), disabling per-channel `epgauto` name-matching on every touched channel so it cannot re-poison them.
- **Second coverage pass** against the enlarged universe: ~2,050 more clean assignments applied (`epg_mapping.json` now ~7,000 entries, zero dangling), plus removal of 176 stale mappings pointing at ids from rippers that were never loaded.

## [1.5.0] - 2026-06-05

### Added
- **AI M3U Bulk Playlist Generation:** Introduced an automated bulk playlist generator within the web UI.
- **Gemini AI EPG Integration:** Added `gemini-3.5-flash` AI logic to automatically map raw IPTV stream names to their official XMLTV EPG IDs.
- **EPG Directory Scraping:** Added support for scraping and parsing bulk EPG text files and full directory indexes from `epgshare01`.
- **API Key Fallback Rotation:** Implemented multi-API key rotation to automatically switch to backup keys and bypass Gemini free-tier rate limits (`429 Quota Exhausted`).
- **M3U Name Sanitizer:** Added a specialized regex cleaner that automatically strips ugly provider prefixes, brackets, and quality tags (e.g., `[1080p]`, `[HEVC]`, `[EXTRA]`) from generated M3U channel names.

### Changed
- **UI Category Sorting:** UI categories in the playlist generator are now sorted alphabetically, allowing for clean grouping by country or prefix.
- **Proxy Timeouts:** Reduced the Webshare proxy request timeout from 60 seconds to 8 seconds to enable ultra-rapid fast-failing on dead proxies.
- **Root Interface Design:** Completely redesigned the root repository `index.html` file into a sleek dark-mode landing page matching the backend UI aesthetic.

### Fixed
- **Gemini Hallucination Auto-Correction:** Engineered strict prompt validation and a fuzzy-matching fallback loop to automatically correct slightly hallucinated EPG IDs returned by the AI (e.g., auto-correcting `TVN7.pl` to `TVN.7.pl`) and dropping entirely fabricated IDs.
- **Xtream API Direct Fallback:** Redesigned the Xtream API fetcher to entirely disable Webshare proxies for lightweight metadata calls (`get_live_categories`, `get_live_streams`), preventing catastrophic 160-second timeouts during bulk M3U generation.
- **Background Refresh Loop Optimization:** Rewrote the background channel refresh loop to securely group providers by `SourceID`, eliminating redundant network fetching and saving massive amounts of bandwidth.

## [1.4.15-beta] - 2026-05-XX

### Added
- **Auto-Discovery:** Implemented auto-discovery for scrambled stream IDs.
- **Global Search:** Added background global search to instantly play local sources without delays.

### Changed
- **Connection Timeout:** Reduced the connection timeout to 1 second to blast through dead alternative streams and accelerate failovers.
- **Hard Disconnects:** Enforced hard disconnects on clients during stream failovers to safely reset and force player decoders (like VLC or TVHeadend) to renegotiate codecs.

### Fixed
- **PCR/DTS Alignment:** Perfectly aligned PCR (Program Clock Reference) and DTS timestamps in the FFmpeg engine to fix massive packet drops in TVHeadend.
- **HTTP Chunking Bug:** Bypassed HTTP chunked transfer boundaries by hijacking the connection and writing raw MPEG-TS bytes directly over the TCP socket, resolving the 10-second TVHeadend disconnect bug.
- **GStreamer Pipeline Error:** Fixed a GStreamer pipeline negotiation error.
- **Buffer Overflows:** Resolved massive datamoshing issues triggered by channel buffer overflows.
- **OS Sleep Precision:** Fixed an OS sleep precision bug causing HD/FHD buffering and missing repository files.
