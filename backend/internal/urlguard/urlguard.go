// Package urlguard contains the checks shared by public links that are
// displayed or exported by the board.
package urlguard

import (
	"net/url"
	"strings"
)

// ContainsCredentials reports whether raw carries URL credentials. Userinfo
// and common credential-bearing query parameters are rejected before a link
// crosses into a public projection.
func ContainsCredentials(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil {
		return true
	}
	for key := range parsed.Query() {
		key = strings.ToLower(strings.TrimSpace(key))
		if sensitiveQueryKeys[key] || strings.HasSuffix(key, "_token") || strings.HasSuffix(key, "_secret") || strings.HasSuffix(key, "_password") {
			return true
		}
	}
	return false
}

var sensitiveQueryKeys = map[string]bool{
	"access_token":  true,
	"api_key":       true,
	"apikey":        true,
	"auth":          true,
	"authorization": true,
	"client_secret": true,
	"cookie":        true,
	"password":      true,
	"passwd":        true,
	"secret":        true,
	"session":       true,
	"sig":           true,
	"signature":     true,
	"token":         true,
}
