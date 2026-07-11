package main

import (
	"regexp"
	"sort"
	"strings"
)

// Common generic/noise words that shouldn't drive a match on their own
var genericWords = map[string]bool{
	"tv": true, "polska": true, "poland": true, "pl": true,
	"cz": true, "sk": true, "uk": true, "us": true, "de": true, "ru": true, "hr": true, "pe": true,
	"channel": true, "sport": true, "extra": true, "plus": true,
	"hd": true, "premium": true, "network": true, "world": true, "film": true,
	// Placeholder words used by PPV/on-demand slots. They are never a brand, so a
	// name made only of these ("PLAY+: EVENT 1") has nothing to anchor on and is
	// left unmapped rather than matched to "blue.Event.D.1.ch".
	"event": true, "events": true, "live": true, "ppv": true, "vod": true,
	"replay": true, "backup": true, "radio": true,
	// Stopwords: "THE WAR" must anchor on "war", not match "The.Box.uk".
	"the": true, "and": true,
}

// Diacritics carry no matching information but do break equality: the EPG has
// "TVN.Fabuła.pl" and "TVP.3.Wrocław.pl" while providers send "TVN FABULA" and
// "TVP3 WROCLAW". Without folding, the brand never matches and the score ties away
// to a sibling regional channel.
var diacriticFold = map[rune]string{
	'ą': "a", 'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a",
	'ć': "c", 'ç': "c", 'č': "c",
	'ď': "d", 'đ': "d",
	'ę': "e", 'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'ě': "e",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i",
	'ĺ': "l", 'ľ': "l", 'ł': "l",
	'ń': "n", 'ñ': "n", 'ň': "n",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o",
	'ŕ': "r", 'ř': "r",
	'ś': "s", 'š': "s", 'ş': "s",
	'ť': "t",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ů': "u",
	'ý': "y", 'ÿ': "y",
	'ź': "z", 'ż': "z", 'ž': "z",
	'ß': "ss", 'æ': "ae",
}

// hasTimeshift reports whether a name is a "+1" (one-hour delayed) feed.
func hasTimeshift(s string) bool {
	return reTimeshift.MatchString(s)
}

func foldDiacritics(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if repl, ok := diacriticFold[r]; ok {
			b.WriteString(repl)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// A brand token may be extended by at most this many characters and still count as
// the same brand ("sky" -> "skysp", "tvp" -> "tvp1"). It stops "disco" from matching
// "discovery" and "ham" from matching "hammer".
const maxBrandExtension = 2

// Score penalty when a channel and a candidate disagree on the "+1" timeshift feed.
// Large enough to break the numeric double-count that makes "ITV 1" prefer "ITV1+1.uk"
// over "ITV1.uk", small enough not to override a brand-level difference.
const timeshiftPenalty = 5

// Score penalty per distinguishing word an EPG ID carries that the channel lacks. Set
// so a candidate that is a more-specific different channel ("ITV LONDON" vs "ITV.Quiz.uk",
// "HBO" vs "HBO.Comedy") loses to a plainer same-brand ID, and drops out entirely when it
// is the only option — leaving the channel unmapped rather than wrongly assigned.
const extraWordPenalty = 8

// Score penalty when a channel and a candidate both carry standalone numbers and none
// agree — different entries in a numbered line-up. Stops "Eurosport 7" -> Eurosport.1
// and "France 24" -> France.2 when the channel's own number has no EPG id.
const numberMismatchPenalty = 8

// a channel number fused with a quality marker, e.g. "1hd", "2fhd", "3uhd", "4sd"
var reNumQuality = regexp.MustCompile(`^([0-9]{1,3})(hd|fhd|uhd|sd)$`)

// a number carried inside a short alphanumeric token, e.g. "r4" (BBC R4), "f1", "tvp3"
var reLettersDigits = regexp.MustCompile(`^([a-z]{1,4})([0-9]{1,3})$`)

// ... or leading it, e.g. "5select", "4fun", "7flix"
var reDigitsLetters = regexp.MustCompile(`^([0-9]{1,3})([a-z]{1,7})$`)

// numericTokens returns the standalone integer tokens in a tokenized name, including
// numbers fused with a quality marker ("Cytavision.Sports.1HD" carries the number 1)
// or hidden in a short token ("BBC.R4" carries the number 4). Country suffixes like
// "us2" carry no channel number and are skipped.
func numericTokens(words []string) []string {
	var out []string
	for _, w := range words {
		if isNumeric(w) {
			out = append(out, w)
		} else if m := reNumQuality.FindStringSubmatch(w); m != nil {
			out = append(out, m[1])
		} else if m := reLettersDigits.FindStringSubmatch(w); m != nil && !knownCountries[m[1]] {
			out = append(out, m[2])
		} else if m := reDigitsLetters.FindStringSubmatch(w); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}

func disjointNumbers(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return false
			}
		}
	}
	return true
}

