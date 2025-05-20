package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Helper function to create a temporary .env file
func createTempEnvFile(t *testing.T, content string) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	envFilePath := filepath.Join(tmpDir, ".env")
	err := os.WriteFile(envFilePath, []byte(content), 0600)
	if err != nil {
		t.Fatalf("Failed to create temp .env file: %v", err)
	}

	// Change current working directory to the temp directory
	// so that .env is found by loadEnvFromFileToSystem
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	return envFilePath, func() {
		if err := os.Chdir(originalWD); err != nil {
			t.Logf("Warning: failed to change back to original working directory: %v", err)
		}
		// os.Remove(envFilePath) // TempDir will clean this up
	}
}

// Helper function to set environment variables for a test
func setEnvVars(t *testing.T, vars map[string]string) func() {
	t.Helper()
	originalVars := make(map[string]string)
	for k, v := range vars {
		originalVal, wasSet := os.LookupEnv(k)
		if wasSet {
			originalVars[k] = originalVal
		} else {
			originalVars[k] = "__NOT_SET__" // Special marker for unset
		}
		os.Setenv(k, v)
	}

	return func() {
		for k, v := range originalVars {
			if v == "__NOT_SET__" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}
}

func TestLoadConfig_FromEnv(t *testing.T) {
	cleanup := setEnvVars(t, map[string]string{
		"LICHESS_TOKEN":      "test_lichess_token_env",
		"OPENROUTER_API_KEY": "test_openrouter_key_env",
		"PORT":               "9090",
	})
	defer cleanup()

	// Ensure no .env file is interfering
	_, cleanupWD := createTempEnvFile(t, "") // Create an empty .env to ensure cwd is set
	defer cleanupWD()                        // Change back to original WD

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	if cfg.LichessToken != "test_lichess_token_env" {
		t.Errorf("Expected LichessToken 'test_lichess_token_env', got '%s'", cfg.LichessToken)
	}
	if cfg.OpenRouterAPIKey != "test_openrouter_key_env" {
		t.Errorf("Expected OpenRouterAPIKey 'test_openrouter_key_env', got '%s'", cfg.OpenRouterAPIKey)
	}
	if cfg.Port != "9090" {
		t.Errorf("Expected Port '9090', got '%s'", cfg.Port)
	}
}

func TestLoadConfig_FromEnv_DefaultPort(t *testing.T) {
	cleanup := setEnvVars(t, map[string]string{
		"LICHESS_TOKEN":      "test_lichess_token_env_default",
		"OPENROUTER_API_KEY": "test_openrouter_key_env_default",
	})
	defer cleanup()
	// Unset PORT to test default
	originalPort, portSet := os.LookupEnv("PORT")
	os.Unsetenv("PORT")
	defer func() {
		if portSet {
			os.Setenv("PORT", originalPort)
		} else {
			os.Unsetenv("PORT")
		}
	}()

	// Ensure no .env file is interfering
	_, cleanupWD := createTempEnvFile(t, "") // Create an empty .env to ensure cwd is set
	defer cleanupWD()                        // Change back to original WD

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	if cfg.LichessToken != "test_lichess_token_env_default" {
		t.Errorf("Expected LichessToken 'test_lichess_token_env_default', got '%s'", cfg.LichessToken)
	}
	if cfg.OpenRouterAPIKey != "test_openrouter_key_env_default" {
		t.Errorf("Expected OpenRouterAPIKey 'test_openrouter_key_env_default', got '%s'", cfg.OpenRouterAPIKey)
	}
	if cfg.Port != defaultPortCfg { // defaultPortCfg is defined in config.go
		t.Errorf("Expected default Port '%s', got '%s'", defaultPortCfg, cfg.Port)
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	envContent := `
LICHESS_TOKEN=test_lichess_token_file
OPENROUTER_API_KEY=test_openrouter_key_file
PORT=7070
`
	_, cleanupWD := createTempEnvFile(t, envContent)
	defer cleanupWD()

	// Clear relevant env vars to ensure they are loaded from file
	cleanupEnv := setEnvVars(t, map[string]string{
		"LICHESS_TOKEN":      "",
		"OPENROUTER_API_KEY": "",
		"PORT":               "",
	})
	defer cleanupEnv()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	if cfg.LichessToken != "test_lichess_token_file" {
		t.Errorf("Expected LichessToken 'test_lichess_token_file', got '%s'", cfg.LichessToken)
	}
	if cfg.OpenRouterAPIKey != "test_openrouter_key_file" {
		t.Errorf("Expected OpenRouterAPIKey 'test_openrouter_key_file', got '%s'", cfg.OpenRouterAPIKey)
	}
	if cfg.Port != "7070" {
		t.Errorf("Expected Port '7070', got '%s'", cfg.Port)
	}
}

func TestLoadConfig_EnvOverridesFile(t *testing.T) {
	envContent := `
LICHESS_TOKEN=file_token
OPENROUTER_API_KEY=file_key
PORT=1111
`
	_, cleanupWD := createTempEnvFile(t, envContent)
	defer cleanupWD()

	cleanupEnv := setEnvVars(t, map[string]string{
		"LICHESS_TOKEN":      "env_token_override",
		"OPENROUTER_API_KEY": "env_key_override",
		"PORT":               "2222",
	})
	defer cleanupEnv()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	if cfg.LichessToken != "env_token_override" {
		t.Errorf("Expected LichessToken 'env_token_override', got '%s'", cfg.LichessToken)
	}
	if cfg.OpenRouterAPIKey != "env_key_override" {
		t.Errorf("Expected OpenRouterAPIKey 'env_key_override', got '%s'", cfg.OpenRouterAPIKey)
	}
	if cfg.Port != "2222" {
		t.Errorf("Expected Port '2222', got '%s'", cfg.Port)
	}
}

func TestLoadConfig_MissingRequired_LichessToken(t *testing.T) {
	cleanup := setEnvVars(t, map[string]string{
		"OPENROUTER_API_KEY": "some_key",
	})
	defer cleanup()
	// Ensure LICHESS_TOKEN is unset
	originalToken, tokenSet := os.LookupEnv("LICHESS_TOKEN")
	os.Unsetenv("LICHESS_TOKEN")
	defer func() {
		if tokenSet {
			os.Setenv("LICHESS_TOKEN", originalToken)
		} else {
			os.Unsetenv("LICHESS_TOKEN")
		}
	}()

	// Ensure no .env file is interfering
	_, cleanupWD := createTempEnvFile(t, "")
	defer cleanupWD()

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("Expected error when LICHESS_TOKEN is missing, but got nil")
	}
	if !strings.Contains(err.Error(), "LICHESS_TOKEN environment variable not set") {
		t.Errorf("Expected error message to contain 'LICHESS_TOKEN environment variable not set', got '%s'", err.Error())
	}
}

func TestLoadConfig_MissingRequired_OpenRouterAPIKey(t *testing.T) {
	cleanup := setEnvVars(t, map[string]string{
		"LICHESS_TOKEN": "some_token",
	})
	defer cleanup()
	// Ensure OPENROUTER_API_KEY is unset
	originalKey, keySet := os.LookupEnv("OPENROUTER_API_KEY")
	os.Unsetenv("OPENROUTER_API_KEY")
	defer func() {
		if keySet {
			os.Setenv("OPENROUTER_API_KEY", originalKey)
		} else {
			os.Unsetenv("OPENROUTER_API_KEY")
		}
	}()

	// Ensure no .env file is interfering
	_, cleanupWD := createTempEnvFile(t, "")
	defer cleanupWD()

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("Expected error when OPENROUTER_API_KEY is missing, but got nil")
	}
	if !strings.Contains(err.Error(), "OPENROUTER_API_KEY environment variable not set") {
		t.Errorf("Expected error message to contain 'OPENROUTER_API_KEY environment variable not set', got '%s'", err.Error())
	}
}

