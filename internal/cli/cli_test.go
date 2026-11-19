package cli

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1mb-dev/lobster/v2/internal/domain"
)

// load parses args on a fresh FlagSet and builds the config the way main does.
func load(t *testing.T, configPath string, args ...string) (*domain.Config, error) {
	t.Helper()
	fs := flag.NewFlagSet("lobster", flag.ContinueOnError)
	opts := RegisterConfigFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse(%v): %v", args, err)
	}
	return LoadConfiguration(fs, configPath, opts)
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfiguration_Defaults(t *testing.T) {
	cfg, err := load(t, "")
	if err != nil {
		t.Fatalf("LoadConfiguration() error = %v", err)
	}
	if want := domain.DefaultConfig(); cfg.BaseURL != want.BaseURL || cfg.Concurrency != want.Concurrency ||
		cfg.Duration != want.Duration || cfg.Rate != want.Rate || cfg.MaxDepth != want.MaxDepth {
		t.Errorf("defaults not applied: %+v", cfg)
	}
}

func TestLoadConfiguration_Flags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
		check   func(*domain.Config) bool
	}{
		{name: "rate 0 refused", args: []string{"-rate", "0"}, wantErr: "MinRate"},
		{name: "rate below MinRate refused", args: []string{"-rate", "0.05"}, wantErr: "MinRate"},
		{name: "rate at MinRate accepted", args: []string{"-rate", "0.1"}, check: func(c *domain.Config) bool { return c.Rate == domain.MinRate }},
		{name: "concurrency 0 refused", args: []string{"-concurrency", "0"}, wantErr: "concurrency"},
		{name: "duration 0 refused", args: []string{"-duration", "0s"}, wantErr: "duration"},
		{name: "negative duration refused", args: []string{"-duration", "-5s"}, wantErr: "duration"},
		{name: "load 0 refused", args: []string{"-load", "0s"}, wantErr: "load must be > 0"},
		{name: "rate above the load ceiling refused", args: []string{"-load", "10s", "-rate", "1001"}, wantErr: "1000"},
		{name: "rate above 1000 without -load accepted", args: []string{"-rate", "1001"}, check: func(c *domain.Config) bool { return c.Rate == 1001 }},
		{name: "max-depth 0 kept", args: []string{"-max-depth", "0"}, check: func(c *domain.Config) bool { return c.MaxDepth == 0 }},
		{name: "flags override defaults", args: []string{"-url", "http://custom.example.com", "-concurrency", "25", "-duration", "10m", "-rate", "15"},
			check: func(c *domain.Config) bool {
				return c.BaseURL == "http://custom.example.com" && c.Concurrency == 25 && c.Duration == "10m" && c.Rate == 15
			}},
		{name: "bool flag set", args: []string{"-ignore-robots"}, check: func(c *domain.Config) bool { return c.IgnoreRobots }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := load(t, "", tt.args...)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if !tt.check(cfg) {
				t.Errorf("unexpected config: %+v", cfg)
			}
		})
	}
}

func TestLoadConfiguration_FromFile(t *testing.T) {
	path := writeConfig(t, `{
		"base_url": "http://file.example.com",
		"concurrency": 50,
		"ignore_robots": true,
		"performance_targets": {"p95_response_time_ms": 0}
	}`)

	cfg, err := load(t, path)
	if err != nil {
		t.Fatalf("LoadConfiguration() error = %v", err)
	}
	if cfg.BaseURL != "http://file.example.com" || cfg.Concurrency != 50 {
		t.Errorf("file values not applied: %+v", cfg)
	}
	if !cfg.IgnoreRobots {
		t.Error("ignore_robots true in the file was overwritten by the flag default")
	}
	if cfg.Duration != domain.DefaultConfig().Duration {
		t.Errorf("duration absent from the file: got %q, want the default", cfg.Duration)
	}
	if cfg.PerformanceTargets.P95ResponseTimeMs != 0 {
		t.Errorf("p95 target 0 in the file: got %g, want 0 (disabled)", cfg.PerformanceTargets.P95ResponseTimeMs)
	}
}

