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

// MatchWithOllama uses an Ollama API (or OpenAI compatible) to match unmapped channels
func MatchWithOllama(apiUrl, apiKey, model string, unmappedChannels []string, epgChannels map[string]string) (map[string]string, error) {
	if apiUrl == "" || model == "" {
		return nil, fmt.Errorf("ollama API URL or model is empty")
	}

	var allIDs []string
	for id := range epgChannels {
		allIDs = append(allIDs, id)
	}

	var matchTasks []string
	for _, ch := range unmappedChannels {
		topIDs := GetTopCandidates(ch, allIDs, 15, 3)
		var opts []string
		for _, id := range topIDs {
			opts = append(opts, fmt.Sprintf("%s (ID: %s)", epgChannels[id], id))
		}
		matchTasks = append(matchTasks, fmt.Sprintf("IPTV Channel: '%s'\nAvailable Options:\n%s\n", ch, strings.Join(opts, "\n")))
	}

	prompt := fmt.Sprintf(`You are an expert TV channel EPG assigner. 
For each IPTV channel, I will provide a short list of 15 candidate EPG options. 
Please select the best matching EPG ID from its options. If none match, leave it empty.

CRITICAL RULES:
1. You MUST select the "ID" EXACTLY as it appears in the options for that specific channel. 
2. Do not invent, guess, or alter IDs. 
3. Return ONLY a valid JSON object where the key is the exact "IPTV Channel" and the value is the exact matched "ID".
4. If a specific regional channel is requested (e.g., 'ITV CENTRAL WEST' or 'BBC One North West') and its exact EPG ID is not in the options, you MUST match it to its national version if available (e.g., 'ITV 1', 'BBC One', 'ITV.uk', 'ITV1.uk', etc.).

Channels to match:
%s`, strings.Join(matchTasks, "\n"))

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
		return nil, err
	}

	req, err := http.NewRequest("POST", apiUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	log.Printf("Calling Ollama API at %s with model %s...", apiUrl, model)
	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Parse Ollama response format
	message, ok := result["message"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format from Ollama (missing message object)")
	}
	content, ok := message["content"].(string)
	if !ok {
		return nil, fmt.Errorf("missing content in Ollama response")
	}

	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	log.Printf("Ollama AI Match Response:\n%s\n", content)

	return parseJSONResponse(content, epgChannels)
}
