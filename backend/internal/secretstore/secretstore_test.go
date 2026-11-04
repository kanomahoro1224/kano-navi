package secretstore

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTripAndFailures(t *testing.T) {
	sealed, err := Encrypt("synthetic-value", "synthetic-key")
	if err != nil || sealed == "" {
		t.Fatalf("encrypt: %v", err)
	}
	plain, err := Decrypt(sealed, "synthetic-key")
	if err != nil || plain != "synthetic-value" {
		t.Fatalf("decrypt: %q %v", plain, err)
	}
	if _, err = Decrypt(sealed, "another-key"); err != ErrDecrypt {
		t.Fatalf("wrong key: %v", err)
	}
	if _, err = Decrypt("v0:a:b:c", "synthetic-key"); err != ErrFormat {
		t.Fatalf("format: %v", err)
	}
	if _, err = Encrypt("value", ""); err != ErrKeyRequired {
		t.Fatalf("missing key: %v", err)
	}
	if sealed, err = Encrypt("", "synthetic-key"); err != nil || sealed != "" {
		t.Fatalf("empty: %q %v", sealed, err)
	}
}

// This fixture is produced by the TypeScript server/secret-store.ts format
// with Node crypto, a fixed nonce, and the same synthetic key. Keeping it in
// the repository makes the compatibility check mandatory in every test run.
func TestReadsTypeScriptCiphertext(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "typescript-ciphertext.txt"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Decrypt(strings.TrimSpace(string(fixture)), "synthetic-key")
	if err != nil || plain != "synthetic-value" {
		t.Fatalf("typescript ciphertext: %q %v", plain, err)
	}
}

// Each encryption draws a fresh nonce: the same value never seals to the same
// text twice, and tampering with the sealed text fails authentication.
func TestEncryptionUsesAFreshNonceAndIsAuthenticated(t *testing.T) {
	const secret = "synthetic-secrets-key-0123456789"
	seen := map[string]bool{}
	nonces := map[string]bool{}
	for i := 0; i < 64; i++ {
		sealed, err := Encrypt("synthetic-provider-key", secret)
		if err != nil {
			t.Fatal(err)
		}
		if seen[sealed] {
			t.Fatal("the same value sealed to the same text twice")
		}
		seen[sealed] = true
		parts := strings.Split(sealed, ":")
		if len(parts) < 3 {
			t.Fatalf("unexpected sealed form %q", sealed)
		}
		if nonces[parts[1]] {
			t.Fatal("a nonce was reused")
		}
		nonces[parts[1]] = true
		// Flipping one bit of the ciphertext must not decrypt.
		body, err := base64.RawURLEncoding.DecodeString(parts[3])
		if err != nil || len(body) == 0 {
			t.Fatalf("ciphertext %q: %v", parts[3], err)
		}
		body[0] ^= 1
		parts[3] = base64.RawURLEncoding.EncodeToString(body)
		if _, err := Decrypt(strings.Join(parts, ":"), secret); err == nil {
			t.Fatal("a tampered value decrypted")
		}
	}
}
