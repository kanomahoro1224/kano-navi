package dotenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLineReadsTheSupportedForms(t *testing.T) {
	for _, tc := range []struct {
		line, key, value string
		ok               bool
	}{
		{"KEY=value", "KEY", "value", true},
		{"  KEY = spaced value  ", "KEY", "spaced value", true},
		{"export KEY=value", "KEY", "value", true},
		{"\uFEFFKEY=value", "KEY", "value", true},
		{"KEY=value # comment", "KEY", "value", true},
		{"KEY=a#b", "KEY", "a#b", true},
		{`KEY="quoted # kept"`, "KEY", "quoted # kept", true},
		{`KEY="  padded  "`, "KEY", "  padded  ", true},
		{`KEY="line\nbreak \"q\" \\n"`, "KEY", "line\nbreak \"q\" \\n", true},
		{`KEY='single \n raw'`, "KEY", `single \n raw`, true},
		{"KEY=`back`", "KEY", "back", true},
		{`KEY="unterminated`, "KEY", `"unterminated`, true},
		{"KEY=", "KEY", "", true},
		{"", "", "", false},
		{"# KEY=value", "", "", false},
		{"=value", "", "", false},
		{"NO_SEPARATOR", "", "", false},
		{"BAD-KEY=value", "", "", false},
		{"BAD KEY=value", "", "", false},
	} {
		key, value, ok := parseLine(tc.line)
		if key != tc.key || value != tc.value || ok != tc.ok {
			t.Errorf("parseLine(%q) = %q, %q, %v; want %q, %q, %v", tc.line, key, value, ok, tc.key, tc.value, tc.ok)
		}
	}
}

func TestLoadKeepsTheEnvironmentAndIgnoresAMissingFile(t *testing.T) {
	if err := Load(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Fatalf("a missing file: %v", err)
	}
	file := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(file, []byte("KANO_DOTENV_SET=from-file\nKANO_DOTENV_NEW=from-file\n# comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KANO_DOTENV_SET", "from-environment")
	t.Setenv("KANO_DOTENV_NEW", "")
	os.Unsetenv("KANO_DOTENV_NEW")
	if err := Load(file); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("KANO_DOTENV_SET"); got != "from-environment" {
		t.Fatalf("the file replaced the environment: %q", got)
	}
	if got := os.Getenv("KANO_DOTENV_NEW"); got != "from-file" {
		t.Fatalf("a variable the environment lacks was not loaded: %q", got)
	}
}
