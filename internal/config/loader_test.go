package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1mb-dev/lobster/v2/internal/domain"
)

func TestNewLoader(t *testing.T) {
	loader := NewLoader()
	if loader == nil {
		t.Fatal("Expected NewLoader() to return non-nil Loader")
	}
}

func TestLoadFromFile_Success(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.json")

	configJSON := `{
		"base_url": "http://example.com",
		"concurrency": 10,
		"duration": "5m",
		"timeout": "60s",
		"rate": 5.0,
		"user_agent": "TestAgent/1.0",
		"follow_links": false,
		"max_depth": 5,
		"output_file": "test-results.json",
		"verbose": true,
		"performance_targets": {
			"requests_per_second": 200,
			"avg_response_time_ms": 25,
			"p95_response_time_ms": 50,
			"p99_response_time_ms": 100,
			"success_rate": 99.5
		}
	}`

	err := os.WriteFile(configPath, []byte(configJSON), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	loader := NewLoader()
	config := &domain.Config{}
	err = loader.LoadFromFile(configPath, config)
	if err != nil {
		t.Fatalf("LoadFromFile() returned error: %v", err)
	}

	// Verify loaded values
	if config.BaseURL != "http://example.com" {
		t.Errorf("Expected BaseURL 'http://example.com', got '%s'", config.BaseURL)
	}
	if config.Concurrency != 10 {
		t.Errorf("Expected Concurrency 10, got %d", config.Concurrency)
	}
	if config.Duration != "5m" {
		t.Errorf("Expected Duration '5m', got '%s'", config.Duration)
	}
	if config.Timeout != "60s" {
		t.Errorf("Expected Timeout '60s', got '%s'", config.Timeout)
	}
	if config.Rate != 5.0 {
		t.Errorf("Expected Rate 5.0, got %f", config.Rate)
	}
	if config.UserAgent != "TestAgent/1.0" {
		t.Errorf("Expected UserAgent 'TestAgent/1.0', got '%s'", config.UserAgent)
	}
	if config.FollowLinks {
		t.Errorf("Expected FollowLinks false, got true")
	}
	if config.MaxDepth != 5 {
		t.Errorf("Expected MaxDepth 5, got %d", config.MaxDepth)
	}
	if config.OutputFile != "test-results.json" {
		t.Errorf("Expected OutputFile 'test-results.json', got '%s'", config.OutputFile)
	}
	if !config.Verbose {
		t.Errorf("Expected Verbose true, got false")
	}
	if config.PerformanceTargets.RequestsPerSecond != 200 {
		t.Errorf("Expected RequestsPerSecond 200, got %f", config.PerformanceTargets.RequestsPerSecond)
	}
}

func TestLoadFromFile_NonExistentFile(t *testing.T) {
	loader := NewLoader()
	err := loader.LoadFromFile("/nonexistent/path/config.json", &domain.Config{})
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}

func TestLoadFromFile_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid-config.json")

	invalidJSON := `{
		"base_url": "http://example.com",
		"concurrency": "not-a-number"
	}`

	err := os.WriteFile(configPath, []byte(invalidJSON), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	loader := NewLoader()
	err = loader.LoadFromFile(configPath, &domain.Config{})
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestSaveToFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "save-test-config.json")

	config := &domain.Config{
		BaseURL:     "http://test.com",
		Concurrency: 15,
		Duration:    "10m",
		Timeout:     "45s",
		Rate:        7.5,
		UserAgent:   "SaveTest/1.0",
		FollowLinks: true,
		MaxDepth:    4,
		OutputFile:  "output.json",
		Verbose:     false,
		PerformanceTargets: domain.PerformanceTargets{
			RequestsPerSecond: 150,
			AvgResponseTimeMs: 30,
			P95ResponseTimeMs: 75,
			P99ResponseTimeMs: 150,
			SuccessRate:       98.0,
		},
	}

	loader := NewLoader()
	err := loader.SaveToFile(config, configPath)
	if err != nil {
		t.Fatalf("SaveToFile() returned error: %v", err)
	}

	// Verify file was created
	_, statErr := os.Stat(configPath)
	if os.IsNotExist(statErr) {
		t.Error("SaveToFile() did not create file")
	}

	// Load back and verify
	loadedConfig := &domain.Config{}
	err = loader.LoadFromFile(configPath, loadedConfig)
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	if loadedConfig.BaseURL != config.BaseURL {
		t.Errorf("Saved BaseURL mismatch: expected '%s', got '%s'",
			config.BaseURL, loadedConfig.BaseURL)
	}
	if loadedConfig.Concurrency != config.Concurrency {
		t.Errorf("Saved Concurrency mismatch: expected %d, got %d",
			config.Concurrency, loadedConfig.Concurrency)
	}
}

