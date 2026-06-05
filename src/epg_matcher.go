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
func MatchWithGemini(apiKey string, unmappedChannels []string, epgChannels map[string]string) (map[string]string, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("gemini API key is empty")
	}

	// Prepare EPG list
	var epgList []string
	for id, name := range epgChannels {
		epgList = append(epgList, fmt.Sprintf("%s (ID: %s)", name, id))
	}

	// We might hit limits if we send too many, but for now let's assume it fits in context.
	prompt := fmt.Sprintf(`You are a smart TV channel EPG assigner. 
I have a list of raw IPTV channel names and a list of available EPG channel IDs. 
Please match the IPTV channels to the closest EPG channel ID.
If you are not sure, leave the ID empty.

CRITICAL RULES:
1. You MUST select the "ID" EXACTLY as it appears in the provided list. 
2. Do not invent, guess, or alter IDs. Pay strict attention to dots, dashes, and suffixes (e.g., if the list has "TVN.7.HD.pl", do not output "TVN7.HD.pl" or "TVN7.pl").
3. If an exact matching ID is not found, return an empty string "".

Available EPG Channels:
%s

Raw IPTV Channels to match:
%s

Return ONLY a valid JSON object where the key is the exact "Raw IPTV Channel" and the value is the exact matched "ID". Do not include markdown codeblocks or any other text.`,
		strings.Join(epgList, "\n"),
		strings.Join(unmappedChannels, "\n"))

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

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-3.5-flash:generateContent?key=%s", apiKey)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini API error: %s", string(bodyBytes))
	}

	var geminiResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, err
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response from Gemini")
	}

	responseText := geminiResp.Candidates[0].Content.Parts[0].Text
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	log.Printf("Gemini AI Match Response:\n%s\n", responseText)

	var parsedResult map[string]string
	if err := json.Unmarshal([]byte(responseText), &parsedResult); err != nil {
		log.Printf("Failed to parse Gemini JSON: %s", responseText)
		return nil, fmt.Errorf("failed to parse JSON from Gemini: %v", err)
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

		// 2. Fuzzy match (auto-correct missing dots)
		corrected := ""
		cleanMatched := strings.ToLower(strings.ReplaceAll(matchedID, ".", ""))
		for validID := range epgChannels {
			cleanValid := strings.ToLower(strings.ReplaceAll(validID, ".", ""))
			if cleanMatched == cleanValid {
				corrected = validID
				break
			}
		}

		if corrected != "" {
			log.Printf("Gemini hallucinated ID '%s', auto-corrected to '%s' for channel '%s'", matchedID, corrected, rawName)
			result[rawName] = corrected
		} else {
			log.Printf("Gemini hallucinated ID '%s' for channel '%s', dropped because it does not exist.", matchedID, rawName)
		}
	}

	return result, nil
}
