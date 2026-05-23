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
