# EPG System — Architecture, Rules & Operations

> Handoff document for AI assistants (Gemini / Antigravity / Claude) and humans.
> Written 2026-07-11 after a multi-day overhaul of the EPG assignment pipeline.
> Read this BEFORE touching anything EPG-related. The invariants at the bottom exist
> because each one was violated once and caused real, user-visible breakage.

## 1. The moving parts

```
Xtream providers (3 panels, 12 accounts)
        │  player_api / xmltv.php
        ▼
Go proxy (this repo, src/ → ./proxy_server, runs in tmux session 0:0.0)
  - streams:      :9000   (/{source}/{channel}.ts)
  - API + M3U:    :9005   (/api/..., /api/playlist/bulk.m3u)
  - epg_mapping.json  = channel-name → EPG-id overrides (THE source of truth)
        │  bulk.m3u with tvg-id per channel
        ▼
TVHeadend (http://<tvheadend-host>:9981, same host)
  - 5 IPTV networks; 3 consume our bulk.m3u URLs (UK / PL / CZ&DE)
  - channels link to EPG via their `epggrab` field (list of EPG-source UUIDs)
        ▲  XMLTV via /var/lib/tvheadend/epggrab/xmltv.sock
        │
/root/tvheadend_proxies/epg.py  (systemd timer update_tvh_epg.timer, 04:00 + 16:00)
  - merges 43 XMLTV URLs (incl. 37 epgshare rippers), UK guide APIs,
    and the providers' own xmltv.php (dedup by channel id, gap-fill only)
  - writes /var/lib/tvheadend/.xmltv/tv_grab_file.xmltv (~820 MB, ~17.9k channels)
  - pushes it into TVHeadend's socket; reports to Discord webhook
```

## 2. How a channel gets its EPG (end to end)

1. `bulk.m3u` emits `tvg-id` per channel: `EPGMapping[raw_stream_name]` if present,
   else the provider's own `epg_channel_id`.
2. Unmapped channels go through **auto-EPG** in the background: fuzzy candidate
   generation (`src/fuzzy.go`) → 7 candidates → AI picks or refuses → validated →
   stored in `epg_mapping.json`. The AI is Ollama cloud (`https://ollama.com/api/chat`);
   `ollama_model` in config.json is a comma-separated preference list tried in order
   (rate-limited/failing models fall through to the next). Current list, benchmarked
   5/5 on the trap set with the free key: `minimax-m3, gpt-oss:120b, nemotron-3-ultra,
   qwen3-coder:480b, gemma4:31b, gpt-oss:20b`. Gemini keys are the fallback only when
   `ollama_api_url` is empty. Subscription-gated on this key (do NOT add): GLM-5.x,
   DeepSeek v3/v4, Kimi K2.x, mistral-large-3, qwen3.5, minimax-m2.7, gemini-3-flash.
3. TVHeadend links a channel to an EPG source **only at channel creation**. It NEVER
   re-links existing channels when the playlist's tvg-id changes. Forcing a re-link is
   done via `POST /api/idnode/save` on the channel's `epggrab` field (see §4).
   A re-link moves **no events**: TVHeadend attaches broadcasts to channels at import
   time, so the channel keeps showing the old source's guide until a grabber re-reads
   the XMLTV. Always follow a re-link with `POST /api/epggrab/internal/rerun`
   (`rerun=1`) — maintenance does this automatically (see Invariant 9).
4. If a channel has no/unresolvable tvg-id and `epgauto: true`, TVHeadend falls back
   to name-matching — this is what produced "ITV 1 +1 shows High Street TV". Channels
   the maintenance touches get `epgauto: false` for exactly this reason.

## 3. The matcher (`src/fuzzy.go`) — rules that exist because they failed

All covered by `src/fuzzy_test.go` (`go test ./...` in `src/`). Every rule below fixed
a real wrong assignment:

| Rule | Wrong assignment it fixed |
|---|---|
| Brand token anchoring, max +2 chars extension | `DISCO POLO`→Discovery, `HAM`→Hammer |
| Country prefix must match id suffix (when same-country candidate exists) | `CZ: AMC`→`AMC.pl`, `DE: 4 BLOCKS`→`Canal.4.de.Costa.Rica.cr` ("de" = Spanish word!) |
| Diacritic folding both sides | `TVN FABULA`→TVN.24 (EPG spells `Fabuła`) |
| Fused tokens both directions (`4 FUN`↔`4FUN`, `TVP3`↔`TVP.3`) | `4 FUN KIDS`→Polsat.Sport.Premium.4 |
| `IsNonChannelName` on the RAW name (PPV/VOD/24-7/fixtures) | `…PL: MAX [PPV EVENTS] 5`→Sony.Max (cleans to "MAX 5"!) |
| "+1" timeshift agreement | `ITV 1`→ITV1+1, `ITV 2+1`→ITV.Quiz |
| Extra-specific-word penalty (id words channel lacks) | `5 Select`→Sky.Cinema.Select, `HBO`→HBO.Comedy |
| Number agreement (incl. digits fused in `1HD`, `R4`, `5select`; disjoint ⇒ discard) | `Eurosport 7`→Eurosport.1, `BBC Radio 2`→BBC.R4.LW |
| Parentheses are token separators | false-deleting `beIN.SPORTS.(HD).sg` |
| Dash-spelled brands collapse (`E-X-X-E-N`→EXXEN) | EXXEN→TRT.SPOR |
| AI may only return ids it was OFFERED for THAT channel (`parseJSONResponse`) | model borrowing another channel's candidate |
| Offline fallback = `GetBestCandidateStrict` (same-country + full cover only) | blind top-1 guesses when AI errored |

**Philosophy: unmapped is always better than wrongly mapped.** TVHeadend shows blank
EPG for unmapped; wrong EPG misleads the user and survives forever.

### Pins — when the heuristic is wrong and a human is right

Some channels **share one broadcast slot**: `CS FILM` airs CS Horror overnight, and the
only 24 h guide for it is the combined id `CS.Film-CS.Horror.HD.sk`. Mapping it there is
correct, but `ExtraIDWords` sees "horror" as a word the channel name lacks and the
janitor would re-point it back to the CS-Film-only guide (06:00-22:00, night blank)
every single night.

`epg_pins.json` (`src/epg_pins.go`) is the escape hatch: channel name → epg id, applied
at startup and at every maintenance run, and skipped by the sanitizer. A pin is *not* a
licence to break Invariant 2 — a pin whose id is not loaded in TVHeadend is reported in
the maintenance output (`pins_unloaded`) and ignored, never written into the mapping.

```
curl -X PUT http://127.0.0.1:9005/api/epg/pin \
     -d '{"channel":"CZ: CS FILM  [1080p]","epg_id":"CS.Film-CS.Horror.HD.sk"}'
curl http://127.0.0.1:9005/api/epg/pins       # list
curl -X DELETE http://127.0.0.1:9005/api/epg/pin -d '{"channel":"..."}'
```

Use a pin for a decision you have *verified* (compare the two feeds' schedules first —
`tools/epg_gap_audit.py`). Do not use it to paper over a matcher bug: fix the matcher.

## 4. Daily maintenance (the "automatic EPG fixing")

`src/epg_maintain.go` — runs **daily at 05:15** (1 h after the 04:00 epg.py refresh)
and on demand:

```
curl -X POST http://127.0.0.1:9005/api/epg/maintain
# → {"mapping_total":6816,"mapping_deleted":0,"mapping_repointed":0,"pins_applied":0,
#    "tvh_channels":3548,"tvh_relinked":0,"tvh_cleared":0,"tvh_reimported":false,
#    "epg_universe":17938,"duration_seconds":38}
```

What it does, in order:

1. **Universe** = TVHeadend's `/api/epggrab/channel/grid?limit=100000` (what is
   actually loaded — matching against anything else creates dangling ids).
   **Safety rail: aborts if universe < 5000** (a broken fetch once returned the
   default 50 rows and gutted the mapping file — see Incidents).
2. **Apply pins** — re-assert every curated override before the sanitizer looks at
   the file (§3). Pins to ids TVHeadend has not loaded are reported, not applied.
3. **Sanitize `epg_mapping.json`**: delete/re-point entries that are dangling
   (id not loaded), name a more-specific different channel (`ExtraIDWords` > 0),
   or belong to PPV/VOD/fixture listings. Pinned channels are skipped. Writes
   `epg_mapping.json.pre-maintenance` backup before any change.
