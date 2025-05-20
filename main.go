package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	lichessToken        string
	openRouterAPIKey    string
	port                string
	httpClient          = &http.Client{Timeout: 10 * time.Second} // For regular API calls
	streamingHttpClient = &http.Client{Timeout: 0}                // For long-lived streams, 0 means no timeout
	activeGames         = make(map[string]*Game)
	activeGamesMutex    = &sync.Mutex{}
	openRouterModel     = "openai/gpt-4o" // User request: openai/o3-mini-high. Let's use this if confirmed.
	botLichessUserID    string
	botLichessUsername  string
)

const defaultPort = "8080"

// loadEnvFromFile loads environment variables from a given file path.
// It prioritizes variables from this file if they are set.
// Lines starting with # are comments. Empty lines are skipped.
func loadEnvFromFile(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("No .env file found at %s, using system environment variables.", filePath)
			return
		}
		log.Printf("Error opening .env file %s: %v. Using system environment variables.", filePath, err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	linesRead := 0
	varsSet := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		linesRead++

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			log.Printf("Warning: Malformed line %d in %s: %s (missing '=')", linesRead, filePath, line)
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove surrounding quotes from value if present (e.g. VAR="some value" or VAR='some value')
		if len(value) > 1 {
			if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
				(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}
		}

		if key == "" {
			log.Printf("Warning: Malformed line %d in %s: %s (empty key)", linesRead, filePath, line)
			continue
		}

		err = os.Setenv(key, value)
		if err != nil {
			log.Printf("Warning: Failed to set environment variable %s from %s: %v", key, filePath, err)
		} else {
			varsSet++
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading from .env file %s: %v", filePath, err)
	}
	if varsSet > 0 {
		log.Printf("Loaded %d environment variable(s) from %s", varsSet, filePath)
	}
}

// Game struct to hold game-specific information
type Game struct {
	ID           string
	Color        string // "white" or "black"
	Moves        []string
	mu           sync.Mutex
	opponentGone bool
	doneCh       chan struct{} // Channel to signal game termination
}

func main() {
	// Attempt to load environment variables from .env file first.
	loadEnvFromFile(".env")

	lichessToken = os.Getenv("LICHESS_TOKEN")
	if lichessToken == "" {
		log.Fatal("LICHESS_TOKEN environment variable not set.")
	}

	openRouterAPIKey = os.Getenv("OPENROUTER_API_KEY")
	if openRouterAPIKey == "" {
		log.Fatal("OPENROUTER_API_KEY environment variable not set.")
	}

	port = os.Getenv("PORT")
	if port == "" {
		port = defaultPort
		log.Printf("PORT environment variable not set, using default port %s", defaultPort)
	}

	var err error
	botLichessUsername, botLichessUserID, err = getBotAccountDetails()
	if err != nil {
		log.Fatalf("Failed to fetch Lichess account details: %v", err)
	}
	log.Printf("Successfully identified bot as: %s (ID: %s)", botLichessUsername, botLichessUserID)

	go streamLichessEvents()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Welcome to the Lichess Bot Agent! Bot: %s", botLichessUsername)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		activeGamesMutex.Lock()
		numActiveGames := len(activeGames)
		activeGamesMutex.Unlock()
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Lichess Bot Agent (%s) is healthy. Active games: %d", botLichessUsername, numActiveGames)
	})

	log.Printf("Starting Lichess Bot Agent server on :%s for bot %s (ID: %s)", port, botLichessUsername, botLichessUserID)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Error starting server: %s\n", err)
	}
}

// isGameOver checks if the game status indicates the game has ended.
func isGameOver(status string) bool {
	switch status {
	case "mate", "resign", "stalemate", "timeout", "draw", "outoftime", "cheat", "aborted", "variantEnd", "noStart":
		return true
	default:
		return false
	}
}