func TestLoadConfiguration_FileErrors(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "missing file", path: "/nonexistent/path/config.json", wantErr: "cannot read"},
		{name: "unknown key", path: writeConfig(t, `{"concurency": 5}`), wantErr: "concurency"},
		{name: "rate 0 in file", path: writeConfig(t, `{"rate": 0}`), wantErr: "MinRate"},
		{name: "negative target", path: writeConfig(t, `{"performance_targets": {"success_rate": -1}}`), wantErr: "success_rate"},
		{name: "removed error_rate target", path: writeConfig(t, `{"performance_targets": {"error_rate": 1}}`), wantErr: "error_rate"},
		{name: "removed follow_links key", path: writeConfig(t, `{"follow_links": true}`), wantErr: "follow_links"},
		{name: "removed respect_429 key", path: writeConfig(t, `{"respect_429": true}`), wantErr: "respect_429"},
		{name: "removed dry_run key", path: writeConfig(t, `{"dry_run": false}`), wantErr: "dry_run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(t, tt.path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadConfiguration_FlagsOverrideFile(t *testing.T) {
	path := writeConfig(t, `{"base_url": "http://file.example.com", "concurrency": 10, "duration": "5m"}`)

	cfg, err := load(t, path, "-concurrency", "25")
	if err != nil {
		t.Fatalf("LoadConfiguration() error = %v", err)
	}
	if cfg.BaseURL != "http://file.example.com" || cfg.Duration != "5m" {
		t.Errorf("file values lost: %+v", cfg)
	}
	if cfg.Concurrency != 25 {
		t.Errorf("Concurrency = %d, want 25 from the flag", cfg.Concurrency)
	}
}

func TestBuildAuthConfig_NoAuth(t *testing.T) {
	opts := &ConfigOptions{}
	cfg, err := BuildAuthConfig(opts)
	if err != nil {
		t.Fatalf("BuildAuthConfig() error = %v", err)
	}
	if cfg != nil {
		t.Error("Expected nil config when no auth is provided")
	}
}

func TestBuildAuthConfig_Basic(t *testing.T) {
	t.Setenv("LOBSTER_AUTH_PASSWORD", "testpass")

	opts := &ConfigOptions{
		AuthType:     "basic",
		AuthUsername: "testuser",
	}
	cfg, err := BuildAuthConfig(opts)
	if err != nil {
		t.Fatalf("BuildAuthConfig() error = %v", err)
	}

	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}
	if cfg.Type != "basic" {
		t.Errorf("Expected Type 'basic', got '%s'", cfg.Type)
	}
	if cfg.Username != "testuser" {
		t.Errorf("Expected Username 'testuser', got '%s'", cfg.Username)
	}
	if cfg.Password != "testpass" {
		t.Errorf("Expected Password 'testpass', got '%s'", cfg.Password)
	}
}

func TestBuildAuthConfig_MutuallyExclusiveStdin(t *testing.T) {
	opts := &ConfigOptions{
		AuthPasswordStdin: true,
		AuthTokenStdin:    true,
	}
	_, err := BuildAuthConfig(opts)
	if err == nil {
		t.Error("Expected error for mutually exclusive stdin flags")
	}
}

func TestBuildAuthConfig_Header(t *testing.T) {
	t.Setenv("LOBSTER_AUTH_HEADER", "secret123")

	cfg, err := BuildAuthConfig(&ConfigOptions{AuthType: "header", AuthHeader: "X-API-Key"})
	if err != nil {
		t.Fatalf("BuildAuthConfig() error = %v", err)
	}
	if cfg == nil || cfg.Headers["X-API-Key"] != "secret123" {
		t.Fatalf("header not built from LOBSTER_AUTH_HEADER: %+v", cfg)
	}
}

