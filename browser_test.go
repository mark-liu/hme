package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"crypto/sha256"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

func TestDeriveKey(t *testing.T) {
	// Verify key derivation produces consistent 16-byte output
	key := deriveKey("testpassword")
	if len(key) != 16 {
		t.Fatalf("key length = %d, want 16", len(key))
	}

	// Same password → same key
	key2 := deriveKey("testpassword")
	if string(key) != string(key2) {
		t.Error("same password produced different keys")
	}

	// Different password → different key
	key3 := deriveKey("otherpassword")
	if string(key) == string(key3) {
		t.Error("different passwords produced same key")
	}
}

// testEncrypt encrypts a value using Chrome's v10 format for round-trip testing.
func testEncrypt(t *testing.T, plaintext, password, hostKey string, dbVersion int) []byte {
	t.Helper()
	key := pbkdf2.Key([]byte(password), []byte(chromeSalt), chromeIterations, chromeKeyLen, sha1.New)

	payload := []byte(plaintext)
	if dbVersion >= 24 {
		hash := sha256.Sum256([]byte(hostKey))
		payload = append(hash[:], payload...)
	}

	// PKCS#7 pad
	padLen := aes.BlockSize - (len(payload) % aes.BlockSize)
	for i := 0; i < padLen; i++ {
		payload = append(payload, byte(padLen))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	ct := make([]byte, len(payload))
	cipher.NewCBCEncrypter(block, []byte(chromeIV)).CryptBlocks(ct, payload)

	// Prepend "v10"
	return append([]byte("v10"), ct...)
}

func TestDecryptCookieValue_V23(t *testing.T) {
	password := "mysecretpassword"
	key := deriveKey(password)
	hostKey := ".example.com"
	plaintext := "some_cookie_value_12345"

	encrypted := testEncrypt(t, plaintext, password, hostKey, 23)

	got, err := decryptCookieValue(encrypted, key, hostKey, 23)
	if err != nil {
		t.Fatalf("decryptCookieValue: %v", err)
	}
	if got != plaintext {
		t.Errorf("got %q, want %q", got, plaintext)
	}
}

func TestDecryptCookieValue_V24(t *testing.T) {
	password := "anothersecret"
	key := deriveKey(password)
	hostKey := ".icloud.com"
	plaintext := "d123456789%20abc"

	encrypted := testEncrypt(t, plaintext, password, hostKey, 24)

	got, err := decryptCookieValue(encrypted, key, hostKey, 24)
	if err != nil {
		t.Fatalf("decryptCookieValue: %v", err)
	}
	if got != plaintext {
		t.Errorf("got %q, want %q", got, plaintext)
	}
}

func TestDecryptCookieValue_V24_HashMismatch(t *testing.T) {
	password := "secret"
	key := deriveKey(password)
	hostKey := ".icloud.com"
	wrongHost := ".wrong.com"

	encrypted := testEncrypt(t, "value", password, hostKey, 24)

	// Decrypt with wrong hostKey should fail hash verification
	_, err := decryptCookieValue(encrypted, key, wrongHost, 24)
	if err == nil {
		t.Error("expected hash mismatch error")
	}
}

func TestDecryptCookieValue_BadPrefix(t *testing.T) {
	_, err := decryptCookieValue([]byte("v11deadbeef"), nil, "", 23)
	if err == nil {
		t.Error("expected error for unsupported version prefix")
	}
}

func TestDecryptCookieValue_TooShort(t *testing.T) {
	_, err := decryptCookieValue([]byte("v1"), nil, "", 23)
	if err == nil {
		t.Error("expected error for too-short value")
	}
}

func TestDecryptCookieValue_EmptyPlaintext(t *testing.T) {
	// Encrypt an empty string (v23, no SHA prefix)
	password := "pwd"
	key := deriveKey(password)
	encrypted := testEncrypt(t, "", password, "", 23)

	got, err := decryptCookieValue(encrypted, key, "", 23)
	if err != nil {
		t.Fatalf("decryptCookieValue: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestPkcs7Unpad(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    []byte
		wantErr bool
	}{
		{
			name:  "pad 1",
			input: []byte{0x41, 0x42, 0x43, 0x01},
			want:  []byte{0x41, 0x42, 0x43},
		},
		{
			name:  "pad 4",
			input: []byte{0x41, 0x04, 0x04, 0x04, 0x04},
			want:  []byte{0x41},
		},
		{
			name:  "full block padding",
			input: bytes16(0x10),
			want:  []byte{},
		},
		{
			name:    "empty input",
			input:   []byte{},
			wantErr: true,
		},
		{
			name:    "pad zero",
			input:   []byte{0x41, 0x00},
			wantErr: true,
		},
		{
			name:    "pad too large",
			input:   []byte{0x41, 0x42, 0x11},
			wantErr: true,
		},
		{
			name:    "inconsistent padding bytes",
			input:   []byte{0x41, 0x02, 0x03},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pkcs7Unpad(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != string(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// bytes16 returns a 16-byte slice filled with b.
func bytes16(b byte) []byte {
	out := make([]byte, 16)
	for i := range out {
		out[i] = b
	}
	return out
}

// Verify our test helper uses the same derivation as the production code.
func TestDeriveKey_Consistency(t *testing.T) {
	password := "Chrome Safe Storage Test"
	got := deriveKey(password)
	want := pbkdf2.Key([]byte(password), []byte("saltysalt"), 1003, 16, sha1.New)
	if string(got) != string(want) {
		t.Error("deriveKey does not match manual PBKDF2")
	}
}