4. **Relink TVHeadend**: fetch the playlists each TVH network consumes (self-request
   via localhost), compute expected tvg-id per channel (mux `iptv_sname` join), set
   `epggrab` via `/api/idnode/save` in batches. Tie in name-coverage goes to the
   curated playlist id; a strictly-better current link is kept (protects e.g.
   `TVNSzkolaZycia.pl` from being downgraded to `TVN.pl`).
5. **Clear garbage links** on channels with no tvg-id whose name-matched link shares
   zero distinguishing words (`STEVEN SEGAL`→Playboy class). `epgauto: false` on all
   touched channels.
6. **Re-import** (`POST /api/epggrab/internal/rerun`) if any link changed — without it
   the new link shows the old guide until TVHeadend's next cron grab (Invariant 9).

The run is **idempotent** — a second run right after reports all zeros (and skips the
re-import, which is why it only fires when something actually changed).

## 5. Operational runbook

| Task | How |
|---|---|
| Run maintenance now | `curl -X POST http://127.0.0.1:9005/api/epg/maintain` |
| Rebuild + restart proxy | `cd src && go test ./... && go build -o ../proxy_server.new . && mv ../proxy_server ../proxy_server.prev && mv ../proxy_server.new ../proxy_server`; then Ctrl-C + `./proxy_server` in tmux `0:0.0` (streams drop ~2 s) |
| Refresh EPG data now | `python3 /root/tvheadend_proxies/epg.py` (root; ~3.5 min, peak ~4-5 GB RAM, fires Discord webhook) or wait for the 04:00/16:00 timer |
| Edit one mapping | `PUT /api/epg/mapping {"channel":"<raw name>","epg_id":"<id>"}` / `DELETE` with `{"channel":...}` — the web UI on :9005 has an editor too |
| Force one TVH channel's EPG | `POST /api/idnode/save` form-field `node=[{"uuid":"<channel uuid>","epggrab":["<epg source uuid>"],"epgauto":false}]`, **then** `POST /api/epggrab/internal/rerun` with `rerun=1` (else the guide keeps the old events for up to 12 h) |
| Pin a curated mapping | `PUT /api/epg/pin {"channel":"<raw name>","epg_id":"<id>"}` — survives the janitor; `GET /api/epg/pins` to list |
| Find part-day guides | `python3 tools/epg_gap_audit.py` — channels whose EPG covers <20 h/day while a same-schedule id covers more (the CS FILM class) |
| Mapping backups | `epg_mapping.json.bak.*` (manual snapshots), `epg_mapping.json.pre-maintenance` (rolling, written before each destructive maintenance) |

Key files: `epg_mapping.json` (runtime, saved by proxy — **never edit while proxy
runs**, use the API), `src/fuzzy.go` (matcher), `src/epg_maintain.go` (janitor),
`src/fuzzy_test.go` (regression suite), `/root/tvheadend_proxies/epg.py` (grabber;
backup `epg.py.orig.bak`).

## 6. Invariants — violate these and you will recreate old bugs

1. **TVHeadend JSON grids return 50 rows unless you pass `limit=`.** Always pass
   `limit=100000`. This single omission once deleted 6,572 mappings (see Incidents).
2. **Never match against ids that are not loaded into TVHeadend.** The epgshare
   directory `.txt` index lists ids from ~100 rippers; only 37 are loaded by epg.py.
   A mapping to an unloaded id is worse than no mapping: it overrides the provider's
   own `epg_channel_id` AND triggers TVHeadend name-matching garbage.
3. **TVHeadend never re-links existing channels.** Fixing the playlist is only half
   the job; the relink (maintenance step 3) is the other half.
4. **`epgauto` (name-matching) is the enemy.** It created High-Street-TV-on-ITV.
   Keep it disabled on curated channels.
5. **Prefer unmapped over guessed.** VOD/PPV/24-7/fixture listings have no EPG by
   definition. ~1,400 TVHeadend channels correctly have none.
6. **Don't edit `epg_mapping.json` on disk while the proxy runs** — it holds the map
   in memory and will overwrite your edit on the next save. Use the API.
7. **epg.py holds the whole XMLTV tree in RAM** (~4-5 GB peak at current size).
   Adding many more sources needs a memory check first (`free -m`, keep ≥3 GB spare).
8. **Restarting the proxy drops live streams for ~2 s** and it runs as a foreground
   process in tmux session `0`, window `0` — restart it there, not via nohup.
