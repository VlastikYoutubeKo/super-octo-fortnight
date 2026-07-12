package main

import (
	"fmt"
	"regexp"
	"strings"
)

func NormalizeChannelName(name string) string {
	if name == "" {
		return ""
	}
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
	
	cleanName = strings.Join(strings.Fields(cleanName), " ")
	
	if len(cleanName) < 2 {
		cleanName = strings.ToLower(name)
	} else {
		cleanName = strings.ToLower(cleanName)
	}

	return cleanName
}

func CleanChannelNameForM3U(name string, stripPrefix bool) string {
	clean := name
	reTags := regexp.MustCompile(`(?i)\[.*?\]|\(.*?\)|<.*?>`)
	clean = reTags.ReplaceAllString(clean, "")
	clean = strings.ReplaceAll(clean, "-:", "-")
	clean = strings.ReplaceAll(clean, ":-", "-")
	clean = strings.ReplaceAll(clean, "::", ":")
	clean = strings.ReplaceAll(clean, "--", "-")
	clean = strings.Join(strings.Fields(clean), " ")
	rePrefix := regexp.MustCompile(`^([A-Za-z]{2,3})\s*[:-]\s*`)
	if stripPrefix {
		clean = rePrefix.ReplaceAllString(clean, "")
	} else {
		clean = rePrefix.ReplaceAllString(clean, "$1 - ")
	}
	clean = strings.TrimSpace(clean)
	clean = strings.Trim(clean, ":- ")
	clean = strings.Join(strings.Fields(clean), " ")
	if clean == "" {
		return name
	}
	return clean
}

func main() {
	names := []string{
		"CZ - PLAY: CT 1",
		"CZ - SKY: NOVA",
	}
	for _, n := range names {
		fmt.Printf("Original: %s\n", n)
		fmt.Printf("Normalized (for EPG): %s\n", NormalizeChannelName(n))
		fmt.Printf("M3U Cleaned (strip=1): %s\n", CleanChannelNameForM3U(n, true))
		fmt.Println("---")
	}
}
