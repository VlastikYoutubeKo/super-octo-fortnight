package main

import (
	"regexp"
	"strings"
)

func NormalizeChannelName(name string) string {
	if name == "" {
		return ""
	}

	// Clean name for better search (remove common quality markers and symbols)
	cleanName := name
	
	// Remove country prefixes like "PL|", "CZ -", "UK:"
	rePrefix := regexp.MustCompile(`(?i)^[a-z]{2,3}\s*[:|-]\s*`)
	cleanName = rePrefix.ReplaceAllString(cleanName, "")
	
	// Remove tags in brackets/parentheses like [720p], (FHD), [CZ]
	reTags := regexp.MustCompile(`(?i)\[.*?\]|\(.*?\)|<.*?>`)
	cleanName = reTags.ReplaceAllString(cleanName, "")
	
	// Remove quality and resolution markers that might not be in brackets
	reQuality := regexp.MustCompile(`(?i)\b(FHD|HD|SD|4K|UHD|1080p|720p|480p|1080|720|HEVC)\b`)
	cleanName = reQuality.ReplaceAllString(cleanName, "")
	
	// Remove remaining extra characters and replace with space
	reChars := regexp.MustCompile(`[._|:,\-]`)
	cleanName = reChars.ReplaceAllString(cleanName, " ")
	
	// Collapse multiple spaces
	cleanName = strings.Join(strings.Fields(cleanName), " ")
	
	// If the name is too short after cleaning, use the original
	if len(cleanName) < 2 {
		cleanName = strings.ToLower(name)
	} else {
		cleanName = strings.ToLower(cleanName)
	}

	return cleanName
}

func CleanChannelNameForM3U(name string) string {
	if name == "" {
		return ""
	}
	clean := name

	// Remove anything in brackets or parentheses e.g. [1080p], (FHD), [EXTRA]
	reTags := regexp.MustCompile(`(?i)\[.*?\]|\(.*?\)|<.*?>`)
	clean = reTags.ReplaceAllString(clean, "")

	// Fix up leftover artifacts like "PL-: " or "PL - :"
	clean = strings.ReplaceAll(clean, "-:", "-")
	clean = strings.ReplaceAll(clean, ":-", "-")
	clean = strings.ReplaceAll(clean, "::", ":")
	clean = strings.ReplaceAll(clean, "--", "-")

	// Collapse multiple spaces so the next regex works predictably
	clean = strings.Join(strings.Fields(clean), " ")

	// Standardize Country/Category prefixes
	// Matches 2-3 letters at start, followed by combinations of spaces, dashes, or colons
	// Example: "PL-", "PL: ", "CZ - ", "UK:" -> "PL - ", "CZ - ", "UK - "
	rePrefix := regexp.MustCompile(`^([A-Za-z]{2,3})\s*[:-]\s*`)
	clean = rePrefix.ReplaceAllString(clean, "$1 - ")

	// Trim redundant spaces and colons/hyphens at the start/end
	clean = strings.TrimSpace(clean)
	clean = strings.Trim(clean, ":- ")
	
	// Collapse multiple spaces again
	clean = strings.Join(strings.Fields(clean), " ")
	
	if clean == "" {
		return name
	}
	return clean
}

// IsCategorySeparator detects dummy channels used by IPTV providers as visual separators in apps.
func IsCategorySeparator(name string) bool {
	// Look for repetitive ascii chars (===, ***, ---) or special unicode stars/blocks
	reSeparator := regexp.MustCompile(`([=*\-~|]{3,}|[✶⋆★☆✦✧✩✪✫✬✭✮✯✰═░▒▓█])`)
	if reSeparator.MatchString(name) {
		return true
	}

	clean := strings.TrimSpace(name)
	if len(clean) > 2 {
		first := clean[0]
		last := clean[len(clean)-1]
		// e.g. "= SPORT =" or "- NEWS -"
		if (first == '=' && last == '=') || 
		   (first == '-' && last == '-') ||
		   (first == '~' && last == '~') {
			return true
		}
	}

	return false
}
