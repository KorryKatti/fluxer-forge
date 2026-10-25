// Package token finds the bot token.
//
// Order: FLUXER_TOKEN, then ./.env, then ./token, then an interactive prompt.
// Nothing is ever logged.
package token

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// FromEnv returns FLUXER_TOKEN or "".
func FromEnv() string {
	return strings.TrimSpace(os.Getenv("FLUXER_TOKEN"))
}

// FromDotEnv reads FLUXER_TOKEN from a .env file. A missing file is not an
// error: most users have no .env.
func FromDotEnv(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "FLUXER_TOKEN" {
			continue
		}
		return cleanValue(value), nil
	}
	return "", nil
}

// FromFile reads a token stored in its own file.
func FromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// Prompt asks for the token on the terminal without echoing it, then saves
// it to path with 0600 so the next run does not ask again.
func Prompt(path string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("no token: set FLUXER_TOKEN, or add FLUXER_TOKEN to .env")
	}

	fmt.Fprint(os.Stderr, "Bot token: ")
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read token: %w", err)
	}

	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", errors.New("no token entered")
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("save token: %w", err)
	}
	return token, nil
}

// cleanValue removes spaces and matching single or double quotes.
func cleanValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return value
}