func TestLoadEnvFromFileToSystem_MalformedLines(t *testing.T) {
	envContent := `
MALFORMED_NO_EQUALS
GOOD_ONE=great_value
# This is a comment
EMPTY_KEY=
=EMPTY_VALUE
WITH_QUOTES_SINGLE='single_quoted'
WITH_QUOTES_DOUBLE="double_quoted"
MALFORMED_AGAIN
	 SPACED_KEY =  spaced_value
VALID_AFTER_MALFORMED=yes
`
	_, cleanupWD := createTempEnvFile(t, envContent)
	defer cleanupWD()

	// Clear potentially conflicting env vars
	varsToClear := []string{
		"MALFORMED_NO_EQUALS", "GOOD_ONE", "EMPTY_KEY", "",
		"WITH_QUOTES_SINGLE", "WITH_QUOTES_DOUBLE", "SPACED_KEY", "VALID_AFTER_MALFORMED",
	}
	originalValues := make(map[string]string)
	for _, k := range varsToClear {
		val, isSet := os.LookupEnv(k)
		if isSet {
			originalValues[k] = val
		}
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range originalValues {
			os.Setenv(k, v)
		}
		// Ensure specific test vars are unset if they weren't set before
		for _, k := range varsToClear {
			if _, exists := originalValues[k]; !exists {
				os.Unsetenv(k)
			}
		}
	}()

	// Call the function that loads .env, which is indirectly tested by LoadConfig
	// We can also call it directly to check os.Getenv values
	loadEnvFromFileToSystem(".env") // This will print warnings, which is fine for this test

	if val := os.Getenv("GOOD_ONE"); val != "great_value" {
		t.Errorf("Expected GOOD_ONE to be 'great_value', got '%s'", val)
	}
	if val := os.Getenv("WITH_QUOTES_SINGLE"); val != "single_quoted" {
		t.Errorf("Expected WITH_QUOTES_SINGLE to be 'single_quoted', got '%s'", val)
	}
	if val := os.Getenv("WITH_QUOTES_DOUBLE"); val != "double_quoted" {
		t.Errorf("Expected WITH_QUOTES_DOUBLE to be 'double_quoted', got '%s'", val)
	}
	if val := os.Getenv("SPACED_KEY"); val != "spaced_value" {
		t.Errorf("Expected SPACED_KEY to be 'spaced_value', got '%s'", val)
	}
	if val := os.Getenv("VALID_AFTER_MALFORMED"); val != "yes" {
		t.Errorf("Expected VALID_AFTER_MALFORMED to be 'yes', got '%s'", val)
	}

	// Check that malformed keys are not set
	if val := os.Getenv("MALFORMED_NO_EQUALS"); val != "" {
		t.Errorf("Expected MALFORMED_NO_EQUALS to be empty, got '%s'", val)
	}
	if val := os.Getenv("EMPTY_KEY"); val != "" { // The key is "EMPTY_KEY", value is empty string
		t.Errorf("Expected EMPTY_KEY to be an empty string, got '%s'", val)
	}
	// A key cannot be empty, so it shouldn't be set.
	// The current implementation of loadEnvFromFileToSystem logs a warning for empty key.
	// It does not try to set env var with empty key.

}

