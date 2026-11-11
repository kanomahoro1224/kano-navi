package adminapi

import (
	"net/http"
	"testing"
)

func TestRelatedAccountsAndExternalPostDecisions(t *testing.T) {
	c := newConsole(t, "development", "", &gatedSteps{})
	code, payload := c.call(t, http.MethodGet, "/api/admin/sources/x/related", "")
	if code != http.StatusOK || len(payload["accounts"].([]any)) != 0 || len(payload["defaultKeywords"].([]any)) != 3 {
		t.Fatalf("empty=%d %v", code, payload)
	}
	// An own account cannot be a related one.
	if code, payload = c.call(t, http.MethodPut, "/api/admin/sources/x/related", `{"accounts":[{"handle":"Synthetic_One","keywords":[]}]}`); code != http.StatusBadRequest {
		t.Fatalf("own handle=%d %v", code, payload)
	}
	code, payload = c.call(t, http.MethodPut, "/api/admin/sources/x/related", `{"accounts":[{"handle":"@agency_synthetic","keywords":[" 鹿乃 ","鹿乃",""]}]}`)
	accounts, _ := payload["accounts"].([]any)
	if code != http.StatusOK || len(accounts) != 1 {
		t.Fatalf("saved=%d %v", code, payload)
	}
	if first := accounts[0].(map[string]any); first["handle"] != "agency_synthetic" || len(first["keywords"].([]any)) != 1 {
		t.Fatalf("account=%v", first)
	}
	c.exec(t, `INSERT INTO x_statuses(id,author_handle,text,published_at,url,raw_json,listed_at,first_seen_at,updated_at)
		VALUES('1900000000000000401','agency_synthetic','news','2026-03-01T00:00:00.000Z','https://x.com/agency_synthetic/status/1900000000000000401',
		'{"id":"1900000000000000401","text":"news","created_at":"2026-03-01T00:00:00Z","author":{"screen_name":"agency_synthetic","name":"Agency"},"mediaURLs":[]}',
		'2026-03-01T00:00:00.000Z','2026-03-01T00:00:00.000Z','2026-03-01T00:00:00.000Z')`)
	code, payload = c.call(t, http.MethodGet, "/api/admin/x-statuses?state=stored&author=agency_synthetic", "")
	if code != http.StatusOK || payload["total"] != float64(1) {
		t.Fatalf("list=%d %v", code, payload)
	}
	if code, payload = c.call(t, http.MethodPut, "/api/admin/x-statuses/1900000000000000401/decision", `{"decision":"promote"}`); code != http.StatusBadRequest {
		t.Fatalf("no reason=%d %v", code, payload)
	}
	code, payload = c.call(t, http.MethodPut, "/api/admin/x-statuses/1900000000000000401/decision", `{"decision":"promote","reason":"mentions Kano in the image"}`)
	status, _ := payload["status"].(map[string]any)
	if code != http.StatusOK || status["state"] != "promoted" || status["decision"] != "promote" {
		t.Fatalf("promote=%d %v", code, payload)
	}
	if got := c.text(t, `SELECT account_handle FROM posts WHERE id='1900000000000000401' AND promoted_at IS NOT NULL`); got != "agency_synthetic" {
		t.Fatalf("post=%s", got)
	}
	if code, _ = c.call(t, http.MethodPut, "/api/admin/x-statuses/1900000000000000999/decision", `{"decision":"demote","reason":"x"}`); code != http.StatusNotFound {
		t.Fatalf("missing=%d", code)
	}
}
