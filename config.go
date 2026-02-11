package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

const configDirName = "hme"
const cookieFileName = "cookies.txt"

// configDir returns ~/.config/hme, creating it with 0700 if needed.
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", configDirName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("could not create config directory: %w", err)
	}
	return dir, nil
}

// cookiePath returns the full path to the cookie file.
func cookiePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, cookieFileName), nil
}

// LoadCookies reads the cookie string from disk.
// Returns os.ErrNotExist if the file doesn't exist.
func LoadCookies() (string, error) {
	p, err := cookiePath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return "", fmt.Errorf("cookie file is empty; run 'hme auth' to configure")
	}
	return s, nil
}

// SaveCookies writes the cookie string to disk with 0600 perms.
func SaveCookies(cookies string) error {
	p, err := cookiePath()
	if err != nil {
		return err
	}
	return os.WriteFile(p, []byte(strings.TrimSpace(cookies)+"\n"), 0600)
}

// ExtractDSID parses the dsid (X-APPLE-WEBAUTH-USER cookie or dsid param) from a cookie string.
// Never logs the full cookie value.
func ExtractDSID(cookies string) (string, error) {
	for _, part := range strings.Split(cookies, ";") {
		part = strings.TrimSpace(part)
		// Look for X-APPLE-WEBAUTH-USER or dsid
		if strings.HasPrefix(part, "X-APPLE-WEBAUTH-USER=") {
			val := strings.TrimPrefix(part, "X-APPLE-WEBAUTH-USER=")
			// The value is URL-encoded; the dsid is the part before the first %
			// or the entire value if no % present
			if idx := strings.Index(val, "%"); idx > 0 {
				return val[:idx], nil
			}
			if val != "" {
				return val, nil
			}
		}
	}
	return "", fmt.Errorf("could not extract dsid from cookies; ensure you copied the full cookie string")
}

// PromptForCookies reads a cookie string from stdin with echo disabled.
func PromptForCookies() (string, error) {
	fmt.Fprint(os.Stderr, "Paste your iCloud cookie string (input hidden): ")
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr) // newline after hidden input
	if err != nil {
		return "", fmt.Errorf("could not read input: %w", err)
	}
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return "", fmt.Errorf("empty input")
	}
	return s, nil
}