func TestBuildAuthConfig_HeaderErrors(t *testing.T) {
	const secret = "s3cr3t-value"
	tests := []struct {
		name    string
		env     string
		opts    ConfigOptions
		wantErr string
	}{
		{name: "Name:Value refused", env: "x", opts: ConfigOptions{AuthHeader: "X-API-Key:" + secret}, wantErr: "header name only"},
		{name: "name without a value", opts: ConfigOptions{AuthHeader: "X-API-Key"}, wantErr: "needs a value"},
		{name: "secret passed as the name", opts: ConfigOptions{AuthHeader: secret}, wantErr: "needs a value"},
		{name: "stdin without a name", opts: ConfigOptions{AuthHeaderStdin: true}, wantErr: "needs -auth-header"},
		{name: "two stdin flags", opts: ConfigOptions{AuthHeader: "X-API-Key", AuthHeaderStdin: true, AuthTokenStdin: true}, wantErr: "mutually exclusive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LOBSTER_AUTH_HEADER", tt.env)
			_, err := BuildAuthConfig(&tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("error echoes the flag's text: %v", err)
			}
		})
	}
}

func TestRemovedFlags(t *testing.T) {
	for _, name := range []string{"-dry-run", "-follow-links", "-respect-429"} {
		fs := flag.NewFlagSet("lobster", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		RegisterConfigFlags(fs)
		if err := fs.Parse([]string{name}); err == nil {
			t.Errorf("%s parsed; it was removed", name)
		}
	}
}

func TestExampleConfigLoads(t *testing.T) {
	cfg, err := load(t, filepath.Join("..", "..", "examples", "config.example.json"))
	if err != nil {
		t.Fatalf("examples/config.example.json: %v", err)
	}
	want := domain.PerformanceTargets{SuccessRate: 99}
	if cfg.PerformanceTargets != want {
		t.Errorf("example targets = %+v, want success rate only", cfg.PerformanceTargets)
	}
}

func TestBuildAuthConfig_Cookie(t *testing.T) {
	t.Setenv("LOBSTER_AUTH_COOKIE", "session=abc123")

	opts := &ConfigOptions{
		AuthType: "cookie",
	}
	cfg, err := BuildAuthConfig(opts)
	if err != nil {
		t.Fatalf("BuildAuthConfig() error = %v", err)
	}

	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}
	if cfg.Cookies == nil {
		t.Fatal("Expected non-nil Cookies map")
	}
	if cfg.Cookies["session"] != "abc123" {
		t.Errorf("Expected cookie value 'abc123', got '%s'", cfg.Cookies["session"])
	}
}

func TestBuildAuthConfig_InvalidCookie(t *testing.T) {
	t.Setenv("LOBSTER_AUTH_COOKIE", "invalid-no-equals")

	opts := &ConfigOptions{
		AuthType: "cookie",
	}
	_, err := BuildAuthConfig(opts)
	if err == nil {
		t.Error("Expected error for invalid cookie format")
	}
}

func TestBuildAuthConfig_Bearer(t *testing.T) {
	t.Setenv("LOBSTER_AUTH_TOKEN", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test")

	opts := &ConfigOptions{
		AuthType: "bearer",
	}
	cfg, err := BuildAuthConfig(opts)
	if err != nil {
		t.Fatalf("BuildAuthConfig() error = %v", err)
	}

	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}
	if cfg.Type != "bearer" {
		t.Errorf("Expected Type 'bearer', got '%s'", cfg.Type)
	}
	if cfg.Token != "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test" {
		t.Errorf("Expected Token to match, got '%s'", cfg.Token)
	}
}

func TestConfirmLowRate(t *testing.T) {
	// Rates below WarnRate prompt when stdin is a terminal, so only rates at or above it are tested.
	for _, rate := range []float64{WarnRate, 5} {
		if err := ConfirmLowRate(io.Discard, rate); err != nil {
			t.Errorf("ConfirmLowRate(%g) error = %v", rate, err)
		}
	}
}

func TestCenterText(t *testing.T) {
	tests := []struct {
		text     string
		width    int
		expected string
	}{
		{"test", 10, "   test   "},
		{"hello", 5, "hello"},
		{"x", 3, " x "},
		{"longer text", 5, "longer text"}, // Text longer than width
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := CenterText(tt.text, tt.width)
			if result != tt.expected {
				t.Errorf("CenterText(%q, %d) = %q, want %q", tt.text, tt.width, result, tt.expected)
			}
		})
	}
}

func TestIsInteractiveTerminal(_ *testing.T) {
	// In a test environment, stdin is typically not a terminal
	// We can't assert the exact value since it depends on how tests are run,
	// but we can verify it doesn't panic
	_ = IsInteractiveTerminal()
}
