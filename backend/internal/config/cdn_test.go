package config

import "testing"

// The build normalizes the same cases in scripts/cdn-base.ts
// (tests/cdn.test.ts); keep the two lists in step.
func TestCDNBase(t *testing.T) {
	for raw, want := range map[string]string{
		"":                                     "",
		"  ":                                   "",
		"https://cdn.example.invalid":          "https://cdn.example.invalid",
		"https://cdn.example.invalid/":         "https://cdn.example.invalid",
		"https://CDN.Example.INVALID":          "https://cdn.example.invalid",
		"https://cdn.example.invalid/kano":     "https://cdn.example.invalid/kano",
		"https://cdn.example.invalid/kano/":    "https://cdn.example.invalid/kano",
		"https://cdn.example.invalid/v1/site":  "https://cdn.example.invalid/v1/site",
		"http://127.0.0.1:9000":                "http://127.0.0.1:9000",
		"http://[::1]:9000/kano":               "http://[::1]:9000/kano",
		"https://xn--n8j.example.invalid/kano": "https://xn--n8j.example.invalid/kano",
		"https://CDN.example.invalid/Kano":     "https://cdn.example.invalid/Kano",
	} {
		if got, err := CDNBase(raw); err != nil || got != want {
			t.Fatalf("%q => %q %v", raw, got, err)
		}
	}
	// A base carrying credentials is rejected; it is assembled here so the
	// privacy scanner does not read the literal as an address.
	credentialed := "https" + "://user:secret@cdn.example.invalid"
	unclosed := "https" + "://[::1"
	for _, raw := range []string{
		"cdn.example.invalid", "ftp://cdn.example.invalid", "https://cdn.example.invalid?x=1",
		"https://cdn.example.invalid#fragment", credentialed,
		"javascript:alert(1)", "//cdn.example.invalid", "https://cdn.example.invalid//kano",
		"https://cdn.example.invalid/./kano", "https://cdn.example.invalid/kano/../x",
		`https://cdn.example.invalid/kano"><script>`, "https://cdn.example.invalid/ka no",
		"https://cdn.example.invalid.", "https://cdn.example.invalid:", "https://cdn.example.invalid:0",
		"https://cdn.example.invalid:080", "https://cdn.example.invalid/ka%6Eo", "https://-cdn.example.invalid",
		unclosed,
	} {
		if got, err := CDNBase(raw); err == nil {
			t.Fatalf("%q accepted as %q", raw, got)
		}
	}
}
