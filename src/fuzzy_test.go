package main

import "testing"

// A representative slice of the real EPG universe.
var testEPGIDs = []string{
	"AMC.pl", "AMC.cz", "plex.tv.AMC.en.Español.plex",
	"4FUN.KIDS.pl", "4FUN.TV.pl", "4FUN.DANCE.pl",
	"Polsat.Sport.Premium.4.pl", "Polsat.Sport.HD.pl", "Polsat.1.pl",
	"13.Ulica.HD.pl", "AXN.pl", "AXN.Spin.pl", "AXN.Black.cz", "iTVN.pl",
	"TVN.24.pl", "TVN.pl", "TVN.7.pl",
	"TVP.1.HD.pl", "TVP.Info.pl", "TVP.HD.pl",
	"CANAL+.Sport.5.HD.pl", "CANAL+.EXTRA.3.HD.pl",
	"Eleven.Sports.4.pl", "IT:.Inazuma.Eleven.Collection.be",
	"Discovery.Channel.pl", "Discovery.Channel.(niem.).pl", "Disco.Polo.Music.pl",
	"Seznam.cz.TV.cz", "Nova.Cinema.cz",
	"n-tv.de", "NHK.World.TV.de", "Canal.4.de.Costa.Rica.cr",
	"SHf.-.Schaffhauser.Fernsehen.ch",
	"Sky.Documentaries.HD.uk", "SkySp.F1.HD.uk", "Faith.UK.uk",
	"DK:.Homes.Under.the.Hammer.be", "TNT.Sports.5.HD.uk",
	"Mezzo.pl", "tvregionalna.pl", "Sky.Sport.Top.Event.de",
	"Motorvision.TV.de", "BBC.R.Ulster.uk", "TV4.pl",
}

func topFor(channel string) []string {
	return GetTopCandidatesForChannel(channel, testEPGIDs, 7, 3)
}

