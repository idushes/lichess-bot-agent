package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// getBotAccountDetails fetches the bot's username and ID from Lichess.
func getBotAccountDetails() (username string, userID string, err error) {
	log.Println("Fetching bot account information to get username and ID...")
	req, newReqErr := http.NewRequest("GET", "https://lichess.org/api/account", nil)
	if newReqErr != nil {
		return "", "", fmt.Errorf("error creating request for account info: %w", newReqErr)
	}
	req.Header.Set("Authorization", "Bearer "+lichessToken)

	resp, doErr := httpClient.Do(req)
	if doErr != nil {
		return "", "", fmt.Errorf("error fetching account info: %w", doErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("lichess account API returned non-OK status: %s. Body: %s", resp.Status, string(bodyBytes))
	}

	var accountInfo struct {
		Username string `json:"username"`
		ID       string `json:"id"`
	}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&accountInfo); decodeErr != nil {
		return "", "", fmt.Errorf("error decoding account info: %w", decodeErr)
	}

	if accountInfo.Username == "" || accountInfo.ID == "" {
		return "", "", fmt.Errorf("username or ID not found in account info: %+v", accountInfo)
	}
	log.Printf("Successfully fetched bot username: %s (ID: %s)", accountInfo.Username, accountInfo.ID)
	return accountInfo.Username, accountInfo.ID, nil
}

