package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

const (
	chromeSalt       = "saltysalt"
	chromeIterations = 1003
	chromeKeyLen     = 16
	chromeIV         = "                " // 16 spaces (0x20)
)

type rawCookie struct {
	Name           string
	EncryptedValue []byte
	HostKey        string
}

// ExtractChromeCookies reads iCloud cookies from Chrome's cookie store.
// On macOS, accessing the encryption key triggers a Touch ID / system password prompt.
func ExtractChromeCookies(profile string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("Chrome cookie extraction is only supported on macOS")
	}

	// 1. Get Chrome Safe Storage password (triggers Touch ID)
	fmt.Fprintln(os.Stderr, "Reading Chrome Safe Storage key (Touch ID prompt)...")
	password, err := chromePassword()
	if err != nil {
		return "", err
	}

	// 2. Derive AES key
	key := deriveKey(password)

	// 3. Find the right Chrome profile
	dbPath, err := chromeCookieDB(profile)
	if err != nil {
		return "", err
	}

	// 4. Check DB version (v24+ has SHA-256 prefix in decrypted values)
	dbVersion, err := chromeDBVersion(dbPath)
	if err != nil {
		return "", err
	}

	// 5. Query iCloud cookies
	raw, err := queryiCloudCookies(dbPath)
	if err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return "", fmt.Errorf("no iCloud cookies found in Chrome\nSign in to https://www.icloud.com in Chrome first")
	}

	// 6. Decrypt and assemble cookie string
	var parts []string
	for _, c := range raw {
		val, err := decryptCookieValue(c.EncryptedValue, key, c.HostKey, dbVersion)
		if err != nil {
			continue // skip undecryptable cookies
		}
		if val != "" {
			parts = append(parts, c.Name+"="+val)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("could not decrypt any iCloud cookies")
	}

	fmt.Fprintf(os.Stderr, "Extracted %d cookies from Chrome.\n", len(parts))
	return strings.Join(parts, "; "), nil
}

// chromePassword retrieves the Chrome Safe Storage password from macOS Keychain.
func chromePassword() (string, error) {
	out, err := exec.Command("security", "find-generic-password", "-w", "-s", "Chrome Safe Storage").Output()
	if err != nil {
		return "", fmt.Errorf("could not read Chrome Safe Storage from Keychain: %w\nIs Chrome installed?", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// deriveKey derives the AES-128 key from the Chrome Safe Storage password.
func deriveKey(password string) []byte {
	return pbkdf2.Key([]byte(password), []byte(chromeSalt), chromeIterations, chromeKeyLen, sha1.New)
}

// chromeDataDirs returns all Chrome user-data directories to search.
// Includes the standard location and any custom --user-data-dir from running processes.
func chromeDataDirs() []string {
	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(home, "Library", "Application Support", "Google", "Chrome"),
	}

	// Detect custom --user-data-dir from running Chrome processes
	out, err := exec.Command("ps", "-xo", "args=").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if !strings.Contains(line, "Google Chrome") {
				continue
			}
			for _, arg := range strings.Fields(line) {
				if strings.HasPrefix(arg, "--user-data-dir=") {
					dir := strings.TrimPrefix(arg, "--user-data-dir=")
					if dir != "" && dir != dirs[0] {
						dirs = append(dirs, dir)
					}
				}
			}
		}
	}
	return dirs
}

// chromeCookieDB returns the path to the Chrome Cookies SQLite database.
// If profile is empty, it auto-detects by scanning all Chrome data directories
// and all profiles within them.
func chromeCookieDB(profile string) (string, error) {
	dataDirs := chromeDataDirs()

	if profile != "" {
		for _, dir := range dataDirs {
			p := filepath.Join(dir, profile, "Cookies")
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		return "", fmt.Errorf("Chrome profile %q not found in any Chrome data directory", profile)
	}

	// Auto-detect: collect all Cookie DBs, prefer one that has iCloud cookies
	var allDBs []string
	for _, dir := range dataDirs {
		candidates := []string{"Default"}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "Profile ") {
				candidates = append(candidates, e.Name())
			}
		}
		for _, name := range candidates {
			p := filepath.Join(dir, name, "Cookies")
			if _, err := os.Stat(p); err == nil {
				allDBs = append(allDBs, p)
			}
		}
	}

	if len(allDBs) == 0 {
		return "", fmt.Errorf("no Chrome profile with a Cookies database found")
	}

	// Prefer a profile that actually has iCloud cookies
	for _, db := range allDBs {
		if hasICloudCookies(db) {
			return db, nil
		}
	}

	// Fall back to first available
	return allDBs[0], nil
}