func TestLoadConfig_EmptyEnvFile(t *testing.T) {
	_, cleanupWD := createTempEnvFile(t, "") // Empty .env file
	defer cleanupWD()

	// Set required vars in environment
	cleanupEnv := setEnvVars(t, map[string]string{
		"LICHESS_TOKEN":      "token_for_empty_env_test",
		"OPENROUTER_API_KEY": "key_for_empty_env_test",
	})
	defer cleanupEnv()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() with empty .env file failed: %v", err)
	}

	if cfg.LichessToken != "token_for_empty_env_test" {
		t.Errorf("Expected LichessToken 'token_for_empty_env_test', got '%s'", cfg.LichessToken)
	}
	if cfg.Port != defaultPortCfg {
		t.Errorf("Expected default Port '%s', got '%s'", defaultPortCfg, cfg.Port)
	}
}

func TestLoadConfig_NoEnvFile(t *testing.T) {
	// Simulate no .env file by changing to a directory where it won't exist
	// (or ensure it's not in the test execution path)
	tmpDir := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWD); err != nil {
			t.Logf("Warning: failed to change back to original working directory: %v", err)
		}
	}()

	cleanupEnv := setEnvVars(t, map[string]string{
		"LICHESS_TOKEN":      "token_no_env_file",
		"OPENROUTER_API_KEY": "key_no_env_file",
		"PORT":               "3333",
	})
	defer cleanupEnv()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() with no .env file failed: %v", err)
	}

	if cfg.LichessToken != "token_no_env_file" {
		t.Errorf("Expected LichessToken 'token_no_env_file', got '%s'", cfg.LichessToken)
	}
	if cfg.OpenRouterAPIKey != "key_no_env_file" {
		t.Errorf("Expected OpenRouterAPIKey 'key_no_env_file', got '%s'", cfg.OpenRouterAPIKey)
	}
	if cfg.Port != "3333" {
		t.Errorf("Expected Port '3333', got '%s'", cfg.Port)
	}
}

