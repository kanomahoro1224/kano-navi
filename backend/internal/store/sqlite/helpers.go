package sqlite

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/yexca/kano-navi/backend/internal/urlguard"
)

type scanner interface{ Scan(...any) error }

func dbErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: query failed", ErrUnavailable)
}

func strPtr(v string) *string { return &v }

func firstString(values ...*string) *string {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func safeHash(v string) bool {
	if len(v) != 64 {
		return false
	}
	if strings.ToLower(v) != v {
		return false
	}
	for _, r := range v {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func sha256Hex(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }

func normalizeSourceURL(v string) string {
	u, err := url.Parse(strings.TrimSpace(v))
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" || urlguard.ContainsCredentials(u.String()) {
		return ""
	}
	u.Fragment = ""
	return u.String()
}

func xHandleFromURL(v string) string {
	u, err := url.Parse(v)
	if err != nil || u.Host != "x.com" && u.Host != "www.x.com" && u.Host != "twitter.com" && u.Host != "www.twitter.com" {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	h := parts[0]
	switch strings.ToLower(h) {
	case "i", "intent", "share", "search", "hashtag":
		return ""
	}
	if len(h) < 1 || len(h) > 30 {
		return ""
	}
	for _, r := range h {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return ""
		}
	}
	return h
}

func dateInJapan(t time.Time) string {
	return t.In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02")
}

func mondayWindow(day string) (string, string) {
	t, _ := time.Parse("2006-01-02", day)
	delta := (int(t.Weekday()) + 6) % 7
	t = t.AddDate(0, 0, -delta)
	return t.Format("2006-01-02"), t.AddDate(0, 0, 6).Format("2006-01-02")
}
