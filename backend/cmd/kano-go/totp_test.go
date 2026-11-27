package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/adminauth"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

func TestTOTPSecretCommandPrintsASecretAndItsAddress(t *testing.T) {
	var out bytes.Buffer
	if code := runTOTPSecret(nil, &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	match := regexp.MustCompile(`^ADMIN_TOTP_SECRET=([A-Z2-7]{32})\n# (otpauth://totp/\S+)\n$`).FindStringSubmatch(out.String())
	if match == nil {
		t.Fatalf("output=%q", out.String())
	}
	if secret, err := adminauth.ParseTOTPSecret(match[1]); err != nil || len(secret) != 20 {
		t.Fatalf("secret does not parse: %v", err)
	}
	if match[2] != adminauth.TOTPURI(match[1]) {
		t.Fatalf("address=%q", match[2])
	}
	if code := runTOTPSecret([]string{"-x"}, &out); code != 2 {
		t.Fatalf("argument exit %d", code)
	}
}

func TestTOTPResetRemovesTheConsolesCodeFromAnExistingDatabaseOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database", "kano.sqlite")
	store, err := sqlite.OpenStandalone(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB().Exec(`INSERT INTO app_settings(key,value,updated_at) VALUES('admin_totp','synthetic-sealed','2026-11-27T00:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	store.Close()
	var out bytes.Buffer
	if code := runTOTPReset([]string{"-database", path}, &out); code != 0 || !strings.Contains(out.String(), "removed") {
		t.Fatalf("reset exit %d: %q", code, out.String())
	}
	out.Reset()
	if code := runTOTPReset([]string{"-database", path}, &out); code != 0 || !strings.Contains(out.String(), "No one-time code") {
		t.Fatalf("second reset exit %d: %q", code, out.String())
	}
	missing := filepath.Join(t.TempDir(), "missing.sqlite")
	if code := runTOTPReset([]string{"-database", missing}, &out); code != 1 {
		t.Fatalf("missing database exit %d", code)
	}
	if _, err = os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("reset created a database")
	}
	if code := runTOTPReset([]string{"extra"}, &out); code != 2 {
		t.Fatalf("argument exit %d", code)
	}
}