func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

var (
	// [1080p], [4K-Q], [EXTRA], (NA), [24/7 O-D], [PPV EVENTS], [F1] ...
	reBracketTags = regexp.MustCompile(`\[[^\[\]]*\]|\([^()]*\)`)
	// bare quality markers that appear without brackets
	reBareQuality = regexp.MustCompile(`(?i)\b(1080p|720p|480p|4k|hevc|h265|sd|fhd|uhd)\b`)
	// leading country/provider group tag delimited without a colon, e.g. "UK| Sky One",
	// "CZ - Nova". Only a 2-letter code with a spaced dash counts, so real brands like
	// "HAM - HAMILTON" or "AXN-SPIN" keep their leading token.
	reGroupPrefix = regexp.MustCompile(`^(?i)[A-Z]{2,4}\s*\|\s*|^(?i)[A-Z]{2}\s+-\s+`)
	// the 2-4 letter code a channel name opens with, e.g. "PL" in "PL-[EXTRA]: AXN"
	reCountryHint = regexp.MustCompile(`^(?i)([A-Z]{2,4})(?:[^A-Za-z]|$)`)
	// trailing digits on an EPG country suffix, e.g. "us2" -> "us"
	reSuffixDigits = regexp.MustCompile(`[0-9]+$`)
	// "+1" timeshift marker: "+1", "+ 1", "plus 1", "plus-1", "plus.1" (e.g. "ITV1+1.uk",
	// "itv1-london-plus-1", "Comedy Central +1"). Anchored on a boundary so "channel 41"
	// or a bare "+" (CANAL+) does not count.
	reTimeshift = regexp.MustCompile(`(?i)(\+\s*1|plus[\s._-]*1)($|[^0-9])`)
	// token separators; '-' included so "N-TV" and "AXN-SPIN" split like their EPG IDs
	tokenSeparators = strings.NewReplacer("-", " ", "_", " ", ".", " ", ":", " ", "|", " ", "+", " ", "/", " ", "(", " ", ")", " ")
)

// Markers that identify a listing as something other than a TV channel: a PPV slot, a
// 24/7 on-demand loop, or a dated sporting fixture. These must be tested against the raw
// name, before bracket tags are stripped — "…: PL: MAX [PPV EVENTS] 5" otherwise cleans
// down to "MAX 5", whose brand legitimately matches "Sony.Max.1.Tv.Channel.Today.in2".
var reNonChannel = regexp.MustCompile(`(?i)\[ppv\b|\bppv events\b|24/7|\bo-d\b|` +
	`live event only|event has not begun|^\s*(live|ended|upcoming)\s*:|` +
	`\b(mon|tue|wed|thu|fri|sat|sun)\s+\d{1,2}\s+(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)\b`)

// IsNonChannelName reports whether a listing is a PPV slot, on-demand loop or fixture
// rather than a real channel. Such listings have no EPG and must never get a tvg-id.
func IsNonChannelName(name string) bool {
	return reNonChannel.MatchString(name)
}

// CleanChannelNameForMatching strips IPTV junk (quality tags, bracketed markers)
// and provider/country group prefixes so only the channel brand remains.
// "PL-[EXTRA]: 4 FUN KIDS [1080p]" -> "4 FUN KIDS"
// dash-spelled brand, e.g. "E-X-X-E-N" -> "EXXEN"
var reDashSpelled = regexp.MustCompile(`\b([A-Za-z])(?:-([A-Za-z]))(?:-([A-Za-z]))(?:-([A-Za-z]))?(?:-([A-Za-z]))?(?:-([A-Za-z]))?\b`)