func TestLoadFromFile_EnvVarSubstitution(t *testing.T) {
	// Set test environment variables
	t.Setenv("TEST_BASE_URL", "http://env-test.com")
	t.Setenv("TEST_PASSWORD", "secret123")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "env-config.json")

	configJSON := `{
		"base_url": "${TEST_BASE_URL}",
		"concurrency": 5,
		"duration": "1m",
		"auth": {
			"type": "basic",
			"username": "admin",
			"password": "${TEST_PASSWORD}"
		}
	}`

	err := os.WriteFile(configPath, []byte(configJSON), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	loader := NewLoader()
	config := &domain.Config{}
	err = loader.LoadFromFile(configPath, config)
	if err != nil {
		t.Fatalf("LoadFromFile() returned error: %v", err)
	}

	if config.BaseURL != "http://env-test.com" {
		t.Errorf("Expected BaseURL 'http://env-test.com', got '%s'", config.BaseURL)
	}
	if config.Auth == nil || config.Auth.Password != "secret123" {
		t.Errorf("Expected Auth.Password 'secret123', got '%v'", config.Auth)
	}
}

func TestLoadFromFile_EnvVarWithDefault(t *testing.T) {
	// Don't set MISSING_VAR - it should use the default
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "default-config.json")

	configJSON := `{
		"base_url": "${MISSING_VAR:-http://default.com}",
		"concurrency": 5,
		"duration": "1m"
	}`

	err := os.WriteFile(configPath, []byte(configJSON), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	loader := NewLoader()
	config := &domain.Config{}
	err = loader.LoadFromFile(configPath, config)
	if err != nil {
		t.Fatalf("LoadFromFile() returned error: %v", err)
	}

	if config.BaseURL != "http://default.com" {
		t.Errorf("Expected BaseURL 'http://default.com' (default), got '%s'", config.BaseURL)
	}
}

func TestLoadFromFile_MissingRequiredEnvVar(t *testing.T) {
	// Don't set REQUIRED_VAR - should cause an error
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "missing-config.json")

	configJSON := `{
		"base_url": "${REQUIRED_VAR_THAT_DOES_NOT_EXIST}",
		"concurrency": 5,
		"duration": "1m"
	}`

	err := os.WriteFile(configPath, []byte(configJSON), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	loader := NewLoader()
	err = loader.LoadFromFile(configPath, &domain.Config{})
	if err == nil {
		t.Error("Expected error for missing required env var, got nil")
	}
}

func TestSubstituteEnvVars_MultipleVars(t *testing.T) {
	t.Setenv("VAR_A", "valueA")
	t.Setenv("VAR_B", "valueB")

	input := `{"a": "${VAR_A}", "b": "${VAR_B}"}`
	result, err := substituteEnvVars(input)
	if err != nil {
		t.Fatalf("substituteEnvVars() returned error: %v", err)
	}

	expected := `{"a": "valueA", "b": "valueB"}`
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestSubstituteEnvVars_NoVars(t *testing.T) {
	input := `{"base_url": "http://example.com"}`
	result, err := substituteEnvVars(input)
	if err != nil {
		t.Fatalf("substituteEnvVars() returned error: %v", err)
	}

	if result != input {
		t.Errorf("Expected unchanged input, got '%s'", result)
	}
}

func TestLoadFromFile_OverDefaults(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"max_depth": 0, "performance_targets": {"p95_response_time_ms": 0}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := domain.DefaultConfig()
	cfg.PerformanceTargets.P95ResponseTimeMs = 100
	if err := NewLoader().LoadFromFile(configPath, &cfg); err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	if cfg.MaxDepth != 0 {
		t.Errorf("max_depth 0 in the file: got %d, want 0", cfg.MaxDepth)
	}
	if cfg.PerformanceTargets.P95ResponseTimeMs != 0 {
		t.Errorf("p95 target 0 in the file: got %g, want 0", cfg.PerformanceTargets.P95ResponseTimeMs)
	}
	if want := domain.DefaultConfig().Concurrency; cfg.Concurrency != want {
		t.Errorf("concurrency absent from the file: got %d, want default %d", cfg.Concurrency, want)
	}
	if cfg.PerformanceTargets.SuccessRate != domain.DefaultPerformanceTargets().SuccessRate {
		t.Errorf("success_rate absent from the file: got %g, want default", cfg.PerformanceTargets.SuccessRate)
	}
}

func TestLoadFromFile_UnknownKey(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"concurency": 5}`), 0o600); err != nil {
		t.Fatal(err)
	}

	err := NewLoader().LoadFromFile(configPath, &domain.Config{})
	if err == nil || !strings.Contains(err.Error(), "concurency") {
		t.Fatalf("LoadFromFile() error = %v, want an unknown-field error naming the key", err)
	}
}