func firstFor(channel string) string {
	if top := topFor(channel); len(top) > 0 {
		return top[0]
	}
	return ""
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestCleanChannelNameForMatching(t *testing.T) {
	cases := map[string]string{
		"PL-[EXTRA]: 4 FUN KIDS [1080p]":                                    "4 FUN KIDS",
		"CZ: AMC [1080p]":                                                   "AMC",
		"PLAY+: CANAL+ EXTRA 3 [1080p]":                                     "CANAL+ EXTRA 3",
		"BE: FUSSBALL.TV 2  [[4K-Q]-Q]":                                     "FUSSBALL.TV 2",
		"- THE EVENT HAS NOT BEGUN -:  : DE: DPLUS [PPV EVENTS] 10 [1080p]": "DPLUS 10",
		"UK: [24/7 O-D] RAMBO [1080p]":                                      "RAMBO",
		"DE: N-TV [H265]":                                                   "N-TV",
		"CZ - Nova Cinema":                                                  "Nova Cinema",
		"PL: [480p] TUNEBOX [720p]":                                         "TUNEBOX",
	}
	for in, want := range cases {
		if got := CleanChannelNameForMatching(in); got != want {
			t.Errorf("CleanChannelNameForMatching(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractCountryHint(t *testing.T) {
	cases := map[string]string{
		"PL-[EXTRA]: AXN [1080p]": "pl",
		"CZ: AMC [1080p]":         "cz",
		"UK-NOWTV: SKY DOCS":      "uk",
		"PRIME: MOTORVISION DE":   "", // 5 letters, not a country
		"PLAY+: CANAL+ EXTRA 3":   "", // 4 letters but "PLAY" is not 2 chars
		"SKY: MEZZO":              "", // 3 letters
		"L1: BURTON ALBION":       "", // not all letters
		"UKSKY SPORTS ACTION":     "",
		"Vidio EPL 12":            "",
	}
	for in, want := range cases {
		if got := ExtractCountryHint(in); got != want {
			t.Errorf("ExtractCountryHint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEPGIDCountry(t *testing.T) {
	cases := map[string]string{
		"13.Ulica.HD.pl":              "pl",
		"NBA.League.Pass.10.us2":      "us",
		"WUNC-DT.20.us_locals1":       "us",
		"plex.tv.AMC.en.Español.plex": "",
		"AMC.cz":                      "cz",
		"uk.ITV London":               "uk", // native prefix form
		"UK| ITV 1":                   "uk",
		"de: RTL":                     "de",
		"la.Las Estrellas":            "", // "la" is not a known country
		"tv.Some Channel":             "",
	}
	for in, want := range cases {
		if got := EPGIDCountry(in); got != want {
			t.Errorf("EPGIDCountry(%q) = %q, want %q", in, got, want)
		}
	}
}

// The bug this whole change exists to fix: a channel must never be handed an EPG ID
// from a different country when its own country has a match.
func TestCountryPrefixIsRespected(t *testing.T) {
	if got := firstFor("CZ: AMC [1080p]"); got != "AMC.cz" {
		t.Errorf("CZ: AMC mapped to %q, want AMC.cz", got)
	}
	if got := firstFor("PL: AMC [1080p]"); got != "AMC.pl" {
		t.Errorf("PL: AMC mapped to %q, want AMC.pl", got)
	}
	if top := topFor("CZ: AMC [1080p]"); contains(top, "AMC.pl") {
		t.Errorf("CZ: AMC was offered the Polish feed: %v", top)
	}
	// "Canal.4.de.Costa.Rica.cr" must not answer to a German "DE:" prefix via the
	// Spanish word "de".
	if top := topFor("DE: 4 BLOCKS S01 [4K-Q]"); len(top) != 0 {
		t.Errorf("DE: 4 BLOCKS should be unmapped, got %v", top)
	}
}

func TestBrandTokenAnchoring(t *testing.T) {
	// The exact hallucinations called out in epg_mapping_refactor_summary.md.
	if top := topFor("PL-[EXTRA]: 4 FUN KIDS [1080p]"); !contains(top, "4FUN.KIDS.pl") {
		t.Errorf("4 FUN KIDS did not find 4FUN.KIDS.pl, got %v", top)
	}
	if top := topFor("PL-[EXTRA]: 4 FUN KIDS [1080p]"); contains(top, "Polsat.Sport.Premium.4.pl") {
		t.Errorf("4 FUN KIDS was offered Polsat.Sport.Premium.4.pl: %v", top)
	}
	if top := topFor("CZ: AMC [1080p]"); contains(top, "Seznam.cz.TV.cz") {
		t.Errorf("CZ: AMC was offered Seznam.cz.TV.cz: %v", top)
	}
	if top := topFor("PL-[EXTRA]: AXN [1080p]"); contains(top, "tvregionalna.pl") {
		t.Errorf("AXN was offered tvregionalna.pl: %v", top)
	}
	// A brand may not bleed into a longer unrelated word.
	if top := topFor("DE: HAUS DES GELDES [1080p]"); contains(top, "SHf.-.Schaffhauser.Fernsehen.ch") {
		t.Errorf("HAUS matched Schaffhauser: %v", top)
	}
	if top := topFor("PL: DISCO POLO [1080p]"); contains(top, "Discovery.Channel.pl") {
		t.Errorf("DISCO matched Discovery: %v", top)
	}
	if top := topFor("UK: [F1]: HAM - HAMILTON MERCEDES [1080p]"); contains(top, "DK:.Homes.Under.the.Hammer.be") {
		t.Errorf("HAM matched Hammer: %v", top)
	}
	// iTVN is not TVN.
	if top := topFor("PL: TVN 24 [1080p]"); contains(top, "iTVN.pl") {
		t.Errorf("TVN 24 was offered iTVN.pl: %v", top)
	}
}

func TestBrandSurvivesSeparators(t *testing.T) {
	if got := firstFor("DE: N-TV [H265]"); got != "n-tv.de" {
		t.Errorf("DE: N-TV mapped to %q, want n-tv.de", got)
	}
	if got := firstFor("PL: 4FUN.TV [1080p]"); got != "4FUN.TV.pl" {
		t.Errorf("PL: 4FUN.TV mapped to %q, want 4FUN.TV.pl", got)
	}
	if got := firstFor("PL-[EXTRA]: 13 ULICA [1080p]"); got != "13.Ulica.HD.pl" {
		t.Errorf("13 ULICA mapped to %q, want 13.Ulica.HD.pl", got)
	}
	if got := firstFor("UK-NOWTV: SKY DOCUMENTARIES [1080p]"); got != "Sky.Documentaries.HD.uk" {
		t.Errorf("SKY DOCUMENTARIES mapped to %q", got)
	}
}

// VOD, PPV, 24/7 loops and fixtures are not channels and must never get a tvg-id.
func TestNonChannelsAreUnmapped(t *testing.T) {
	for _, ch := range []string{
		"UK: [24/7 O-D] RAMBO [1080p]",
		"DE: AGATHA ALL ALONG   [1080p]",
		"DE: 4 BLOCKS S01 [4K-Q]",
		"- THE EVENT HAS NOT BEGUN -:  : DE: DPLUS [PPV EVENTS] 10 [1080p]",
		"Vidio EPL 12  [1080p]",
		"UEFA : 01 - [1080p]",
		"5Action HD",
		"PL: WALKI 1 (LIVE EVENT ONLY) [1080p]",
		"PLAY+: EVENT 1 [1080p]",
		"PLAY+: EVENT 4 [1080p]",
		"UK: EFL-L1 REPLAY 2 [720p]",
	} {
		if top := topFor(ch); len(top) != 0 {
			t.Errorf("%q should be unmapped, got %v", ch, top)
		}
	}
}

// The offline fallback has no AI to veto it, so it must only accept same-country
// brand matches.
func TestGetBestCandidateStrict(t *testing.T) {
	if got := GetBestCandidateStrict("CZ: AMC [1080p]", testEPGIDs); got != "AMC.cz" {
		t.Errorf("strict CZ: AMC = %q, want AMC.cz", got)
	}
	// No AMC.uk exists -> refuse rather than hand over AMC.pl.
	if got := GetBestCandidateStrict("UK: AMC [1080p]", testEPGIDs); got != "" {
		t.Errorf("strict UK: AMC = %q, want \"\" (no UK feed exists)", got)
	}
	// No country prefix -> refuse.
	if got := GetBestCandidateStrict("SKY: MEZZO [1080p]", testEPGIDs); got != "" {
		t.Errorf("strict SKY: MEZZO = %q, want \"\"", got)
	}
	if got := GetBestCandidateStrict("ULSTER GAA 10 [1080p]", testEPGIDs); got != "" {
		t.Errorf("strict ULSTER GAA = %q, want \"\"", got)
	}
}

// The AI may only return an ID that was actually offered to that specific channel.
func TestParseJSONResponseRejectsUnofferedIDs(t *testing.T) {
	epg := map[string]string{"AMC.cz": "AMC", "AMC.pl": "AMC", "TVN.7.pl": "TVN 7"}
	offered := map[string]map[string]bool{
		"CZ: AMC":   {"AMC.cz": true},
		"PL: TVN 7": {"TVN.7.pl": true},
	}

	got, err := parseJSONResponse(`{"CZ: AMC":"AMC.pl","PL: TVN 7":"TVN7.pl"}`, epg, offered)
	if err != nil {
		t.Fatalf("parseJSONResponse: %v", err)
	}
	if _, ok := got["CZ: AMC"]; ok {
		t.Errorf("AMC.pl was accepted for CZ: AMC even though only AMC.cz was offered: %v", got)
	}
	// "TVN7.pl" is a near-miss of an offered ID and should be auto-corrected.
	if got["PL: TVN 7"] != "TVN.7.pl" {
		t.Errorf("TVN7.pl was not auto-corrected to the offered TVN.7.pl: %v", got)
	}

	// A channel the AI was never asked about must be ignored entirely.
	got, err = parseJSONResponse(`{"XX: GHOST":"AMC.cz"}`, epg, offered)
	if err != nil {
		t.Fatalf("parseJSONResponse: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("accepted a channel that was never offered: %v", got)
	}
}

// Real channels whose names merely contain a placeholder word must still map.
func TestPlaceholderWordsDoNotBreakRealBrands(t *testing.T) {
	ids := append([]string{"Mezzo.Live.HD.pl", "MTV.Live.pl"}, testEPGIDs...)
	if got := GetTopCandidatesForChannel("PL: MEZZO LIVE [720p]", ids, 7, 3); len(got) == 0 || got[0] != "Mezzo.Live.HD.pl" {
		t.Errorf("PL: MEZZO LIVE = %v, want Mezzo.Live.HD.pl first", got)
	}
}

// The EPG spells Polish channels with diacritics ("TVN.Fabuła.pl") while providers
// send ASCII ("TVN FABULA"). Without folding, the brand ties away to a sibling.
func TestDiacriticsAreFolded(t *testing.T) {
	ids := []string{
		"TVN.Fabuła.pl", "TVN.24.pl", "TVN.pl",
		"TVP.3.Wrocław.pl", "TVP3.Opole.cz", "TVP.3.Białystok.pl", "TVP3.Lublin.cz",
		"TVP.3.Rzeszów.pl",
	}
	cases := map[string]string{
		"PL: TVN FABULA [720p]":    "TVN.Fabuła.pl",
		"PL: TVP3 WROCLAW [1080p]": "TVP.3.Wrocław.pl",
		"PL: TVP3 BIALYSTOK":       "TVP.3.Białystok.pl",
		"PL: TVP3 RZESZOW":         "TVP.3.Rzeszów.pl",
	}
	for ch, want := range cases {
		top := GetTopCandidatesForChannel(ch, ids, 7, 3)
		if len(top) == 0 || top[0] != want {
			t.Errorf("%q -> %v, want %q first", ch, top, want)
		}
	}
}

// A channel may fuse tokens the EPG splits, or vice versa.
func TestFusedBrandTokens(t *testing.T) {
	ids := []string{
		"TVP.3.Wrocław.pl", "TVP.1.pl",
		"SkyShowtime.1.pl", "SkyShowtime.2.pl", "Sky.Sport.2.de",
		"4FUN.TV.pl", "Nova.Fun.cz",
		"n-tv.de",
	}
	cases := map[string]string{
		"PL: TVP3 WROCLAW":     "TVP.3.Wrocław.pl", // "tvp3" meets "TVP" + "3"
		"PL: SKY SHOWTIME 2":   "SkyShowtime.2.pl", // "sky showtime" meets "SkyShowtime"
		"PL: 4 FUN TV [1080p]": "4FUN.TV.pl",       // "4 fun" meets "4FUN"
		"DE: N-TV [H265]":      "n-tv.de",
	}
	for ch, want := range cases {
		top := GetTopCandidatesForChannel(ch, ids, 7, 3)
		if len(top) == 0 || top[0] != want {
			t.Errorf("%q -> %v, want %q first", ch, top, want)
		}
	}
	// Fusing must not let a brand bleed into an unrelated longer word.
	if top := GetTopCandidatesForChannel("PL: DISCO POLO", []string{"Discovery.Channel.pl"}, 7, 3); len(top) != 0 {
		t.Errorf("DISCO POLO matched Discovery: %v", top)
	}
}

func TestStopwordsDoNotAnchorBrands(t *testing.T) {
	ids := []string{"The.Box.uk", "The.War.Channel.us"}
	if top := GetTopCandidatesForChannel("PL: THE WAR [1080p]", ids, 7, 3); contains(top, "The.Box.uk") {
		t.Errorf("THE WAR anchored on \"the\" and matched The.Box.uk: %v", top)
	}
	// A channel genuinely named "The Box" still resolves via "box".
	if top := GetTopCandidatesForChannel("UK: THE BOX [1080p]", ids, 7, 3); len(top) == 0 || top[0] != "The.Box.uk" {
		t.Errorf("UK: THE BOX -> %v, want The.Box.uk", top)
	}
}

// PPV slots and fixtures must be rejected on the raw name, before bracket tags are
// stripped and a plausible-looking brand emerges.
func TestNonChannelMarkersRejectedBeforeCleaning(t *testing.T) {
	ids := []string{"Sony.Max.1.Tv.Channel.Today.in2", "Sky.Sports.Main.Event.HD.ie", "Cinemax.pl"}
	for _, ch := range []string{
		"LIVE: ISCO CHAMPIONSHIP: 1. DZIEŃ: Thu 09 Jul 22:00 CEST (PL):  : PL: MAX [PPV EVENTS] 5 [1080p]",
		"ENDED: GOODWOOD FESTIVAL OF SPEED: Thu 09 Jul 07:50 UTC (UK):  : UK: MAX [PPV EVENTS] 1 [1080p]",
		"UK: [24/7 O-D] RAMBO [1080p]",
		"PL: WALKI 1 (LIVE EVENT ONLY) [1080p]",
		"FA Player 06 : Portsmouth vs Sunderland // UK Sun 9 Feb 3:00pm [1080p]",
	} {
		if !IsNonChannelName(ch) {
			t.Errorf("IsNonChannelName(%q) = false, want true", ch)
		}
		if top := GetTopCandidatesForChannel(ch, ids, 7, 3); len(top) != 0 {
			t.Errorf("%q should be unmapped, got %v", ch, top)
		}
	}
	// Real channels must not be swept up by the markers.
	for _, ch := range []string{"PL: CINEMAX [1080p]", "PL: MEZZO LIVE [720p]", "UK: SKY SPORTS MAIN EVENT [1080p]"} {
		if IsNonChannelName(ch) {
			t.Errorf("IsNonChannelName(%q) = true, want false", ch)
		}
	}
}

// A base channel must map to its base feed, and a "+1" channel to its timeshift feed;
// the numeric coincidence in "ITV 1" vs "ITV1+1" must not flip them.
func TestTimeshiftAgreement(t *testing.T) {
	ids := []string{
		"ITV1.uk", "ITV1.HD.uk", "ITV1+1.uk", "ITV.Quiz.uk",
		"ITV2.uk", "ITV2+1.uk", "ITV3.uk", "ITV3+1.uk", "ITV4.uk", "ITV4+1.uk",
		"Comedy.Central.uk", "Comedy.Central.+1.uk",
	}
	cases := map[string]string{
		"UK: ITV 1 [720p]":              "ITV1.uk",
		"UK: ITV 2 [720p]":              "ITV2.uk",
		"UK: ITV 4 [720p]":              "ITV4.uk",
		"UK: ITV 1 +1 [1080p]":          "ITV1+1.uk",
		"UK: ITV 2+1 [1080p]":           "ITV2+1.uk",
		"UK: ITV 4+1 [1080p]":           "ITV4+1.uk",
		"UK: COMEDY CENTRAL [1080p]":    "Comedy.Central.uk",
		"UK: COMEDY CENTRAL +1 [1080p]": "Comedy.Central.+1.uk",
	}
	for ch, want := range cases {
		top := GetTopCandidatesForChannel(ch, ids, 5, 3)
		if len(top) == 0 || top[0] != want {
			t.Errorf("%q -> %v, want %q first", ch, top, want)
		}
	}
}

func TestHasTimeshift(t *testing.T) {
	yes := []string{"ITV1+1.uk", "UK: ITV 2+1", "Comedy Central +1", "itv1-london-plus-1", "Sport Plus 1"}
	no := []string{"ITV1.uk", "UK: ITV 1", "CANAL+ SPORT", "Channel 41", "ITV 4"}
	for _, s := range yes {
		if !hasTimeshift(s) {
			t.Errorf("hasTimeshift(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if hasTimeshift(s) {
			t.Errorf("hasTimeshift(%q) = true, want false", s)
		}
	}
}

// A plain channel must not be assigned a more-specific variant's id. The correct match
// (which shares the channel's words) is never penalized; only candidates carrying a
// distinguishing word the channel lacks are pushed down, and dropped when they are the
// only option so the channel is left unmapped rather than wrongly assigned.
func TestExtraSpecificWordNotMatched(t *testing.T) {
	ids := []string{
		"HBO.pl", "HBO.Comedy.pl", "HBO.2.pl",
		"AXN.pl", "AXN.Spin.pl",
		"Discovery.Channel.pl", "Discovery.Science.pl", "Discovery.Historia.pl",
		"MTV.pl", "MTV.Hits.pl",
	}
	// exact base channel wins over its more-specific siblings
	mustFirst := map[string]string{
		"PL: HBO [1080p]":        "HBO.pl",
		"PL: AXN [1080p]":        "AXN.pl",
		"PL: MTV [1080p]":        "MTV.pl",
		"PL: HBO COMEDY [1080p]": "HBO.Comedy.pl", // when the channel IS the variant, it still matches
		"PL: AXN SPIN [1080p]":   "AXN.Spin.pl",
	}
	for ch, want := range mustFirst {
		top := GetTopCandidatesForChannel(ch, ids, 5, 3)
		if len(top) == 0 || top[0] != want {
			t.Errorf("%q -> %v, want %q first", ch, top, want)
		}
	}
	// a specific variant must never be offered for the plain channel
	for _, bad := range []struct{ ch, notWanted string }{
		{"PL: HBO [1080p]", "HBO.Comedy.pl"},
		{"PL: AXN [1080p]", "AXN.Spin.pl"},
		{"PL: MTV [1080p]", "MTV.Hits.pl"},
	} {
		if top := GetTopCandidatesForChannel(bad.ch, ids, 5, 3); contains(top, bad.notWanted) {
			t.Errorf("%q was offered %q: %v", bad.ch, bad.notWanted, top)
		}
	}
	// when only a differently-specific channel is available, leave it unmapped
	if top := GetTopCandidatesForChannel("UK: ITV LONDON [1080p]", []string{"ITV.Quiz.uk"}, 5, 3); len(top) != 0 {
		t.Errorf("ITV LONDON should be unmapped when only ITV.Quiz.uk exists, got %v", top)
	}
	// "Discovery" -> "Discovery.Channel" (Channel is generic, not a distinguishing word)
	if top := GetTopCandidatesForChannel("PL: DISCOVERY [1080p]", ids, 5, 3); len(top) == 0 || top[0] != "Discovery.Channel.pl" {
		t.Errorf("PL: DISCOVERY -> %v, want Discovery.Channel.pl", top)
	}
}

// A channel's standalone number must match: a numbered feed with no EPG id of its own
// must not fall onto a different-numbered sibling.
func TestNumberMismatchNotMatched(t *testing.T) {
	ids := []string{"Eurosport.1.pl", "Eurosport.2.pl", "France.2.de", "France.3.de", "France.24.de"}
	// exact number wins
	if top := GetTopCandidatesForChannel("PL: EUROSPORT 1 [1080p]", ids, 5, 3); len(top) == 0 || top[0] != "Eurosport.1.pl" {
		t.Errorf("EUROSPORT 1 -> %v, want Eurosport.1.pl", top)
	}
	// a number with no matching id must not fall onto a sibling number
	if top := GetTopCandidatesForChannel("PL: EUROSPORT 7 [1080p]", ids, 5, 3); contains(top, "Eurosport.1.pl") || contains(top, "Eurosport.2.pl") {
		t.Errorf("EUROSPORT 7 wrongly matched a sibling: %v", top)
	}
	// France 24 must find France.24, never France.2/France.3
	if top := GetTopCandidatesForChannel("DE: FRANCE 24 [1080p]", ids, 5, 3); len(top) == 0 || top[0] != "France.24.de" {
		t.Errorf("FRANCE 24 -> %v, want France.24.de", top)
	}
	if top := GetTopCandidatesForChannel("DE: FRANCE 24 FAST [1080p]", []string{"France.2.de", "France.3.de"}, 5, 3); len(top) != 0 {
		t.Errorf("FRANCE 24 FAST should be unmapped when only France.2/3 exist, got %v", top)
	}
}

// Numbers hidden in fused quality tokens ("1HD") and numbered ids offered to
// number-less channels are both number mismatches.
func TestNumberRuleEdgeCases(t *testing.T) {
	// fused number+quality in the id
	ids := []string{"Cytavision.Sports.1HD.cy", "Cytavision.Sports.5HD.cy"}
	if top := GetTopCandidatesForChannel("CY: CYTAVISION SPORTS 5 [720p]", ids, 3, 3); len(top) == 0 || top[0] != "Cytavision.Sports.5HD.cy" {
		t.Errorf("CYTAVISION SPORTS 5 -> %v, want 5HD first", top)
	}
	if top := GetTopCandidatesForChannel("CY: CYTAVISION SPORTS 7 [720p]", ids, 3, 3); contains(top, "Cytavision.Sports.1HD.cy") {
		t.Errorf("CYTAVISION SPORTS 7 wrongly offered 1HD: %v", top)
	}
	// a number-less channel must prefer the number-less id
	ids2 := []string{"RTL.rs", "RTL.2.rs"}
	if top := GetTopCandidatesForChannel("RS: RTL [1080p]", ids2, 3, 3); len(top) == 0 || top[0] != "RTL.rs" {
		t.Errorf("RS: RTL -> %v, want RTL.rs first", top)
	}
	// numbered channel still matches its numbered id
	if top := GetTopCandidatesForChannel("RS: RTL 2 [1080p]", ids2, 3, 3); len(top) == 0 || top[0] != "RTL.2.rs" {
		t.Errorf("RS: RTL 2 -> %v, want RTL.2.rs first", top)
	}
}

// Digits hidden in short tokens ("R4"), parenthesised qualifiers ("(HD)") and
// dash-spelled brands ("E-X-X-E-N") must not derail matching.
func TestTokenizationEdgeCases(t *testing.T) {
	// BBC Radio 2 must not land on BBC.R4.LW (the 4 in "R4" disagrees with 2)
	ids := []string{"BBC.R4.LW.uk", "BBC.R2.uk"}
	if top := GetTopCandidatesForChannel("UK: BBC RADIO 2 [1080p]", ids, 3, 3); contains(top, "BBC.R4.LW.uk") {
		t.Errorf("BBC RADIO 2 offered BBC.R4.LW.uk: %v", top)
	}
	if top := GetTopCandidatesForChannel("UK: BBC RADIO 2 [1080p]", ids, 3, 3); len(top) == 0 || top[0] != "BBC.R2.uk" {
		t.Errorf("BBC RADIO 2 -> %v, want BBC.R2.uk", top)
	}
	// "(HD)" and "(RS)" are separators, not distinguishing words
	if top := GetTopCandidatesForChannel("SG: BEIN SPORTS [720p]", []string{"beIN.SPORTS.(HD).sg"}, 3, 3); len(top) == 0 {
		t.Errorf("BEIN SPORTS did not match beIN.SPORTS.(HD).sg")
	}
	// dash-spelled brand collapses
	if got := CleanChannelNameForMatching("TR: E-X-X-E-N SPOR 4 [720p]"); got != "EXXEN SPOR 4" {
		t.Errorf("dash-spelled clean = %q, want \"EXXEN SPOR 4\"", got)
	}
	// EXXEN must not fall onto HT.SPOR (brand exxen missing there)
	if top := GetTopCandidatesForChannel("TR: E-X-X-E-N SPOR 4 [720p]", []string{"HT.SPOR.HD.tr"}, 3, 3); len(top) != 0 {
		t.Errorf("EXXEN SPOR wrongly matched HT.SPOR: %v", top)
	}
	// but a real country suffix with digits ("us2") is not a channel number
	if top := GetTopCandidatesForChannel("US: CLEO TV [1080p]", []string{"Cleo.TV.HD.us2"}, 3, 3); len(top) == 0 {
		t.Errorf("CLEO TV did not match Cleo.TV.HD.us2")
	}
}