func CleanChannelNameForMatching(name string) string {
	clean := reDashSpelled.ReplaceAllStringFunc(name, func(m string) string {
		return strings.ReplaceAll(m, "-", "")
	})
	// Loop: bracket tags nest, e.g. "[[4K-Q]-Q]".
	for i := 0; i < 5; i++ {
		next := reBracketTags.ReplaceAllString(clean, " ")
		if next == clean {
			break
		}
		clean = next
	}
	clean = strings.NewReplacer("[", " ", "]", " ", "(", " ", ")", " ").Replace(clean)
	clean = reBareQuality.ReplaceAllString(clean, " ")
	clean = strings.ReplaceAll(clean, "LIVE EVENT ONLY", " ")

	// Provider group tags are everything up to the last colon, so "UK: [F1]: HAM ..."
	// and "- THE EVENT HAS NOT BEGUN -: : DE: DPLUS 1" both collapse correctly.
	if idx := strings.LastIndex(clean, ":"); idx != -1 {
		clean = clean[idx+1:]
	}
	// A prefix may also be delimited without a colon ("UK| Sky One", "CZ - Nova").
	clean = reGroupPrefix.ReplaceAllString(strings.TrimSpace(clean), "")

	return strings.TrimSpace(strings.Join(strings.Fields(clean), " "))
}

// ExtractCountryHint returns the lowercase 2-letter country code a channel name
// is prefixed with ("PL-[EXTRA]: AXN" -> "pl"), or "" when there is none. Only a
// code immediately followed by a non-letter counts, so "PRIME:" and "PLAY+:"
// yield no hint rather than a bogus one.
func ExtractCountryHint(name string) string {
	m := reCountryHint.FindStringSubmatch(strings.TrimSpace(name))
	if m == nil {
		return ""
	}
	code := strings.ToLower(m[1])
	if len(code) != 2 {
		return ""
	}
	return code
}

// Country codes recognized in the *prefix* position of a TVHeadend-native EPG id
// ("uk.ITV London", "UK| ITV 1", "de: RTL"). Kept to real codes so a leading "tv."
// or "la." is not mistaken for a country.
var knownCountries = map[string]bool{
	"uk": true, "de": true, "pl": true, "cz": true, "sk": true, "us": true, "fr": true,
	"es": true, "it": true, "nl": true, "ie": true, "at": true, "ch": true, "ru": true,
	"ua": true, "hr": true, "ro": true, "be": true, "dk": true, "no": true, "se": true,
	"fi": true, "pt": true, "tr": true, "gr": true, "hu": true, "ca": true, "au": true,
	"in": true, "pe": true, "ar": true, "cl": true, "mx": true, "br": true, "rs": true,
	"bg": true, "si": true, "lt": true, "lv": true, "ee": true,
}

// EPGIDCountry returns the 2-letter country code of an EPG ID. It reads the suffix
// ("13.Ulica.HD.pl" -> "pl", "NBA.League.Pass.10.us2" -> "us"), and failing that a
// known-country prefix on TVHeadend-native ids ("uk.ITV London" / "UK| ITV 1" -> "uk").
// Returns "" when neither is present (e.g. "...plex").
func EPGIDCountry(id string) string {
	segs := strings.Split(id, ".")
	last := strings.ToLower(segs[len(segs)-1])
	if i := strings.Index(last, "_"); i != -1 {
		last = last[:i]
	}
	last = reSuffixDigits.ReplaceAllString(last, "")
	if len(last) == 2 {
		return last
	}
	// prefix form: a known 2-letter code followed by ".", "|", ":" or "-"
	if len(id) > 2 {
		if code := strings.ToLower(id[:2]); knownCountries[code] {
			switch id[2] {
			case '.', '|', ':', '-', ' ':
				return code
			}
		}
	}
	return ""
}

func tokenize(s string) (words []string, joined string) {
	words = strings.Fields(tokenSeparators.Replace(foldDiacritics(strings.ToLower(s))))
	return words, strings.Join(words, "")
}

// bigrams returns each pair of adjacent words fused together, so a channel's "4 FUN"
// can meet an EPG ID's "4fun" and "SKY SHOWTIME" can meet "SkyShowtime".
func bigrams(words []string) []string {
	if len(words) < 2 {
		return nil
	}
	out := make([]string, 0, len(words)-1)
	for i := 0; i+1 < len(words); i++ {
		out = append(out, words[i]+words[i+1])
	}
	return out
}

// a brand that ends in digits, e.g. "tvp3", which an EPG ID may split as "TVP.3"
var reAlphaThenDigits = regexp.MustCompile(`^([a-z]+)[0-9]+$`)

