package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	// "os" // os might not be needed here if getBestMoveFromLLM is the only user and moves
)

// getBestMoveFromLLM queries the OpenRouter API for the best move using a list of moves.
func getBestMoveFromLLM(moves []string, invalidMoveFeedback string) (string, error) {
	var promptBuilder strings.Builder
	promptBuilder.WriteString("You are a chess engine. The current game has the following moves in UCI format: ")
	if len(moves) == 0 {
		promptBuilder.WriteString("(empty, it's the first move of the game).")
	} else {
		promptBuilder.WriteString(strings.Join(moves, " ") + ".")
	}
	promptBuilder.WriteString("\nWhat is the best next move in UCI format for the current player?")

	if invalidMoveFeedback != "" {
		promptBuilder.WriteString(fmt.Sprintf("\nIMPORTANT: %s", invalidMoveFeedback))
	}
	promptBuilder.WriteString("\nOnly respond with the single best move in UCI notation (e.g., 'a1b1' or 'e7e8q' for promotion). Do not add any explanation or any other text.")

	prompt := promptBuilder.String()
	log.Printf("Querying LLM with move list. Prompt length: %d. Feedback provided: %t. Moves count: %d", len(prompt), invalidMoveFeedback != "", len(moves))
	log.Printf("LLM Request Prompt: %s", prompt)

	requestBody := map[string]interface{}{
		"model": openRouterModel,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a chess engine that provides the best move in UCI format given a list of previous moves."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.5,
		"max_tokens":  10000,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("error marshalling OpenRouter request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error creating OpenRouter request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+openRouterAPIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "lichess-bot-a2a-ai-agent")
	req.Header.Set("X-Title", "Lichess Bot A2A AI Agent")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request to OpenRouter: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", fmt.Errorf("error reading OpenRouter response body: %w", readErr)
	}
	log.Printf("LLM Raw Response Body: %s", string(bodyBytes))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenRouter API request failed with status %s: %s", resp.Status, string(bodyBytes))
	}

	var openRouterResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(bodyBytes, &openRouterResponse); err != nil {
		return "", fmt.Errorf("error unmarshalling OpenRouter response: %w. Body: %s", err, string(bodyBytes))
	}

	if openRouterResponse.Error != nil && openRouterResponse.Error.Message != "" {
		return "", fmt.Errorf("OpenRouter API error: %s (Type: %s, Code: %v)", openRouterResponse.Error.Message, openRouterResponse.Error.Type, openRouterResponse.Error.Code)
	}

	if len(openRouterResponse.Choices) > 0 && openRouterResponse.Choices[0].Message.Content != "" {
		move := strings.TrimSpace(openRouterResponse.Choices[0].Message.Content)
		if (len(move) >= 4 && len(move) <= 5) && !strings.ContainsAny(move, " \t\n\r") {
			log.Printf("LLM suggested move (raw): '%s', processed: '%s' (based on %d moves)", openRouterResponse.Choices[0].Message.Content, move, len(moves))
			return move, nil
		}
		log.Printf("LLM response '%s' (raw: '%s') doesn't look like a valid UCI move (based on %d moves).", move, openRouterResponse.Choices[0].Message.Content, len(moves))
		return "", fmt.Errorf("LLM response '%s' is not in expected UCI format after trimming. Raw response: '%s', (based on %d moves)", move, openRouterResponse.Choices[0].Message.Content, len(moves))
	}

	return "", fmt.Errorf("no move found in OpenRouter response. Full response body: %s. (based on %d moves)", string(bodyBytes), len(moves))
}