// hasICloudCookies checks if a Cookies database has any iCloud cookies.
func hasICloudCookies(dbPath string) bool {
	uri := "file://" + dbPath + "?immutable=1"
	out, err := exec.Command("sqlite3", uri,
		"SELECT count(*) FROM cookies WHERE host_key LIKE '%.icloud.com';").Output()
	if err != nil {
		return false
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n > 0
}

// chromeDBVersion reads the database version from the meta table.
func chromeDBVersion(dbPath string) (int, error) {
	uri := "file://" + dbPath + "?immutable=1"
	out, err := exec.Command("sqlite3", uri,
		"SELECT value FROM meta WHERE key='version';").Output()
	if err != nil {
		return 0, fmt.Errorf("reading Chrome DB version: %w", err)
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("parsing Chrome DB version: %w", err)
	}
	return v, nil
}

// queryiCloudCookies reads iCloud cookies from Chrome's Cookies database using sqlite3 CLI.
// Uses ?immutable=1 to bypass Chrome's file lock.
func queryiCloudCookies(dbPath string) ([]rawCookie, error) {
	uri := "file://" + dbPath + "?immutable=1"
	out, err := exec.Command("sqlite3", "-separator", "\x1f", uri,
		"SELECT name, hex(encrypted_value), host_key FROM cookies WHERE host_key LIKE '%.icloud.com' ORDER BY name;").Output()
	if err != nil {
		return nil, fmt.Errorf("querying Chrome cookie database: %w", err)
	}

	var cookies []rawCookie
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 3)
		if len(parts) != 3 {
			continue
		}
		encBytes, err := hex.DecodeString(parts[1])
		if err != nil {
			continue
		}
		cookies = append(cookies, rawCookie{
			Name:           parts[0],
			EncryptedValue: encBytes,
			HostKey:        parts[2],
		})
	}
	return cookies, nil
}

// decryptCookieValue decrypts a Chrome v10 encrypted cookie value.
// For DB version >= 24, strips the 32-byte SHA-256(host_key) prefix from the plaintext.
func decryptCookieValue(encrypted []byte, key []byte, hostKey string, dbVersion int) (string, error) {
	if len(encrypted) < 3 {
		return "", fmt.Errorf("value too short")
	}
	if string(encrypted[:3]) != "v10" {
		return "", fmt.Errorf("unsupported encryption version: %q", string(encrypted[:3]))
	}
	ciphertext := encrypted[3:]

	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return "", fmt.Errorf("invalid ciphertext length")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, []byte(chromeIV)).CryptBlocks(plaintext, ciphertext)

	// Remove PKCS#7 padding
	plaintext, err = pkcs7Unpad(plaintext)
	if err != nil {
		return "", err
	}

	// DB version >= 24: first 32 bytes are SHA-256(host_key)
	if dbVersion >= 24 {
		if len(plaintext) < 32 {
			return "", fmt.Errorf("decrypted value too short for v24 format")
		}
		// Optionally verify the hash
		expectedHash := sha256.Sum256([]byte(hostKey))
		if string(plaintext[:32]) != string(expectedHash[:]) {
			return "", fmt.Errorf("host_key hash mismatch")
		}
		plaintext = plaintext[32:]
	}

	return string(plaintext), nil
}

// pkcs7Unpad removes PKCS#7 padding from plaintext.
func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > aes.BlockSize || padLen > len(data) {
		return nil, fmt.Errorf("invalid PKCS#7 padding")
	}
	for _, b := range data[len(data)-padLen:] {
		if int(b) != padLen {
			return nil, fmt.Errorf("invalid PKCS#7 padding")
		}
	}
	return data[:len(data)-padLen], nil
}
