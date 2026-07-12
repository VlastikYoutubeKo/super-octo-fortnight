package main

import (
	"fmt"
	"regexp"
	"strings"
)

func CleanChannelNameForM3U(name string, stripPrefix bool) string {
	if name == "" {
		return ""
	}
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
	fmt.Println(CleanChannelNameForM3U("PL - ZŁOTE PRZEBOJE", true))
}