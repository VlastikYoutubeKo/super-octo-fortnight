package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// MatchWithOllama uses an Ollama API (or OpenAI compatible) to match unmapped channels.
// model may be a comma-separated preference list ("minimax-m3,gpt-oss:120b,..."); models
// are tried in order so a rate-limited or failing model falls through to the next one.
func MatchWithOllama(apiUrl, apiKey, model string, unmappedChannels []string, epgChannels map[string]string) (map[string]string, error) {
	if apiUrl == "" || model == "" {
		return nil, fmt.Errorf("ollama API URL or model is empty")
	}

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

	var lastErr error
	for _, m := range strings.Split(model, ",") {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		content, err := callOllamaModel(apiUrl, apiKey, m, prompt)
		if err != nil {
			lastErr = err
			log.Printf("Ollama model %s failed: %v (trying next)", m, err)
			continue
		}
		log.Printf("Ollama AI Match Response (model %s):\n%s\n", m, content)
		return parseJSONResponse(content, epgChannels, offered)
	}
	return nil, fmt.Errorf("all Ollama models failed. Last error: %v", lastErr)
}

// callOllamaModel sends one chat request and returns the raw JSON content string.
func callOllamaModel(apiUrl, apiKey, model, prompt string) (string, error) {
	data := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "You MUST output ONLY a valid JSON object. Do not include markdown formatting or explanations."},
			{"role": "user", "content": prompt},
		},
		"format": "json",
		"stream": false,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", apiUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	log.Printf("Calling Ollama API at %s with model %s...", apiUrl, model)
	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, truncateStr(string(bodyBytes), 200))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	message, ok := result["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid response format (missing message object)")
	}
	content, ok := message["content"].(string)
	if !ok {
		return "", fmt.Errorf("missing content in response")
	}

	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	if content == "" {
		return "", fmt.Errorf("empty content from model")
	}
	return content, nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