// brandForms returns every spelling of the channel's brand an EPG ID might use.
func brandForms(words []string, brand string) []string {
	forms := []string{brand}
	for i := 0; i+1 < len(words); i++ {
		if words[i] == brand || words[i+1] == brand {
			forms = append(forms, words[i]+words[i+1])
		}
	}
	// "tvp3" also appears as the token "tvp" next to a separate "3".
	if m := reAlphaThenDigits.FindStringSubmatch(brand); m != nil &&
		len(m[1]) >= 3 && !genericWords[m[1]] {
		forms = append(forms, m[1])
	}
	return forms
}

// idCarriesBrand reports whether an EPG ID genuinely carries one of the channel's brand
// forms: an exact token, a token the brand extends into by <= maxBrandExtension chars
// ("sky" -> "skysp"), a token where the brand is fused to a leading number ("fun" ->
// "4fun"), or an ID whose whole name collapses to the brand ("n-tv" -> "ntv").
func idCarriesBrand(forms []string, idWords []string, idJoined string) bool {
	for _, f := range forms {
		if len(f) < 2 {
			continue
		}
		if idJoined == f {
			return true
		}
		for _, iw := range idWords {
			if iw == f {
				return true
			}
			if len(f) >= 3 && len(iw) > len(f) && len(iw)-len(f) <= maxBrandExtension &&
				strings.HasPrefix(iw, f) {
				return true
			}
			if len(iw) > len(f) && strings.HasSuffix(iw, f) && isNumeric(iw[:len(iw)-len(f)]) {
				return true
			}
		}
	}
	return false
}

// isBrandWord reports whether an EPG ID word is (part of) the channel's brand, using
// the same exact / extension / number-fusion rules as idCarriesBrand.
func isBrandWord(forms []string, iw string) bool {
	for _, f := range forms {
		if len(f) < 2 {
			continue
		}
		if iw == f {
			return true
		}
		if len(f) >= 3 && len(iw) > len(f) && len(iw)-len(f) <= maxBrandExtension &&
			strings.HasPrefix(iw, f) {
			return true
		}
		if len(iw) > len(f) && strings.HasSuffix(iw, f) && isNumeric(iw[:len(iw)-len(f)]) {
			return true
		}
	}
	return false
}

// extraSpecificWords counts distinguishing words an EPG ID carries that the channel
// does not. "quiz" in "ITV.Quiz.uk" is extra for "ITV LONDON"; "comedy" in "HBO.Comedy"
// is extra for "HBO". Such a word means the candidate is a *more specific, different*
// channel, so it should not win over a plainer same-brand ID. Generic words, numbers,
// the brand itself, and words the channel also has (exactly or related) do not count.
func extraSpecificWords(channelWords, idWords []string, forms []string) int {
	n := 0
	for _, iw := range idWords {
		if len(iw) < 4 || genericWords[iw] || isNumeric(iw) || isBrandWord(forms, iw) {
			continue
		}
		found := false
		for _, w := range channelWords {
			if w == iw || tokensRelated(w, iw) {
				found = true
				break
			}
		}
		if !found {
			n++
		}
	}
	return n
}

// tokensRelated is the softer, score-only kinship test between a channel word and an
// EPG ID word. Candidacy is already settled by idCarriesBrand, so this only ranks.
func tokensRelated(w, iw string) bool {
	if len(w) >= 3 && len(iw) >= 3 && (strings.HasPrefix(iw, w) || strings.HasPrefix(w, iw)) {
		return true
	}
	if len(iw) > len(w) && strings.HasSuffix(iw, w) && isNumeric(iw[:len(iw)-len(w)]) {
		return true
	}
	if len(w) > len(iw) && strings.HasSuffix(w, iw) && isNumeric(w[:len(w)-len(iw)]) {
		return true
	}
	return false
}

// GetTopCandidatesForChannel cleans a raw IPTV channel name, then returns the best
// EPG IDs for it. When the channel carries a country prefix and at least one candidate
// comes from that same country, candidates from other countries are dropped — that is
// what stops "CZ: AMC" from being handed "AMC.pl".
func GetTopCandidatesForChannel(rawName string, epgIDs []string, topN int, minScore int) []string {
	if IsNonChannelName(rawName) {
		return nil
	}
	clean := CleanChannelNameForMatching(rawName)
	if clean == "" {
		return nil
	}

	// Over-fetch so the country filter still has depth to work with.
	candidates := GetTopCandidates(clean, epgIDs, topN*4, minScore)
	if len(candidates) == 0 {
		return nil
	}

	if hint := ExtractCountryHint(rawName); hint != "" {
		var sameCountry []string
		for _, id := range candidates {
			if EPGIDCountry(id) == hint {
				sameCountry = append(sameCountry, id)
			}
		}
		if len(sameCountry) > 0 {
			candidates = sameCountry
		}
	}

	if len(candidates) > topN {
		candidates = candidates[:topN]
	}
	return candidates
}

