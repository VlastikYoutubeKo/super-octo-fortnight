# Changelog

All notable changes to the **IPTV Stream Proxy Manager** project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

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
