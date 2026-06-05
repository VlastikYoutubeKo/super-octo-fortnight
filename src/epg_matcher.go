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
	for _, ch := range unmappedChannels {
		topIDs := GetTopCandidates(ch, allIDs, 15)
		var opts []string
		for _, id := range topIDs {
			opts = append(opts, fmt.Sprintf("%s (ID: %s)", epgChannels[id], id))
		}
		matchTasks = append(matchTasks, fmt.Sprintf("IPTV Channel: '%s'\nAvailable Options:\n%s\n", ch, strings.Join(opts, "\n")))
	}

	prompt := fmt.Sprintf(`You are a smart TV channel EPG assigner. 
For each IPTV channel, I will provide a short list of 15 candidate EPG options. 
Please select the best matching EPG ID from its options. If none match, leave it empty.

CRITICAL RULES:
1. You MUST select the "ID" EXACTLY as it appears in the options for that specific channel. 
2. Do not invent, guess, or alter IDs. 
3. Return ONLY a valid JSON object where the key is the exact "IPTV Channel" and the value is the exact matched "ID".

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

	return parseJSONResponse(responseText, epgChannels)
}

// parseJSONResponse processes the raw JSON string from AI models
func parseJSONResponse(responseText string, epgChannels map[string]string) (map[string]string, error) {
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
		
		// 1. Strict match
		if _, ok := epgChannels[matchedID]; ok {
			result[rawName] = matchedID
			continue
		}

		// 2. Auto-Correction / Fuzzy Match fallback for hallucinations
		cleanID := strings.ReplaceAll(matchedID, ".", "")
		cleanID = strings.ReplaceAll(cleanID, "-", "")
		cleanID = strings.ToLower(cleanID)

		bestMatch := ""
		for actualID := range epgChannels {
			cleanActual := strings.ReplaceAll(actualID, ".", "")
			cleanActual = strings.ReplaceAll(cleanActual, "-", "")
			cleanActual = strings.ToLower(cleanActual)
			if cleanID == cleanActual {
				bestMatch = actualID
				break
			}
		}

		if bestMatch != "" {
			log.Printf("Auto-corrected AI hallucination: %s -> %s", matchedID, bestMatch)
			result[rawName] = bestMatch
		} else {
			log.Printf("AI suggested invalid EPG ID that couldn't be auto-corrected: %s", matchedID)
		}
	}

	return result, nil
}