// GetBestCandidateStrict returns an EPG ID only when it can be assigned without any
// judgement call: the channel must declare a country, the ID must come from that same
// country, and the ID must carry the channel's brand. Used by the offline fallback,
// which has no AI to veto a bad guess.
func GetBestCandidateStrict(rawName string, epgIDs []string) string {
	hint := ExtractCountryHint(rawName)
	if hint == "" {
		return ""
	}
	for _, id := range GetTopCandidatesForChannel(rawName, epgIDs, 1, 10) {
		if EPGIDCountry(id) == hint {
			return id
		}
	}
	return ""
}

// GetTopCandidates finds the top N closest EPG IDs for an already-cleaned channel
// name based on weighted word intersection, anchored on the channel's brand token.
func GetTopCandidates(channelName string, epgIDs []string, topN int, minScore int) []string {
	words, joined := tokenize(channelName)
	if len(words) == 0 {
		return nil
	}

	// Identify the "brand token" = first non-generic, non-numeric word.
	brandWord := ""
	for _, w := range words {
		if !genericWords[w] && !isNumeric(w) && len(w) > 1 {
			brandWord = w
			break
		}
	}
	// Names made only of generic words split across separators ("N-TV") still have a
	// brand once collapsed.
	if brandWord == "" && len(joined) >= 3 && !genericWords[joined] && !isNumeric(joined) {
		brandWord = joined
	}
	// Nothing distinctive to anchor on (e.g. "TV 2", "01") -> refuse to guess.
	if brandWord == "" {
		return nil
	}

	forms := brandForms(words, brandWord)
	channelBigrams := bigrams(words)
	channelTimeshift := hasTimeshift(channelName)
	channelNums := numericTokens(words)

	type score struct {
		id         string
		matchCount int
		lengthDiff int
	}

	var scores []score
	for _, id := range epgIDs {
		idWords, idJoined := tokenize(id)
		if country := EPGIDCountry(id); country != "" && len(idWords) > 0 {
			// Don't let the country suffix leak into the brand ("Canal.4.de.Costa.Rica.cr"
			// must not answer to a "DE:" prefix).
			idJoined = strings.TrimSuffix(idJoined, country)
		}

		// HARD RULE: an ID that doesn't carry the brand is not a candidate at all.
		if !idCarriesBrand(forms, idWords, idJoined) {
			continue
		}

		matches := 0
		for _, w := range words {
			weight := 10
			if genericWords[w] || isNumeric(w) {
				weight = 1 // numbers/generic words barely count
			}
			for _, iw := range idWords {
				if w == iw {
					matches += weight
				} else if tokensRelated(w, iw) {
					matches += weight / 3
				}
			}
		}
		// A fused token in the ID ("4fun", "tvp3") is as strong as an exact word.
		for _, bg := range channelBigrams {
			for _, iw := range idWords {
				if bg == iw {
					matches += 10
				}
			}
		}
		// The "+1" timeshift feed is a different channel: a base channel must not prefer
		// its +1 variant ("ITV 1" -> ITV1.uk, not ITV1+1.uk) and a +1 channel must prefer
		// the +1 feed. Penalize disagreement so the numeric coincidence can't win.
		if channelTimeshift != hasTimeshift(id) {
			matches -= timeshiftPenalty
		}
		// A candidate that carries a distinguishing word the channel lacks is a more
		// specific, different channel — deprioritize it, and drop it when it is the only
		// option so we leave the channel unmapped rather than assign it wrongly.
		matches -= extraWordPenalty * extraSpecificWords(words, idWords, forms)
		// Different standalone numbers = a different channel in a numbered line-up.
		// When both sides carry numbers and none agree, discard outright ("Eurosport 7"
		// is never "Eurosport 1", "France 24" never "France 2"). A numbered id offered to
		// a number-less channel is merely penalized ("RTL" vs "RTL 2"), since some
		// flagships only exist in numbered form.
		if idNums := numericTokens(idWords); len(idNums) > 0 {
			if len(channelNums) > 0 && disjointNumbers(channelNums, idNums) {
				continue
			}
			if len(channelNums) == 0 {
				matches -= numberMismatchPenalty
			}
		}

		if matches >= minScore {
			scores = append(scores, score{id, matches, abs(len(id) - len(channelName))})
		}
	}

	sort.Slice(scores, func(i, j int) bool {
		if scores[i].matchCount == scores[j].matchCount {
			return scores[i].lengthDiff < scores[j].lengthDiff
		}
		return scores[i].matchCount > scores[j].matchCount
	})

	var result []string
	for i := 0; i < len(scores) && i < topN; i++ {
		result = append(result, scores[i].id)
	}
	return result
}

