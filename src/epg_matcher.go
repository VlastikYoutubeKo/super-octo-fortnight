package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// FetchEPGChannelIDs downloads EPG channels. Supports .xml, .xml.gz, .txt, or a directory URL (ends with /) to scrape all .txt files.
func FetchEPGChannelIDs(epgUrl string) (map[string]string, error) {
	if strings.HasSuffix(epgUrl, "/") {
		return fetchAllTxtFromDirectory(epgUrl)
	}
	if strings.HasSuffix(epgUrl, ".txt") {
		return fetchTxtEPG(epgUrl)
	}
	return fetchXmlEPG(epgUrl)
}

func fetchTxtEPG(url string) (map[string]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	channels := make(map[string]string)
	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "--") && !strings.HasPrefix(line, "202") {
			channels[line] = line
		}
	}
	return channels, nil
}

func fetchAllTxtFromDirectory(dirUrl string) (map[string]string, error) {
	resp, err := http.Get(dirUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)
	channels := make(map[string]string)

	// simple manual parsing for href="...txt"
	parts := strings.Split(html, `href="`)
	for i := 1; i < len(parts); i++ {
		endIdx := strings.Index(parts[i], `"`)
		if endIdx != -1 {
			link := parts[i][:endIdx]
			if strings.HasSuffix(link, ".txt") {
				// download this txt
				txtMap, err := fetchTxtEPG(dirUrl + link)
				if err == nil {
					for k, v := range txtMap {
						channels[k] = v
					}
				}
			}
		}
	}
	return channels, nil
}

func fetchXmlEPG(epgUrl string) (map[string]string, error) {
	resp, err := http.Get(epgUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to fetch EPG: HTTP %d", resp.StatusCode)
	}

	var reader io.Reader = resp.Body
	if strings.HasSuffix(epgUrl, ".gz") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	}

	decoder := xml.NewDecoder(reader)
	channels := make(map[string]string)

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch se := token.(type) {
		case xml.StartElement:
			if se.Name.Local == "channel" {
				var ch struct {
					ID          string   `xml:"id,attr"`
					DisplayName []string `xml:"display-name"`
				}
				if err := decoder.DecodeElement(&ch, &se); err == nil {
					name := ch.ID
					if len(ch.DisplayName) > 0 && ch.DisplayName[0] != "" {
						name = ch.DisplayName[0]
					}
					channels[ch.ID] = name
				}
			}
		}
	}

	return channels, nil
}