// checkAndMakeMove processes the game state and makes a move if it's the bot's turn.
// Reverted to use move list for turn determination and LLM query.
func checkAndMakeMove(game *Game, originalGameState map[string]interface{}) {
	game.mu.Lock() // Lock for reading game.Color and game.Moves
	numMoves := len(game.Moves)
	var currentTurnColor string
	if numMoves%2 == 0 {
		currentTurnColor = "white"
	} else {
		currentTurnColor = "black"
	}

	if game.Color == "" {
		log.Printf("Game %s: Bot color not yet determined in checkAndMakeMove. Original state: %+v", game.ID, originalGameState)
		game.mu.Unlock()
		return
	}

	if game.Color != currentTurnColor {
		log.Printf("Game %s: Not our turn. It's %s's turn (based on %d moves). Bot is %s.", game.ID, currentTurnColor, numMoves, game.Color)
		game.mu.Unlock()
		return
	}
	log.Printf("Game %s: It's our turn (%s). Current moves count: %d", game.ID, game.Color, numMoves)

	currentMovesForLLM := make([]string, numMoves)
	copy(currentMovesForLLM, game.Moves)
	gameColorForLogic := game.Color
	gameIDForLogic := game.ID
	game.mu.Unlock() // Unlock before potentially long-running operation (LLM call)

	var nextMove string
	var err error

	if gameColorForLogic == "white" && numMoves == 0 {
		nextMove = "d2d4"
		log.Printf("Game %s: Playing as white, first move: %s", gameIDForLogic, nextMove)
	} else {
		log.Printf("Game %s: Querying LLM for next move. Current moves: %v", gameIDForLogic, currentMovesForLLM)
		nextMove, err = getBestMoveFromLLM(currentMovesForLLM, "")
		if err != nil {
			log.Printf("Error getting move from LLM for game %s (moves: %v): %v. Bot will not move this turn.", gameIDForLogic, currentMovesForLLM, err)
			return
		}
	}

	if nextMove == "" {
		log.Printf("Game %s: LLM did not return a move (moves: %v). Bot will not move this turn.", gameIDForLogic, currentMovesForLLM)
		return
	}
	log.Printf("Game %s: LLM suggests move: %s (for moves: %v)", gameIDForLogic, nextMove, currentMovesForLLM)

	// Retry loop for invalid moves from Lichess
	for attempt := 0; attempt < 3; attempt++ {
		err = makeMove(gameIDForLogic, nextMove)
		if err == nil {
			log.Printf("Game %s: Move %s submitted successfully to Lichess (moves were: %v).", gameIDForLogic, nextMove, currentMovesForLLM)
			return
		}

		if strings.Contains(strings.ToLower(err.Error()), "illegal move") ||
			strings.Contains(strings.ToLower(err.Error()), "invalid uci") ||
			strings.Contains(strings.ToLower(err.Error()), "member move not found") ||
			strings.Contains(strings.ToLower(err.Error()), "not possible to play") ||
			strings.Contains(strings.ToLower(err.Error()), "cannot move to") {
			log.Printf("Game %s: Move %s was invalid (Lichess error: %v) for moves %v. Asking LLM again (attempt %d/3).", gameIDForLogic, nextMove, err, currentMovesForLLM, attempt+1)

			var feedbackToLLM string
			if nextMove != "" {
				feedbackToLLM = fmt.Sprintf("Your previous suggested move %s (for move list: %v) was invalid. Please provide a different valid UCI move.", nextMove, currentMovesForLLM)
			} else {
				feedbackToLLM = fmt.Sprintf("Please provide a valid UCI move (current move list: %v).", currentMovesForLLM)
			}

			nextMove, err = getBestMoveFromLLM(currentMovesForLLM, feedbackToLLM)
			if err != nil {
				log.Printf("Error getting new move from LLM for game %s (moves: %v) after invalid attempt: %v. Bot will not move this turn.", gameIDForLogic, currentMovesForLLM, err)
				return
			}
			if nextMove == "" {
				log.Printf("Game %s: LLM did not return a new move for moves %v after invalid attempt. Bot will not move this turn.", gameIDForLogic, currentMovesForLLM)
				return
			}
			log.Printf("Game %s: LLM suggests new move: %s (for moves: %v)", gameIDForLogic, nextMove, currentMovesForLLM)
		} else {
			log.Printf("Error making move %s for game %s (moves: %v): %v. Not an invalid move error by Lichess. Will not retry with LLM.", nextMove, gameIDForLogic, currentMovesForLLM, err)
			return
		}
	}
	log.Printf("Game %s: Failed to make a valid move for moves %v after 3 attempts with LLM feedback.", gameIDForLogic, currentMovesForLLM)
}

// init function to fetch username at startup
// func init() { // Not used for now
// }
