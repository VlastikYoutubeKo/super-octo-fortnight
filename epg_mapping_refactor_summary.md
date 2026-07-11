# EPG Mapping Refactor Summary

This document summarizes the changes made to the EPG mapping system to fix channels being
assigned the wrong `tvg-id`.

## 1. The Problem

The system automatically maps IPTV channels (e.g. `CZ: AMC [1080p]`) to TVHeadend EPG IDs
using Gemini/Ollama. The workflow is:

1. Clean the channel name.
2. Use a fuzzy string search to find the top candidate EPG IDs.
3. Send the channel name and its candidates to the AI for final selection.

Channels were receiving `tvg-id`s belonging to entirely different channels. An audit of
`epg_mapping.json` found **3,666 of 5,045 mappings were wrong** (73%). There were four
independent root causes.

### A. The fuzzy candidate generator scored on noise

Matches were driven by generic words, numbers and country codes, so the correct EPG ID was
often not even among the candidates. The AI was then forced to pick from a list of garbage:

- `PL-[EXTRA]: 4 FUN KIDS` → `Polsat.Sport.Premium.4.pl` (shared the digit `4`)
- `CZ: AMC` → `Seznam.cz.TV.cz` (shared the token `cz`)
- `UK: AMC` → `Faith.UK.uk` (shared the token `uk`)

### B. There was no country guard at all

Nothing tied a channel's country prefix to the EPG ID's country suffix:

- `CZ: AMC` → `AMC.pl` — right brand, wrong country's feed
- `DE: 4 BLOCKS` → `Canal.4.de.Costa.Rica.cr` — the German prefix `DE` matched the
  *Spanish word* "de" inside a Costa Rican channel name

### C. Diacritics broke equality

The EPG spells Polish channels with diacritics while providers send ASCII. The brand token
never matched, and the score tied away to a sibling regional channel:

- `TVN FABULA` → `TVN.24.pl` (correct answer was `TVN.Fabuła.pl`)
- `TVP3 WROCLAW` → `TVP3.Opole.cz` (correct answer was `TVP.3.Wrocław.pl`)
- `TVP3 BIALYSTOK` → `TVP3.Lublin.cz`

### D. Non-channels were being mapped

VOD titles, PPV slots, 24/7 movie loops and sporting fixtures have no EPG, but were being
assigned real channels anyway:

- `DE: AGATHA ALL ALONG` → `Canal.4.de.Panamá.(RPC.TV).sv`
- `TNT SPORTS: Event 12` → `Sky.Sports.Main.Event.HD.ie`
- `- THE EVENT HAS NOT BEGUN -: DE: DPLUS [PPV EVENTS] 10` → `Sky.Sport.Top.Event.de`

## 2. The Solution

All matching logic now lives in `src/fuzzy.go` and is covered by `src/fuzzy_test.go`.

### A. Brand-token anchoring (`idCarriesBrand`)

A candidate is discarded outright unless it carries the channel's **brand token** — the
first word that is not generic, not numeric and not a stopword. A brand may only be
extended by up to 2 characters (`sky` → `skysp`), so `disco` no longer matches `discovery`
and `ham` no longer matches `hammer`. Numbers and generic words are weighted `1` against
`10` for real words, so they can rank but never qualify a candidate.

Brands are matched across token fusion in both directions, via `brandForms` and bigrams:

| Channel | EPG ID | Mechanism |
|---|---|---|
| `4 FUN KIDS` | `4FUN.KIDS.pl` | bigram `4fun`, and `fun` fused to a leading number |
| `SKY SHOWTIME 2` | `SkyShowtime.2.pl` | bigram `skyshowtime` |
| `TVP3 WROCLAW` | `TVP.3.Wrocław.pl` | brand `tvp3` also tried as stem `tvp` |
| `N-TV` | `n-tv.de` | whole name collapses to `ntv` |

### B. Country agreement (`GetTopCandidatesForChannel`)

