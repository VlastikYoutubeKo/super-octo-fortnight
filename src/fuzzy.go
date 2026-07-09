package main

import (
	"strings"
	"sort"
)

// GetTopCandidates finds the top N closest EPG IDs for a given channel name based on word intersection
func GetTopCandidates(channelName string, epgIDs []string, topN int, minScore int) []string {
	// tokenize channel name
	cleanName := strings.ToLower(channelName)
	cleanName = strings.ReplaceAll(cleanName, "-", " ")
	cleanName = strings.ReplaceAll(cleanName, "_", " ")
	cleanName = strings.ReplaceAll(cleanName, ".", " ")
	cleanName = strings.ReplaceAll(cleanName, ":", " ")
	
	words := strings.Fields(cleanName)
	if len(words) == 0 {
		if len(epgIDs) > topN {
			return epgIDs[:topN]
		}
		return epgIDs
	}

	type score struct {
		id string
		matchCount int
		lengthDiff int
	}

	var scores []score
	for _, id := range epgIDs {
		idLower := strings.ToLower(id)
		idLower = strings.ReplaceAll(idLower, ".", " ")
		idWords := strings.Fields(idLower)
		
		matches := 0
		for _, w := range words {
			for _, iw := range idWords {
				if w == iw {
					matches += 10
				} else if strings.HasPrefix(iw, w) || strings.HasPrefix(w, iw) {
					// partial match
					if len(w) > 2 && len(iw) > 2 {
						matches += 3
					}
				}
			}
		}
		
		// Require at least minScore match score
		if matches >= minScore {
			lenDiff := len(idLower) - len(cleanName)
			if lenDiff < 0 {
				lenDiff = -lenDiff
			}
			scores = append(scores, score{id, matches, lenDiff})
		}
	}

	// Sort by highest match count, then by smallest length difference
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