// ---- mapping-audit helpers (used by the daily EPG maintenance) ----

var qualityWords = map[string]bool{"hd": true, "uhd": true, "fhd": true, "sd": true, "4k": true}

// auditCovered reports whether an EPG-id word is accounted for by the channel name:
// contained in the fused channel name, a plural of a channel word (sports~sport), an
// extension of one (geographic~geo, itv1~itv), or a 1-char decoration (rtv~tv).
func auditCovered(iw string, cwords []string, cjoin string) bool {
	if strings.Contains(cjoin, iw) {
		return true
	}
	if strings.HasSuffix(iw, "s") && strings.Contains(cjoin, iw[:len(iw)-1]) {
		return true
	}
	for _, cw := range cwords {
		if len(cw) >= 3 && strings.HasPrefix(iw, cw) {
			return true
		}
		if len(cw) >= 2 && len(iw)-len(cw) == 1 && strings.Contains(iw, cw) {
			return true
		}
	}
	return false
}

// ExtraIDWords returns the distinguishing words an EPG id carries that the channel
// name does not account for. A non-empty result means the id names a different, more
// specific channel ("5 Select" vs "Sky.Cinema.Select.uk" -> [sky cinema]).
func ExtraIDWords(channel, id string) []string {
	cw, cj := tokenize(CleanChannelNameForMatching(channel))
	idW, _ := tokenize(id)
	if c := EPGIDCountry(id); c != "" && len(idW) > 0 && idW[len(idW)-1] == c {
		idW = idW[:len(idW)-1]
	}
	var out []string
	for _, iw := range idW {
		if len(iw) < 2 || genericWords[iw] || isNumeric(iw) || qualityWords[iw] || knownCountries[iw] {
			continue
		}
		if !auditCovered(iw, cw, cj) {
			out = append(out, iw)
		}
	}
	return out
}

// UncoveredChannelWords counts channel words (len>=2 so "f1" counts) the id does not carry.
func UncoveredChannelWords(channel, id string) int {
	cw, _ := tokenize(CleanChannelNameForMatching(channel))
	_, idJ := tokenize(id)
	if c := EPGIDCountry(id); c != "" {
		idJ = strings.TrimSuffix(idJ, c)
	}
	n := 0
	for _, w := range cw {
		if len(w) < 2 || genericWords[w] || isNumeric(w) || qualityWords[w] {
			continue
		}
		if !strings.Contains(idJ, w) &&
			!(strings.HasSuffix(w, "s") && strings.Contains(idJ, w[:len(w)-1])) {
			n++
		}
	}
	return n
}

// CleanReplacement finds an id that matches the channel with no judgement call left:
// country agrees, all channel words covered, no extra id words, and numbers agree
// (a channel whose only number is 1 may take a number-less id: Sky Arts 1 = Sky Arts).
func CleanReplacement(channel string, byCountry map[string][]string, inUniverse map[string]bool) string {
	hint := ExtractCountryHint(channel)
	if hint == "" {
		return ""
	}
	chWords, _ := tokenize(CleanChannelNameForMatching(channel))
	chNums := numericTokens(chWords)
	onlyOne := true
	for _, n := range chNums {
		if n != "1" {
			onlyOne = false
		}
	}
	for _, cand := range GetTopCandidatesForChannel(channel, byCountry[hint], 8, 10) {
		if !inUniverse[cand] || UncoveredChannelWords(channel, cand) != 0 || len(ExtraIDWords(channel, cand)) != 0 {
			continue
		}
		if len(chNums) > 0 {
			cw, _ := tokenize(cand)
			candNums := numericTokens(cw)
			if len(candNums) == 0 {
				if !onlyOne {
					continue
				}
			} else if disjointNumbers(chNums, candNums) {
				continue
			}
		}
		return cand
	}
	return ""
}