9. **A re-link is invisible until a grabber re-imports.** TVHeadend attaches
   broadcasts to channels at import time. Change `epggrab` and the channel still shows
   the *old* source's events — for up to 12 h (cron `4 */12 * * *`). Always follow with
   `POST /api/epggrab/internal/rerun` (`rerun=1`, form-encoded; it 400s without the
   argument). Maintenance now does this automatically.
10. **Every EPG id exists TWICE in TVHeadend** — once per grabber module: `xmltv` (the
    socket epg.py pushes to) and the internal cron grabber script in `/usr/bin/`
    (`tv_grab_*`, reading the same `~/.xmltv/tv_grab_file.xmltv`). `/api/epggrab/channel/grid`
    therefore returns 35,949 entries for 17,938 ids, and channel links are split across
    both modules (~1,456 socket / ~1,281 file). Both carry identical data, so either
    works — but a link resolved by id (first-match) may land on the module that is not
    the one you just fed. If a channel's guide looks stale after a push, check *which*
    module's entry it is linked to and re-run that grabber.
11. **Not every partial guide is a bug, and not every "missing" channel is missing.**
    Timeshare channels (CS Film / CS Horror) have one combined feed; a channel whose
    guide covers 06:00-22:00 with a blank night is usually mapped to the half of the
    slot. `tools/epg_gap_audit.py` finds these by evidence (schedule agreement), not by
    name — do not "fix" them by fuzzy-matching a plausible-looking id.

## 7. Incident log (what already went wrong once)

- **2026-07-11: mapping gutted by grid default-limit.** First maintenance run used
  `FetchTVHeadendEPGChannels` which lacked `limit=` → universe of 50 → 6,572 mappings
  deleted as "dangling". Recovered from `epg_mapping.json.bak.reverse.*` + replay of
  the pending fix plan. Fixes: `limit=100000`, the <5000 safety rail, and the
  pre-maintenance backup. Lesson → Invariant 1.
- **2026-07-10 audit: 3,666 of 5,045 mappings were wrong** (72%) — accumulated from
  the pre-rule-hardening AI era. Root causes are the table in §3.
- **`epgauto` name-matching poisoned ~430 channels** that should have no EPG at all
  (movie loops → Playboy etc.). Cleared; `epgauto` disabled on touched channels.
- **2026-07-14: CS FILM had no overnight guide.** CS Film timeshares its slot with CS
  Horror (22:00-06:00); it was mapped to `CS.Film.cz`, which only covers 06:00-22:00.
  Re-pointed and pinned to the combined `CS.Film-CS.Horror.HD.sk` (same schedule, same
  `+0200` clock, full 24 h). Two follow-on discoveries came out of it: the re-link did
  not reach the guide until a grabber re-run (→ Invariants 9/10), and an audit found
  ~57 more channels on part-day guides — most notably the Austrian ripper, which serves
  only ~3-4 h/day for ~24 `AT:` channels. Those are NOT yet fixed; see
  `tools/epg_gap_audit.py`.

## 8. Current state (2026-07-14)

- `epg_pins.json`: 3 pins (the CS FILM streams → `CS.Film-CS.Horror.HD.sk`).
- Known-unfixed: ~57 channels on part-day guides (`tools/epg_gap_audit.py`), incl. the
  near-empty Austrian ripper. Needs a human decision per channel — the audit's
  suggestions are candidates, not verdicts (it pairs on schedule agreement, so a
  near-empty feed can agree with the wrong channel: `ProSieben.at`→`VOX.de` is bogus).
- `epg_mapping.json`: ~6,350 entries, **0 dangling**, reverse-sweep clean.
- TVHeadend: 3,552 channels — ~1,740 linked to curated ids, ~180 name-matched OK,
  ~1,440 correctly EPG-less (VOD/PPV), 0 known-wrong.
- EPG universe: ~17,900 loaded ids (37 epgshare rippers + provider EPG + APIs).
- Verified cases: `5 SELECT`→`5SELECT.uk`, `ITV 1 +1`→`ITV1+1.uk`,
  `ITV 4`→`ITV4.uk`, `STV`→`STV.uk`, `QUEST`→`QUEST.uk`, `BBC TWO`→`BBC.Two.HD.uk`,
  `SKY MAX`→`Sky.Max.HD.uk`, `TVP3 WROCLAW`→`TVP 3 Wrocław`, `CZ: AMC`→`AMC.cz`.
