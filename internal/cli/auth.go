package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/1mb-dev/lobster/v2/internal/domain"
)

// BuildAuthConfig builds authentication configuration from environment variables and stdin.
// Credentials are read from:
// 1. Environment variables (LOBSTER_AUTH_PASSWORD, LOBSTER_AUTH_TOKEN, LOBSTER_AUTH_COOKIE, LOBSTER_AUTH_HEADER)
// 2. Stdin when -auth-password-stdin, -auth-token-stdin or -auth-header-stdin is set
// CLI flags for credentials are intentionally not supported to prevent exposure in process lists.
func BuildAuthConfig(opts *ConfigOptions) (*domain.AuthConfig, error) {
	// Only one value can be read from stdin
	stdinFlags := 0
	for _, set := range []bool{opts.AuthPasswordStdin, opts.AuthTokenStdin, opts.AuthHeaderStdin} {
		if set {
			stdinFlags++
		}
	}
	if stdinFlags > 1 {
		return nil, fmt.Errorf("-auth-password-stdin, -auth-token-stdin and -auth-header-stdin are mutually exclusive")
	}

	headers, err := headerAuth(opts)
	if err != nil {
		return nil, err
	}

	// Get credentials from environment variables
	password := os.Getenv("LOBSTER_AUTH_PASSWORD")
	token := os.Getenv("LOBSTER_AUTH_TOKEN")
	cookie := os.Getenv("LOBSTER_AUTH_COOKIE")

	// Read from stdin if requested (overrides env vars)
	if opts.AuthPasswordStdin {
		stdinPassword, readErr := ReadSecretFromStdin("password")
		if readErr != nil {
			return nil, readErr
		}
		password = stdinPassword
	}

	if opts.AuthTokenStdin {
		stdinToken, readErr := ReadSecretFromStdin("token")
		if readErr != nil {
			return nil, readErr
		}
		token = stdinToken
	}

	// Check if any auth configuration is provided
	hasAuth := opts.AuthType != "" || opts.AuthUsername != "" || headers != nil ||
		password != "" || token != "" || cookie != ""

	if !hasAuth {
		return nil, nil
	}

	authCfg := &domain.AuthConfig{
		Type:     opts.AuthType,
		Username: opts.AuthUsername,
		Password: password,
		Token:    token,
		Headers:  headers,
	}

	// Parse cookie string (name=value) from env var
	if cookie != "" {
		parts := strings.SplitN(cookie, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("invalid LOBSTER_AUTH_COOKIE format: expected 'name=value', got %q", cookie)
		}
		authCfg.Cookies = make(map[string]string)
		authCfg.Cookies[parts[0]] = parts[1]
	}

	return authCfg, nil
}

// headerAuth returns the -auth-header name mapped to its value from
// LOBSTER_AUTH_HEADER or stdin, or nil when -auth-header is unset.
// Errors never include the flag's text: it may hold a secret.
func headerAuth(opts *ConfigOptions) (map[string]string, error) {
	name := strings.TrimSpace(opts.AuthHeader)
	if name == "" {
		if opts.AuthHeaderStdin {
			return nil, fmt.Errorf("-auth-header-stdin needs -auth-header <Name>")
		}
		return nil, nil
	}
	if strings.ContainsAny(name, ": \t") {
		return nil, fmt.Errorf("-auth-header takes the header name only; " +
			"set the value in LOBSTER_AUTH_HEADER or pipe it with -auth-header-stdin")
	}

	value := os.Getenv("LOBSTER_AUTH_HEADER")
	if opts.AuthHeaderStdin {
		stdinValue, err := ReadSecretFromStdin("header value")
		if err != nil {
			return nil, err
		}
		value = stdinValue
	}
	if value == "" {
		return nil, fmt.Errorf("-auth-header needs a value: set LOBSTER_AUTH_HEADER or use -auth-header-stdin")
	}
	return map[string]string{name: value}, nil
}

// ReadSecretFromStdin reads a single line from stdin for secure credential input.
// Returns an error if stdin is empty or closed without data.
func ReadSecretFromStdin(name string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("reading %s from stdin: %w", name, err)
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", fmt.Errorf("%s from stdin is empty", name)
	}
	return trimmed, nil
}
