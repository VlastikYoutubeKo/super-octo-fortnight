package main

import "testing"

// The janitor must not undo human decisions. The motivating case: CS FILM shares its
// broadcast slot with CS Horror overnight, so the only 24h guide for it is the
// combined id CS.Film-CS.Horror.HD.sk. ExtraIDWords sees "horror" as an extra word
// and would re-point it back to the CS-Film-only guide (06:00-22:00, night missing).
func TestPlanSanitizeKeepsPinned(t *testing.T) {
	universe := map[string]bool{
		"CS.Film.cz":              true,
		"CS.Film-CS.Horror.HD.sk": true,
	}
	byCountry := map[string][]string{
		"cz": {"CS.Film.cz"},
		"sk": {"CS.Film-CS.Horror.HD.sk"},
	}
	snapshot := map[string]string{
		"CZ: CS FILM  [1080p]": "CS.Film-CS.Horror.HD.sk",
	}

	// unpinned: the sanitizer rightly considers the combined id "too specific"
	repoint, del := planSanitize(snapshot, map[string]string{}, universe, byCountry)
	if len(repoint) == 0 && len(del) == 0 {
		t.Fatal("expected the unpinned combined id to be re-pointed or deleted")
	}

	// pinned: left strictly alone
	pins := map[string]string{"CZ: CS FILM  [1080p]": "CS.Film-CS.Horror.HD.sk"}
	repoint, del = planSanitize(snapshot, pins, universe, byCountry)
	if len(repoint) != 0 || len(del) != 0 {
		t.Fatalf("pinned mapping must survive maintenance, got repoint=%v del=%v", repoint, del)
	}
}

// A pin is not a licence to break invariant 2: an id TVHeadend has not loaded must
// never reach the mapping, pinned or not.
func TestApplyPinsRejectsUnloadedID(t *testing.T) {
	EPGPins = map[string]string{
		"CZ: CS FILM  [1080p]": "CS.Film-CS.Horror.HD.sk",
		"CZ: GHOST CHANNEL":    "Not.Loaded.cz",
	}
	EPGMapping = map[string]string{}
	defer func() {
		EPGPins = map[string]string{}
		EPGMapping = map[string]string{}
	}()

	applied, unloaded := applyPins(map[string]bool{"CS.Film-CS.Horror.HD.sk": true})
	if applied != 1 {
		t.Fatalf("expected 1 pin applied, got %d", applied)
	}
	if got := EPGMapping["CZ: CS FILM  [1080p]"]; got != "CS.Film-CS.Horror.HD.sk" {
		t.Fatalf("loaded pin not applied, mapping=%q", got)
	}
	if _, ok := EPGMapping["CZ: GHOST CHANNEL"]; ok {
		t.Fatal("pin to an unloaded id must not be written into the mapping")
	}
	if len(unloaded) != 1 || unloaded[0] != "CZ: GHOST CHANNEL" {
		t.Fatalf("stale pin should be reported, got %v", unloaded)
	}
}

// Pins must not become a hiding place for garbage: an id that is loaded but that the
// sanitizer would otherwise delete is kept only because a human asked for it, so the
// non-pinned entries around it must still be cleaned.
func TestPlanSanitizeStillCleansUnpinnedNeighbours(t *testing.T) {
	universe := map[string]bool{"CS.Film-CS.Horror.HD.sk": true, "CS.Film.cz": true}
	byCountry := map[string][]string{"cz": {"CS.Film.cz"}}
	snapshot := map[string]string{
		"CZ: CS FILM  [1080p]":    "CS.Film-CS.Horror.HD.sk", // pinned
		"UK: [24/7 O-D] RAMBO":    "CS.Film.cz",              // VOD junk
		"CZ: DANGLING [1080p]":    "Gone.From.Universe.cz",   // dangling
		"CZ: CS HISTORY  [1080p]": "CS.Film.cz",              // wrong channel
	}
	pins := map[string]string{"CZ: CS FILM  [1080p]": "CS.Film-CS.Horror.HD.sk"}

	repoint, del := planSanitize(snapshot, pins, universe, byCountry)
	if _, ok := repoint["CZ: CS FILM  [1080p]"]; ok {
		t.Fatal("pinned entry must not be re-pointed")
	}
	for _, ch := range del {
		if ch == "CZ: CS FILM  [1080p]" {
			t.Fatal("pinned entry must not be deleted")
		}
	}
	if len(del) == 0 {
		t.Fatal("expected the VOD/dangling entries to still be cleaned")
	}
}
