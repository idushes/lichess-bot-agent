package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

const defaultPortCfg = "8080"

// BotConfig holds all configuration for the bot
type BotConfig struct {
	LichessToken     string
	OpenRouterAPIKey string
	Port             string
}

// loadEnvFromFileToSystem loads environment variables from a given file path into the system environment.
// It's a helper for LoadConfig.
func loadEnvFromFileToSystem(filePath string) {
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

		// Remove surrounding quotes from value if present
		if len(value) >= 2 {
			// Only remove quotes if they match at start and end
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		if key == "" {
			log.Printf("Warning: Malformed line %d in %s: %s (empty key)", linesRead, filePath, line)
			continue
		}

		err = os.Setenv(key, value) // This sets it globally for the process
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
		log.Printf("Loaded %d environment variable(s) from %s into system environment.", varsSet, filePath)
	}
}

// LoadConfig loads the bot configuration from environment variables,
// trying to load from a .env file first if environment variables are not already set.
func LoadConfig() (*BotConfig, error) {
	// Create a Config with values from system environment first
	cfg := &BotConfig{}

	// Check if required variables exist in system environment
	cfg.LichessToken = os.Getenv("LICHESS_TOKEN")
	cfg.OpenRouterAPIKey = os.Getenv("OPENROUTER_API_KEY")
	cfg.Port = os.Getenv("PORT")

	// Only load from .env if any required variables are missing
	if cfg.LichessToken == "" || cfg.OpenRouterAPIKey == "" || cfg.Port == "" {
		loadEnvFromFileToSystem(".env")

		// Update config with any values from .env that weren't already set
		if cfg.LichessToken == "" {
			cfg.LichessToken = os.Getenv("LICHESS_TOKEN")
		}
		if cfg.OpenRouterAPIKey == "" {
			cfg.OpenRouterAPIKey = os.Getenv("OPENROUTER_API_KEY")
		}
		if cfg.Port == "" {
			cfg.Port = os.Getenv("PORT")
		}
	}

	// Final validation
	if cfg.LichessToken == "" {
		return nil, fmt.Errorf("LICHESS_TOKEN environment variable not set")
	}

	if cfg.OpenRouterAPIKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	if cfg.Port == "" {
		cfg.Port = defaultPortCfg
		log.Printf("PORT environment variable not set, using default port %s", defaultPortCfg)
	}

	return cfg, nil
}
