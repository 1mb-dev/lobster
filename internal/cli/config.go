package cli

import (
	"flag"
	"fmt"

	"github.com/1mb-dev/lobster/v2/internal/config"
	"github.com/1mb-dev/lobster/v2/internal/domain"
)

// RegisterConfigFlags defines the flags that feed domain.Config on fs.
// Their defaults are never applied: LoadConfiguration copies only the flags
// that were set, so defaults come from domain.DefaultConfig.
func RegisterConfigFlags(fs *flag.FlagSet) *ConfigOptions {
	o := &ConfigOptions{}
	fs.StringVar(&o.BaseURL, "url", "", "Base URL to test")
	fs.IntVar(&o.Concurrency, "concurrency", 0, "Number of concurrent workers")
	fs.StringVar(&o.Duration, "duration", "", "Test duration (e.g., 30s, 5m, 1h)")
	fs.StringVar(&o.Timeout, "timeout", "", "Request timeout")
	fs.Float64Var(&o.Rate, "rate", 0, "Requests per second, shared by all workers")
	fs.StringVar(&o.UserAgent, "user-agent", "", "User agent string")
	fs.IntVar(&o.MaxDepth, "max-depth", 0, "Maximum crawl depth (0 = base URL only)")
	fs.IntVar(&o.QueueSize, "queue-size", 0, "URL queue buffer size")
	fs.BoolVar(&o.InsecureSkipVerify, "insecure-skip-verify", false, "INSECURE: Skip TLS certificate verification")
	fs.BoolVar(&o.IgnoreRobots, "ignore-robots", false, "Ignore robots.txt directives (use responsibly)")
	fs.StringVar(&o.OutputFile, "output", "", "JSON results file")
	fs.StringVar(&o.HTMLFile, "html", "", "HTML report file")
	fs.BoolVar(&o.Verbose, "verbose", false, "Verbose logging")
	fs.StringVar(&o.AuthType, "auth-type", "", "Authentication type: basic, bearer, cookie, header")
	fs.StringVar(&o.AuthUsername, "auth-username", "", "Username for basic authentication")
	fs.BoolVar(&o.AuthPasswordStdin, "auth-password-stdin", false, "Read password from stdin (one line)")
	fs.BoolVar(&o.AuthTokenStdin, "auth-token-stdin", false, "Read bearer token from stdin (one line)")
	fs.StringVar(&o.AuthHeader, "auth-header", "", "Custom header name; value from LOBSTER_AUTH_HEADER or -auth-header-stdin")
	fs.BoolVar(&o.AuthHeaderStdin, "auth-header-stdin", false, "Read the -auth-header value from stdin (one line)")
	return o
}

// LoadConfiguration builds the run's config: defaults, then the config file
// (if any) over them, then each flag set on fs, then validation.
func LoadConfiguration(fs *flag.FlagSet, configPath string, opts *ConfigOptions) (*domain.Config, error) {
	cfg := domain.DefaultConfig()

	if configPath != "" {
		if err := config.NewLoader().LoadFromFile(configPath, &cfg); err != nil {
			return nil, err
		}
	}

	setters := flagSetters(&cfg, opts)
	fs.Visit(func(f *flag.Flag) {
		if set, ok := setters[f.Name]; ok {
			set()
		}
	})

	// Build authentication configuration from CLI flags and environment variables
	authCfg, err := BuildAuthConfig(opts)
	if err != nil {
		return nil, fmt.Errorf("authentication configuration: %w", err)
	}
	if authCfg != nil {
		cfg.Auth = authCfg
	}

	if validateErr := cfg.Validate(); validateErr != nil {
		return nil, validateErr
	}
	return &cfg, nil
}

// flagSetters maps each config flag's name to the copy of its value into cfg.
func flagSetters(cfg *domain.Config, o *ConfigOptions) map[string]func() {
	return map[string]func(){
		"url":                  func() { cfg.BaseURL = o.BaseURL },
		"concurrency":          func() { cfg.Concurrency = o.Concurrency },
		"duration":             func() { cfg.Duration = o.Duration },
		"timeout":              func() { cfg.Timeout = o.Timeout },
		"rate":                 func() { cfg.Rate = o.Rate },
		"user-agent":           func() { cfg.UserAgent = o.UserAgent },
		"max-depth":            func() { cfg.MaxDepth = o.MaxDepth },
		"queue-size":           func() { cfg.QueueSize = o.QueueSize },
		"output":               func() { cfg.OutputFile = o.OutputFile },
		"html":                 func() { cfg.HTMLFile = o.HTMLFile },
		"verbose":              func() { cfg.Verbose = o.Verbose },
		"insecure-skip-verify": func() { cfg.InsecureSkipVerify = o.InsecureSkipVerify },
		"ignore-robots":        func() { cfg.IgnoreRobots = o.IgnoreRobots },
	}
}