`ExtractCountryHint` reads the 2-letter prefix off the channel (only when followed by a
non-letter, so `PRIME:` and `PLAY+:` yield nothing) and `EPGIDCountry` reads the suffix off
the EPG ID. When the channel declares a country **and at least one candidate comes from it,
candidates from every other country are dropped**. `CZ: AMC` can no longer be handed
`AMC.pl`. When no same-country candidate exists the foreign ones are still offered, and the
AI is instructed to refuse them.

### C. Diacritic folding (`foldDiacritics`)

Both sides are folded before tokenizing (`ł`→`l`, `ó`→`o`, `ż`→`z`, `ß`→`ss`, …), so
`FABULA` matches `Fabuła` and `WROCLAW` matches `Wrocław`. No new dependency.

### D. Refusing to guess

- If `GetTopCandidates` returns no candidate, the AI is never called for that channel.
- If a name has no brand to anchor on (`PLAY+: EVENT 1`, `UEFA : 01`, `Vidio EPL 12`), it is
  left unmapped. `event`, `live`, `ppv`, `vod`, `replay`, `backup`, `radio`, `the` and `and`
  are all treated as generic.
- `IsNonChannelName` rejects PPV slots, 24/7 loops and dated fixtures on the **raw** name,
  before bracket tags are stripped. `LIVE: ISCO CHAMPIONSHIP: … : PL: MAX [PPV EVENTS] 5`
  otherwise cleans down to `MAX 5`, whose brand legitimately matches `Sony.Max.1…`.
- `GetBestCandidateStrict` backs the **offline fallback** (used when the AI errors). It has
  no AI to veto a bad guess, so it only accepts an unambiguous same-country brand match —
  previously it blindly took the top-1 candidate.
- `parseJSONResponse` now receives the exact set of options each channel was shown and
  **rejects any ID that was not offered for that specific channel**, closing the path where
  the AI returned an option belonging to a different channel. Near-miss auto-correction
  (`TVN7.pl` → `TVN.7.pl`) is still applied, but only within that channel's offered set.

### E. Stricter prompt

Two rules were added for Gemini and Ollama alike:

> 7. The country prefix of the IPTV channel (`CZ:`, `PL:`, `DE:`) MUST agree with the
>    country suffix of the EPG ID (`.cz`, `.pl`, `.de`) …
> 8. Video-on-demand, PPV, 24/7 movie/series loops and sporting fixtures … are NOT TV
>    channels. They have no EPG. RETURN AN EMPTY STRING.

## 3. Cleanup

`epg_mapping.json` was purged of every entry the new matcher would refuse — **3,666 of
5,045 removed, 1,379 kept**:

| Reason | Count |
|---|---|
| brand mismatch | 3,155 |
| country mismatch | 302 |
| no channel name after cleaning | 196 |
| non-channel (PPV / VOD / fixture marker) | 13 |

A backup was written to `epg_mapping.json.bak.<timestamp>`. Purged channels fall back to
the provider's own `epg_channel_id`, and Auto-EPG re-maps them on the next run. The stale
`m3u_cache_*.m3u` files were deleted so the corrected mapping takes effect immediately
rather than after the 12-hour TTL.

## 4. Verification

`src/fuzzy_test.go` covers every case above. Beyond the unit tests, the matcher was run
over the **real** 9,705-ID EPG universe fetched from TVHeadend against the 515 real channel
names in the live Polish playlist. Top-1 picks whose country contradicted the channel prefix
fell from 18 to 7, and each of the remaining 7 is a channel with **no Polish EPG entry in
existence** (`SkyShowtime`, `High League`, …) — offered to the AI, which rule 7 requires it
to refuse.

## 5. Known limitation

The brand+country filter cannot adjudicate errors *within* a brand and country. Mappings
like `BBC RADIO OXFORD` → `BBC.Radio.Bristol.104.6FM.uk` share both the brand `bbc` and the
country `uk`, so they survive the filter and were kept. Only the stricter prompt and manual
override (the EPG mapping editor in the web UI) address those.
