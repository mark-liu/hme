package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadCookies(t *testing.T) {
	tmp := t.TempDir()
	// Override HOME so configDir() uses the temp directory
	t.Setenv("HOME", tmp)

	cookie := "X-APPLE-WEBAUTH-USER=d123456789%200; X-APPLE-WEBAUTH-TOKEN=sometoken"

	if err := SaveCookies(cookie); err != nil {
		t.Fatalf("SaveCookies: %v", err)
	}

	// Verify file permissions
	p := filepath.Join(tmp, ".config", configDirName, cookieFileName)
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat cookie file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("cookie file perms = %o, want 0600", perm)
	}

	// Verify directory permissions
	dirInfo, err := os.Stat(filepath.Dir(p))
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0700 {
		t.Errorf("config dir perms = %o, want 0700", perm)
	}

	got, err := LoadCookies()
	if err != nil {
		t.Fatalf("LoadCookies: %v", err)
	}
	if got != cookie {
		t.Errorf("LoadCookies = %q, want %q", got, cookie)
	}
}

func TestLoadCookies_NotExist(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	_, err := LoadCookies()
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}

func TestLoadCookies_Empty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dir := filepath.Join(tmp, ".config", configDirName)
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, cookieFileName), []byte("  \n"), 0600)

	_, err := LoadCookies()
	if err == nil {
		t.Error("expected error for empty cookie file")
	}
}

func TestExtractDSID(t *testing.T) {
	tests := []struct {
		name    string
		cookies string
		want    string
		wantErr bool
	}{
		{
			name:    "standard URL-encoded dsid",
			cookies: "X-APPLE-WEBAUTH-USER=d123456789%200; X-APPLE-WEBAUTH-TOKEN=abc",
			want:    "d123456789",
		},
		{
			name:    "dsid without encoding",
			cookies: "other=foo; X-APPLE-WEBAUTH-USER=d987654321; bar=baz",
			want:    "d987654321",
		},
		{
			name:    "missing dsid",
			cookies: "X-APPLE-WEBAUTH-TOKEN=abc; other=def",
			wantErr: true,
		},
		{
			name:    "empty cookie string",
			cookies: "",
			wantErr: true,
		},
		{
			name:    "dsid with multiple % segments",
			cookies: "X-APPLE-WEBAUTH-USER=d111222333%20something%20else",
			want:    "d111222333",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractDSID(tt.cookies)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ExtractDSID = %q, want %q", got, tt.want)
			}
		})
	}
}