// Test specifically the quote removal in loadEnvFromFileToSystem
func TestLoadEnvFromFileToSystem_QuoteRemoval(t *testing.T) {
	envContent := `
KEY_NO_QUOTES=value
KEY_SINGLE_QUOTES='quoted_value'
KEY_DOUBLE_QUOTES="double_quoted_value"
KEY_MIXED_QUOTES_INVALID_START='"unbalanced_single_end' 
KEY_MIXED_QUOTES_INVALID_END='unbalanced_double_end"
KEY_EMPTY_QUOTES_S=''
KEY_EMPTY_QUOTES_D=""
KEY_QUOTED_SPACE_S=' value with spaces '
KEY_QUOTED_SPACE_D=" value with spaces "
KEY_INTERNAL_QUOTE_S="va'lue"
KEY_INTERNAL_QUOTE_D='va"lue'
`
	_, cleanupWD := createTempEnvFile(t, envContent)
	defer cleanupWD()

	varsToTest := []string{
		"KEY_NO_QUOTES", "KEY_SINGLE_QUOTES", "KEY_DOUBLE_QUOTES",
		"KEY_MIXED_QUOTES_INVALID_START", "KEY_MIXED_QUOTES_INVALID_END",
		"KEY_EMPTY_QUOTES_S", "KEY_EMPTY_QUOTES_D",
		"KEY_QUOTED_SPACE_S", "KEY_QUOTED_SPACE_D",
		"KEY_INTERNAL_QUOTE_S", "KEY_INTERNAL_QUOTE_D",
	}
	cleanupEnv := setEnvVars(t, map[string]string{}) // Ensure a clean slate
	for _, k := range varsToTest {
		os.Unsetenv(k) // Make sure they are not set from environment
	}
	defer cleanupEnv()

	loadEnvFromFileToSystem(".env") // This loads the variables

	expectedValues := map[string]string{
		"KEY_NO_QUOTES":                  "value",
		"KEY_SINGLE_QUOTES":              "quoted_value",
		"KEY_DOUBLE_QUOTES":              "double_quoted_value",
		"KEY_MIXED_QUOTES_INVALID_START": `"unbalanced_single_end`,  // Quotes are both removed
		"KEY_MIXED_QUOTES_INVALID_END":   `'unbalanced_double_end"`, // Quotes not removed
		"KEY_EMPTY_QUOTES_S":             "",
		"KEY_EMPTY_QUOTES_D":             "",
		"KEY_QUOTED_SPACE_S":             " value with spaces ",
		"KEY_QUOTED_SPACE_D":             " value with spaces ",
		"KEY_INTERNAL_QUOTE_S":           "va'lue",
		"KEY_INTERNAL_QUOTE_D":           `va"lue`,
	}

	for key, expected := range expectedValues {
		actual := os.Getenv(key)
		if actual != expected {
			t.Errorf("For key '%s': expected '%s', got '%s'", key, expected, actual)
		}
	}
}

func TestLoadEnvFromFileToSystem_EmptyKeyOrValue(t *testing.T) {
	envContent := `
EMPTY_VALUE_KEY=
=EMPTY_KEY_VALUE
BOTH_EMPTY=
VALID_KEY=valid_value
`
	envFilePath, cleanupWD := createTempEnvFile(t, envContent)
	defer cleanupWD()

	varsToClearAndTest := []string{"EMPTY_VALUE_KEY", "VALID_KEY", ""}
	// "" is for the case of =EMPTY_KEY_VALUE, where the key itself is empty.
	// We want to ensure it doesn't pollute os.Getenv("") somehow or cause a crash.
	// The current implementation logs a warning for empty key and doesn't set it.

	originalValues := make(map[string]string)
	for _, k := range varsToClearAndTest {
		if k == "" { // os.LookupEnv cannot take empty string
			continue
		}
		val, isSet := os.LookupEnv(k)
		if isSet {
			originalValues[k] = val
		}
		os.Unsetenv(k)
	}
	// Ensure a specific one that could be an empty key doesn't exist
	// (though os.Setenv with empty key errors out)
	os.Unsetenv("")

	defer func() {
		for k, v := range originalValues {
			os.Setenv(k, v)
		}
		for _, k := range varsToClearAndTest {
			if k == "" {
				continue
			}
			if _, exists := originalValues[k]; !exists {
				os.Unsetenv(k)
			}
		}
		os.Unsetenv("")
	}()

	loadEnvFromFileToSystem(filepath.Base(envFilePath)) // Pass just ".env"

	if val := os.Getenv("EMPTY_VALUE_KEY"); val != "" {
		t.Errorf("Expected EMPTY_VALUE_KEY to be '', got '%s'", val)
	}
	if val := os.Getenv("VALID_KEY"); val != "valid_value" {
		t.Errorf("Expected VALID_KEY to be 'valid_value', got '%s'", val)
	}
	// Test that an empty key was not set.
	// There's no direct way to check if `os.Setenv("", ...)` was called and failed,
	// but we ensure no unexpected environment variable with an empty name was created.
	// `os.Getenv("")` behaviour is not standard for checking this.
	// The `loadEnvFromFileToSystem` function logs a warning for empty keys.
}