func FetchTVHeadendEPGChannels() (map[string]string, error) {
	configLock.RLock()
	tvh := Config.TVHeadend
	configLock.RUnlock()

	if tvh.URL == "" {
		return nil, fmt.Errorf("TVHeadend URL is not configured")
	}

	// TVHeadend grids default to 50 rows without an explicit limit
	url := strings.TrimRight(tvh.URL, "/") + "/api/epggrab/channel/grid?limit=100000"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if tvh.Username != "" {
		req.SetBasicAuth(tvh.Username, tvh.Password)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("TVHeadend API returned %d", resp.StatusCode)
	}

	var result struct {
		Entries []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"entries"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	channels := make(map[string]string)
	for _, entry := range result.Entries {
		if entry.ID != "" {
			channels[entry.ID] = entry.Name
		}
	}

	return channels, nil
}

type GeminiRequest struct {
	Contents []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// MatchWithGemini asks the Gemini API to match unmapped channels with available EPG IDs.
func MatchWithGemini(apiKeys []string, unmappedChannels []string, epgChannels map[string]string) (map[string]string, error) {
	if len(apiKeys) == 0 || (len(apiKeys) == 1 && apiKeys[0] == "") {
		return nil, fmt.Errorf("gemini API key is empty")
	}

	// Instead of dumping 26,000+ channels, pre-filter locally using GetTopCandidates
	var allIDs []string
	for id := range epgChannels {
		allIDs = append(allIDs, id)
	}

	var matchTasks []string
	offered := make(map[string]map[string]bool)
	for _, ch := range unmappedChannels {
		topIDs := GetTopCandidatesForChannel(ch, allIDs, 7, 3)
		if len(topIDs) == 0 {
			// no candidate passed brand check -> skip AI entirely, mark unmapped
			continue
		}
		var opts []string
		offered[ch] = make(map[string]bool, len(topIDs))
		for _, id := range topIDs {
			offered[ch][id] = true
			opts = append(opts, fmt.Sprintf("%s (ID: %s)", epgChannels[id], id))
		}
		matchTasks = append(matchTasks, fmt.Sprintf("IPTV Channel: '%s'\nAvailable Options:\n%s\n", ch, strings.Join(opts, "\n")))
	}

	if len(matchTasks) == 0 {
		return make(map[string]string), nil
	}

	prompt := fmt.Sprintf(`You are an extremely strict TV channel EPG assigner. 
For each IPTV channel, I will provide a short list of 7 candidate EPG options. 
Please select the exact matching EPG ID from its options. If NONE are a perfect logical match, YOU MUST LEAVE IT EMPTY.

CRITICAL RULES:
1. You MUST select the "ID" EXACTLY as it appears in the options for that specific channel. 
2. Do not invent, guess, or alter IDs. 
3. Return ONLY a valid JSON object where the key is the exact "IPTV Channel" and the value is the exact matched "ID".
4. If a specific regional channel is requested (e.g., 'ITV CENTRAL WEST' or 'BBC One North West') and its exact EPG ID is not in the options, you MUST match it to its national version if available (e.g., 'ITV 1', 'BBC One', 'ITV.uk', 'ITV1.uk', etc.).
5. If NO option is a logical match, YOU MUST LEAVE IT EMPTY. DO NOT GUESS!
6. NEVER match based only on shared numbers or country codes! (e.g., '4 FUN KIDS' is NOT 'Polsat.Sport.Premium.4.pl' just because they both have a '4' and '.pl'. 'DISCOVERY' is NOT 'Discovery.Channel.(niem.).pl' if you know 'niem' means German. 'AXN' is NOT 'tvregionalna.pl'). If the brand name does not match, RETURN AN EMPTY STRING "".
7. The country prefix of the IPTV channel ('CZ:', 'PL:', 'DE:') MUST agree with the country suffix of the EPG ID ('.cz', '.pl', '.de'). 'CZ: AMC' is NOT 'AMC.pl' - it is a Czech feed, not a Polish one. If no option shares the channel's country, RETURN AN EMPTY STRING "".
8. Video-on-demand, PPV, 24/7 movie/series loops and sporting fixtures (e.g. 'DE: AGATHA ALL ALONG', 'UK: [24/7 O-D] RAMBO', 'PPV EVENTS 12', 'Team A vs Team B') are NOT TV channels. They have no EPG. RETURN AN EMPTY STRING "".

Channels to match:
%s`, strings.Join(matchTasks, "\n"))

	reqBody := GeminiRequest{}
	reqBody.Contents = append(reqBody.Contents, struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}{
		Parts: []struct {
			Text string `json:"text"`
		}{
			{Text: prompt},
		},
	})

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	var responseText string
	var lastErr error

	for _, apiKey := range apiKeys {
		if apiKey == "" {
			continue
		}

		var keyPreview string
		if len(apiKey) > 8 {
			keyPreview = "..." + apiKey[len(apiKey)-4:]
		} else {
			keyPreview = apiKey
		}

		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-3.5-flash:generateContent?key=%s", apiKey)
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			lastErr = err
			log.Printf("Gemini API request failed for key %s: %v", keyPreview, err)
			continue
		}

		if resp.StatusCode != 200 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(bodyBytes))
			log.Printf("Gemini API error for key %s: %v", keyPreview, lastErr)
			continue
		}

		var geminiResp GeminiResponse
		if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
			resp.Body.Close()
			lastErr = err
			continue
		}
		resp.Body.Close()

		if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
			lastErr = fmt.Errorf("empty response from Gemini")
			continue
		}

		responseText = geminiResp.Candidates[0].Content.Parts[0].Text
		break // Success!
	}

	if responseText == "" {
		return nil, fmt.Errorf("all Gemini API keys failed. Last error: %v", lastErr)
	}

	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	log.Printf("Gemini AI Match Response:\n%s\n", responseText)

	return parseJSONResponse(responseText, epgChannels, offered)
}

func normalizeEPGID(id string) string {
	id = strings.ReplaceAll(id, ".", "")
	id = strings.ReplaceAll(id, "-", "")
	return strings.ToLower(id)
}

// parseJSONResponse processes the raw JSON string from AI models. offered maps each
// channel to the exact set of IDs it was shown; an ID outside that set is a
// hallucination (or an option stolen from another channel) and is rejected.
func parseJSONResponse(responseText string, epgChannels map[string]string, offered map[string]map[string]bool) (map[string]string, error) {
	var parsedResult map[string]string
	if err := json.Unmarshal([]byte(responseText), &parsedResult); err != nil {
		log.Printf("Failed to parse AI JSON: %s", responseText)
		return nil, fmt.Errorf("failed to parse JSON from AI: %v", err)
	}

	result := make(map[string]string)
	for rawName, matchedID := range parsedResult {
		if matchedID == "" {
			continue
		}

		allowed, hasOptions := offered[rawName]
		if !hasOptions {
			log.Printf("AI returned a channel it was never asked about: %q", rawName)
			continue
		}

		// 1. Strict match against the options this channel was actually offered
		if allowed[matchedID] {
			result[rawName] = matchedID
			continue
		}

		// 2. Auto-Correction for near-miss hallucinations (e.g. "TVN7.pl" -> "TVN.7.pl"),
		//    still constrained to the offered options.
		cleanID := normalizeEPGID(matchedID)
		bestMatch := ""
		for actualID := range allowed {
			if cleanID == normalizeEPGID(actualID) {
				bestMatch = actualID
				break
			}
		}

		if bestMatch != "" {
			log.Printf("Auto-corrected AI hallucination: %s -> %s", matchedID, bestMatch)
			result[rawName] = bestMatch
		} else {
			log.Printf("Rejected AI EPG ID %q for %q: not among the offered options", matchedID, rawName)
		}
	}

	return result, nil
}
