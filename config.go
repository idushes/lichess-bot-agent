package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

const defaultPortCfg = "8080"

// BotConfig holds all configuration for the bot
type BotConfig struct {
	LichessToken     string
	OpenRouterAPIKey string
	Port             string
}

// LoadConfig loads the bot configuration from environment variables,
// trying to load from a .env file only if needed environment variables are not already set.
func LoadConfig() (*BotConfig, error) {
	// Create a Config with values from system environment first
	cfg := &BotConfig{}

	// Check if required variables exist in system environment
	cfg.LichessToken = os.Getenv("LICHESS_TOKEN")
	cfg.OpenRouterAPIKey = os.Getenv("OPENROUTER_API_KEY")
	cfg.Port = os.Getenv("PORT")

	// Only load from .env if any required variables are missing
	if cfg.LichessToken == "" || cfg.OpenRouterAPIKey == "" || cfg.Port == "" {
		// Load environment variables from .env file
		err := loadDotEnv()
		if err != nil {
			log.Printf("Warning: Failed to load .env file: %v. Using system environment variables.", err)
		}

		// Only update missing values from environment after loading .env
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

	// Check required fields and provide defaults
	if cfg.LichessToken == "" {
		return nil, fmt.Errorf("LICHESS_TOKEN environment variable not set")
	}

	if cfg.OpenRouterAPIKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY environment variable not set")
	}

	// Set default port if not provided
	if cfg.Port == "" {
		cfg.Port = defaultPortCfg
		log.Printf("PORT environment variable not set, using default port %s", defaultPortCfg)
	}

	return cfg, nil
}

// loadDotEnv loads variables from a .env file into the environment
func loadDotEnv() error {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("No .env file found, using system environment variables")
		} else {
			log.Printf("Error loading .env file: %v", err)
		}
		return err
	}

	// Проверяем, сколько переменных было загружено
	envVars, readErr := godotenv.Read()
	if readErr != nil {
		log.Printf("Error reading .env file content: %v", readErr)
		return nil // Игнорируем ошибку чтения, т.к. Load() уже отработал успешно
	}

	if len(envVars) == 0 {
		log.Printf("No variables loaded from .env file")
	} else {
		log.Printf("Successfully loaded %d environment variables from .env file", len(envVars))
	}
	return nil
}
