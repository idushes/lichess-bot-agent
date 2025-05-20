# Lichess Bot Agent (A2A-AI-Agent)

This is a Go-based Lichess bot agent that uses an LLM (via OpenRouter.ai) to decide its chess moves.

## Features

- Connects to Lichess using the Lichess API.
- Listens for incoming game challenges.
- Accepts standard chess challenges from human players.
- Manages multiple games concurrently using goroutines.
- For each game:
    - Streams game events (moves, game state).
    - Stores the history of moves.
    - When it's the bot's turn:
        - Plays "d2d4" as the first move if playing as White.
        - For subsequent moves, queries an LLM (Language Model) via the OpenRouter.ai API to get the best move.
        - Submits the suggested move to Lichess.
        - If Lichess deems the move invalid, it re-queries the LLM with feedback about the invalid move and tries again (up to 3 attempts per turn).
- Terminates game-specific goroutines when a game ends (mate, resign, draw, etc.).
- Requires `LICHESS_TOKEN` and `OPENROUTER_API_KEY` environment variables to run.
- Listens on a configurable port (default `8080`) for HTTP health checks.

## Setup

### 1. Environment Variables

Before running the agent, you need to set the following environment variables:

- `LICHESS_TOKEN`: Your Lichess API access token. You can generate this from your Lichess account settings (Preferences -> API access tokens). Ensure the token has the following scopes:
    - Play games with the bot API (`bot:play`)
    - Read incoming challenges (`challenge:read`)
    - Create, accept, decline challenges (`challenge:write`)
    - Read your public profile (`player:read`) - used to get bot's username/ID.
- `OPENROUTER_API_KEY`: Your API key for OpenRouter.ai. You can get this from your OpenRouter account.
- `PORT` (Optional): The port on which the bot agent's HTTP server will listen. Defaults to `8080` if not set.

Example (add these to your `.bashrc`, `.zshrc`, or an `.env` file):

```bash
export LICHESS_TOKEN="your_lichess_api_token_here"
export OPENROUTER_API_KEY="your_openrouter_api_key_here"
export PORT="8080" # Optional
```

If using an `.env` file, you might run the bot with a helper like `source .env && ./your_bot_executable` or use a tool that loads `.env` files.

### 2. Go Environment

Ensure you have Go installed (version 1.18 or newer recommended).

## Building and Running

1.  **Clone the repository (if applicable) or ensure you have the Go files in a directory.**

2.  **Navigate to the project directory:**
    ```bash
    cd path/to/lichess-bot-agent
    ```

3.  **Build the executable:**
    ```bash
    go build -o lichess-bot-agent .
    ```
    This will create an executable file named `lichess-bot-agent` (or `lichess-bot-agent.exe` on Windows).

4.  **Run the bot:**
    Make sure your environment variables (`LICHESS_TOKEN`, `OPENROUTER_API_KEY`) are set in your current shell session.
    ```bash
    ./lichess-bot-agent
    ```

    The bot will start, attempt to connect to Lichess, and begin listening for game challenges.

## LLM Model

Currently, the bot is configured to use a model like `openai/gpt-3.5-turbo` via OpenRouter.ai. You can change the `openRouterModel` variable in the source code (`main.go` or `llm_client.go`) if you wish to use a different compatible model available on OpenRouter (e.g., `openai/o3-mini-high` as per original request, ensure it's a valid identifier on OpenRouter).

## How it Works

1.  **Initialization**: The agent starts, checks for required API tokens, and fetches its own Lichess account details.
2.  **Event Streaming**: It opens a persistent connection to the Lichess event stream (`/api/stream/event`).
3.  **Challenge Handling**: When a `challenge` event is received:
    - It checks if the challenge is for a standard game and from a human player.
    - If valid, it accepts the challenge using `/api/challenge/{challengeId}/accept`.
4.  **Game Start**: When a `gameStart` event is received (indicating an accepted challenge has started a game):
    - A new goroutine is spawned to handle this specific game.
    - This goroutine opens a stream for game-specific events (`/api/bot/game/stream/{gameId}`).
5.  **Game Play**: 
    - The game goroutine listens for `gameFull` (initial game state) and `gameState` (updates after each move) events.
    - Moves are recorded in memory.
    - When it's the bot's turn (determined by whose turn it is and the bot's color):
        - **First Move (White)**: If the bot is White and it's the first move, it plays `d2d4`.
        - **LLM for Moves**: Otherwise, it constructs a prompt including all previous moves in UCI notation. This prompt is sent to the configured LLM via OpenRouter.ai, asking for the best next move.
        - **Invalid Move Handling**: If the LLM suggests a move that Lichess rejects (e.g., "illegal move"), the bot informs the LLM about the invalid move and requests a new one. This process is repeated up to two more times for that turn.
        - **Submitting Move**: Valid moves from the LLM are sent to Lichess via `/api/bot/game/{gameId}/move/{move}`.
6.  **Game End**: When a game-ending event occurs (e.g., mate, resignation, draw) or the game stream closes, the game-specific goroutine cleans up and terminates.

## Health Check

The agent runs a simple HTTP server with a health check endpoint:

- `GET /health`: Returns a status message indicating if the bot is healthy and the number of active games it's currently managing.

## Project Structure

- `main.go`: Core application lifecycle, global variables, `Game` struct, main game logic (`checkAndMakeMove`), and HTTP server.
- `lichess_client.go`: Functions for all direct interactions with the Lichess API (event streaming, challenge handling, game streaming, making moves, account details).
- `llm_client.go`: Functions for interacting with the OpenRouter.ai API to get moves from the LLM.

## TODO / Potential Improvements

- More sophisticated error handling and retry logic for network operations.
- Implement claiming victory when an opponent disconnects and time runs out.
- Allow configuration of LLM parameters (e.g., temperature, specific model) via environment variables or a config file.
- More robust UCI validation for moves received from the LLM.
- Option to resign games in hopeless positions (would require game state evaluation).
- Persistent storage for game history (currently in-memory per game).
- Better structured logging. 