// streamLichessEvents connects to the Lichess event stream and handles incoming events.
func streamLichessEvents() {
	log.Println("Starting to stream Lichess events...")
	req, err := http.NewRequest("GET", "https://lichess.org/api/stream/event", nil)
	if err != nil {
		log.Fatalf("Error creating request for event stream: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+lichessToken)
	req.Header.Set("Accept", "application/x-ndjson")

	resp, err := streamingHttpClient.Do(req)
	if err != nil {
		// TODO: Implement robust retry logic instead of Fatalf for intermittent network issues.
		log.Printf("Error connecting to Lichess event stream: %v. Retrying in 15 seconds...", err)
		time.Sleep(15 * time.Second)
		go streamLichessEvents() // Retry
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Lichess event stream returned non-OK status: %s. Body: %s. Retrying in 15 seconds...", resp.Status, string(bodyBytes))
		time.Sleep(15 * time.Second)
		go streamLichessEvents() // Retry
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" { // Keep-alive or empty line
			continue
		}
		log.Printf("Received event: %s", line)

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			log.Printf("Error unmarshalling event: %v. Line: %s", err, line)
			continue
		}

		eventType, ok := event["type"].(string)
		if !ok {
			log.Printf("Event type not found or not a string: %+v", event)
			continue
		}

		switch eventType {
		case "challenge":
			challengeData, ok := event["challenge"].(map[string]interface{})
			if !ok {
				log.Printf("Challenge data not found or not a map: %+v", event)
				continue
			}
			challengeID, idOk := challengeData["id"].(string)
			status, statusOk := challengeData["status"].(string)
			if idOk && statusOk && status == "created" {
				log.Printf("Received challenge %s", challengeID)
				variant, variantOk := challengeData["variant"].(map[string]interface{})
				variantKey, keyOk := variant["key"].(string)
				if !variantOk || !keyOk || variantKey != "standard" {
					log.Printf("Declining challenge %s: not a standard game (variant: %v)", challengeID, variant)
					go declineChallenge(challengeID, "standardOnly")
					continue
				}

				challenger, challengerOk := challengeData["challenger"].(map[string]interface{})
				if challengerOk {
					_, isBot := challenger["bot"].(bool) // Check if 'bot' field exists (true if it does)
					challengerID, _ := challenger["id"].(string)
					if isBot || (challengerID == botLichessUserID) { // Decline if challenger is a bot or self
						log.Printf("Declining challenge %s: challenger is a bot or self.", challengeID)
						go declineChallenge(challengeID, "noBot") // or a more specific reason if self-challenge
						continue
					}
				}
				go acceptChallenge(challengeID)
			}
		case "gameStart":
			gameData, ok := event["game"].(map[string]interface{})
			if !ok {
				log.Printf("Game data not found or not a map: %+v", event)
				continue
			}
			gameID, idOk := gameData["id"].(string)
			if idOk {
				log.Printf("Game %s started. Initiating game stream.", gameID)
				go streamGameEvents(gameID)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Error reading from Lichess event stream: %v. Restarting stream...", err)
		time.Sleep(5 * time.Second) // Wait before reconnecting
		go streamLichessEvents()    // Reconnect
	} else {
		log.Println("Lichess event stream closed by Lichess. Restarting...")
		time.Sleep(5 * time.Second)
		go streamLichessEvents() // Reconnect
	}
}

// acceptChallenge accepts a challenge on Lichess.
func acceptChallenge(challengeID string) {
	log.Printf("Attempting to accept challenge %s", challengeID)
	acceptURL := fmt.Sprintf("https://lichess.org/api/challenge/%s/accept", challengeID)
	req, err := http.NewRequest("POST", acceptURL, nil)
	if err != nil {
		log.Printf("Error creating request to accept challenge %s: %v", challengeID, err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+lichessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("Error accepting challenge %s: %v", challengeID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Printf("Challenge %s accepted successfully!", challengeID)
	} else {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to accept challenge %s. Status: %s. Body: %s", challengeID, resp.Status, string(bodyBytes))
	}
}

// declineChallenge declines a challenge on Lichess.
func declineChallenge(challengeID string, reason string) {
	log.Printf("Attempting to decline challenge %s for reason: %s", challengeID, reason)
	declineURL := fmt.Sprintf("https://lichess.org/api/challenge/%s/decline", challengeID)
	payload := strings.NewReader(fmt.Sprintf("reason=%s", reason))
	req, err := http.NewRequest("POST", declineURL, payload)
	if err != nil {
		log.Printf("Error creating request to decline challenge %s: %v", challengeID, err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+lichessToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("Error declining challenge %s: %v", challengeID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Printf("Challenge %s declined successfully.", challengeID)
	} else {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to decline challenge %s. Status: %s. Body: %s", challengeID, resp.Status, string(bodyBytes))
	}
}

// streamGameEvents connects to the Lichess game event stream for a specific gameID.
func streamGameEvents(gameID string) {
	activeGamesMutex.Lock()
	if _, exists := activeGames[gameID]; exists {
		log.Printf("Game %s stream already being handled.", gameID)
		activeGamesMutex.Unlock()
		return
	}
	game := &Game{
		ID:     gameID,
		Moves:  make([]string, 0),
		doneCh: make(chan struct{}),
	}
	activeGames[gameID] = game
	activeGamesMutex.Unlock()

	log.Printf("Starting to stream events for game %s", gameID)
	defer func() {
		activeGamesMutex.Lock()
		delete(activeGames, gameID)
		log.Printf("Game %s ended. Removed from active games. Goroutine finishing.", gameID)
		activeGamesMutex.Unlock()
	}()

	gameURL := fmt.Sprintf("https://lichess.org/api/bot/game/stream/%s", gameID)
	req, err := http.NewRequest("GET", gameURL, nil)
	if err != nil {
		log.Printf("Error creating request for game stream %s: %v", gameID, err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+lichessToken)
	req.Header.Set("Accept", "application/x-ndjson")

	resp, err := streamingHttpClient.Do(req)
	if err != nil {
		log.Printf("Error connecting to Lichess game stream for %s: %v. Game goroutine will terminate.", gameID, err)
		// No automatic retry here as the game might be unrecoverable or already over.
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Lichess game stream for %s returned non-OK status: %s. Body: %s. Game goroutine will terminate.", gameID, resp.Status, string(bodyBytes))
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	for {
		select {
		case <-game.doneCh:
			log.Printf("Game %s marked as done via doneCh. Closing stream scanner.", gameID)
			return
		default:
			if !scanner.Scan() {
				if scanErr := scanner.Err(); scanErr != nil {
					log.Printf("Error reading from game stream %s: %v", gameID, scanErr)
				} else {
					log.Printf("Game stream %s closed by Lichess (EOF).", gameID)
				}
				game.mu.Lock()
				// If stream closes and game not marked done, assume opponent might be gone or game aborted by Lichess
				if !game.opponentGone && activeGames[gameID] == game { // Check if not already marked as ended
					log.Printf("Game stream for %s closed unexpectedly. Marking as opponent gone.", game.ID)
					game.opponentGone = true
				}
				game.mu.Unlock()
				close(game.doneCh) // Ensure game is marked as done to clean up resources
				return
			}

			line := scanner.Text()
			if line == "" { // Keep-alive
				continue
			}
			log.Printf("Game %s event: %s", gameID, line)

			var eventData map[string]interface{}
			if err := json.Unmarshal([]byte(line), &eventData); err != nil {
				log.Printf("Error unmarshalling game event for %s: %v. Line: %s", gameID, err, line)
				continue
			}

			eventType, ok := eventData["type"].(string)
			if !ok {
				log.Printf("Game event type not found or not a string in game %s: %+v", gameID, eventData)
				continue
			}

			game.mu.Lock() // Lock before accessing game state

			switch eventType {
			case "gameFull":
				initialState, ok := eventData["state"].(map[string]interface{})
				if !ok {
					log.Printf("Error: 'state' field missing or not a map in gameFull event for game %s: %s", gameID, line)
					game.mu.Unlock()
					close(game.doneCh)
					return
				}

				// Determine bot's color using pre-fetched botLichessUserID
				whitePlayer, wOk := eventData["white"].(map[string]interface{})
				blackPlayer, bOk := eventData["black"].(map[string]interface{})

				if wOk {
					if id, idOk := whitePlayer["id"].(string); idOk && id == botLichessUserID {
						game.Color = "white"
					}
				}
				if bOk {
					if id, idOk := blackPlayer["id"].(string); idOk && id == botLichessUserID {
						game.Color = "black"
					}
				}

				if game.Color == "" {
					if botSide, ok := eventData["botSide"].(string); ok {
						game.Color = botSide
					} else {
						log.Printf("Could not determine bot color for game %s using bot ID %s. White: %+v, Black: %+v", gameID, botLichessUserID, whitePlayer, blackPlayer)
					}
				}
				log.Printf("Game %s: Bot is playing as %s.", gameID, game.Color)

				movesStr, _ := initialState["moves"].(string)

				if movesStr != "" {
					game.Moves = strings.Fields(movesStr)
				}
				log.Printf("Game %s initial state. Moves: %d. Bot color: %s.", gameID, len(game.Moves), game.Color)

				game.mu.Unlock()
				checkAndMakeMove(game, initialState) // Call with original signature

			case "gameState":
				movesStr, _ := eventData["moves"].(string)
				status, _ := eventData["status"].(string)

				if movesStr != "" {
					game.Moves = strings.Fields(movesStr)
				}
				log.Printf("Game %s state update. Moves: %d, Status: %s.", gameID, len(game.Moves), status)

				if isGameOver(status) {
					log.Printf("Game %s is over. Status: %s", gameID, status)
					game.mu.Unlock()
					close(game.doneCh)
					return
				}
				game.mu.Unlock()
				checkAndMakeMove(game, eventData) // Call with original signature

			case "chatLine":
				log.Printf("Game %s chat: User %s says %s", gameID, eventData["username"], eventData["text"])
				game.mu.Unlock() // Unlock as chat doesn't modify critical game state for move making

			case "opponentGone":
				isGone, ok := eventData["gone"].(bool)
				claimWinInSeconds, _ := eventData["claimWinInSeconds"].(float64)

				if ok && isGone {
					log.Printf("Game %s: Opponent is gone. Can claim win in %.0f seconds.", gameID, claimWinInSeconds)
					game.opponentGone = true
					// Lichess may auto-claim, or we might need to implement a claimWin function.
					// For now, noting. If claimWinInSeconds is 0, we could potentially claim immediately.
				} else {
					log.Printf("Game %s: Opponent gone status update: gone=%v, claimIn=%.0fs", gameID, isGone, claimWinInSeconds)
				}
				game.mu.Unlock()

			default:
				log.Printf("Game %s: Unhandled event type '%s': %s", gameID, eventType, line)
				game.mu.Unlock()
			}
		}
	}
}

// makeMove sends a move to Lichess for a given game.
// move should be in UCI format (e.g., "e2e4", "e7e8q").
func makeMove(gameID string, move string) error {
	log.Printf("Attempting to make move %s in game %s", move, gameID)
	moveURL := fmt.Sprintf("https://lichess.org/api/bot/game/%s/move/%s", gameID, move)

	req, err := http.NewRequest("POST", moveURL, nil)
	if err != nil {
		return fmt.Errorf("error creating request to make move %s for game %s: %w", move, gameID, err)
	}
	req.Header.Set("Authorization", "Bearer "+lichessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making move %s for game %s: %w", move, gameID, err)
	}
	defer resp.Body.Close()

	bodyBytes, readErr := io.ReadAll(resp.Body) // Read body for all responses for better error info
	if readErr != nil {
		log.Printf("Error reading response body after move %s for game %s: %v", move, gameID, readErr)
		// Fall through to check status code, as the request might have succeeded.
	}

	if resp.StatusCode == http.StatusOK {
		var apiResponse map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &apiResponse); err != nil {
			log.Printf("Move %s in game %s successful (status OK), but error decoding JSON response: %v. Body: %s", move, gameID, err, string(bodyBytes))
			return nil // Treat as success if status is OK, Lichess is sometimes inconsistent with "ok":true
		}
		if ok, success := apiResponse["ok"].(bool); success && ok {
			log.Printf("Move %s in game %s successful and confirmed by Lichess.", move, gameID)
			return nil
		}
		if errMsg, hasError := apiResponse["error"].(string); hasError {
			log.Printf("Lichess API error for move %s in game %s (Status OK but error in body): %s", move, gameID, errMsg)
			return fmt.Errorf("lichess error on move %s: %s", move, errMsg)
		}
		log.Printf("Move %s in game %s returned OK status but API response was not 'ok:true' and no error field: %+v. Body: %s", move, gameID, apiResponse, string(bodyBytes))
		return nil // Assume OK if status 200 and no explicit error in JSON (Lichess can be weird)
	} else {
		// Try to parse error if Lichess returns structured JSON error even on non-200
		var errorResponse struct {
			Error string `json:"error"`
			// Lichess might also send: {"error":"Not your turn","status":400}
		}
		if err := json.Unmarshal(bodyBytes, &errorResponse); err == nil && errorResponse.Error != "" {
			return fmt.Errorf("failed to make move %s for game %s. Status: %s. Lichess error: %s", move, gameID, resp.Status, errorResponse.Error)
		}
		return fmt.Errorf("failed to make move %s for game %s. Status: %s. Body: %s", move, gameID, resp.Status, string(bodyBytes))
	}
}
