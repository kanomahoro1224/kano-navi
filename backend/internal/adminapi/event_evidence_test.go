package adminapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func evidenceIDs(t *testing.T, payload map[string]any, key string) []string {
	t.Helper()
	items, ok := payload[key].([]any)
	if !ok {
		t.Fatalf("%s missing: %v", key, payload)
	}
	ids := []string{}
	for _, item := range items {
		ids = append(ids, item.(map[string]any)["id"].(string))
	}
	return ids
}

func TestEventEvidenceListsSourcePostsAndTheirAccountsNearbyPosts(t *testing.T) {
	c := newConsole(t, "development", "", &gatedSteps{})
	c.exec(t, `INSERT INTO posts(id,source,account_handle,text,published_at,url,raw_json,promoted_at) VALUES
	 ('ev-src','x','synthetic_one','announcement','2026-03-01T03:00:00.000Z','https://post.example.test/1','{"secret":"must-not-leak"}',NULL),
	 ('ev-merged','x','synthetic_one','reminder','2026-03-01T05:00:00.000Z','https://post.example.test/2',NULL,NULL),
	 ('ev-cancel','x','synthetic_one','called off','2026-03-02T01:00:00.000Z','https://post.example.test/3',NULL,NULL),
	 ('near-before','x','synthetic_one','context before','2026-02-28T10:00:00.000Z','https://post.example.test/4',NULL,NULL),
	 ('far-before','x','synthetic_one','too early','2026-02-27T00:00:00.000Z','https://post.example.test/5',NULL,NULL),
	 ('near-after','x','synthetic_one','context after','2026-03-02T20:00:00.000Z','https://post.example.test/6',NULL,NULL),
	 ('other-account','x','synthetic_two','someone else','2026-03-01T04:00:00.000Z','https://post.example.test/7',NULL,NULL),
	 ('lone-near','x','synthetic_one','lead-up','2026-03-04T00:00:00.000Z','https://post.example.test/8',NULL,NULL),
	 ('promoted','x','synthetic_two','promoted','2026-03-04T01:00:00.000Z','https://post.example.test/9',NULL,'2026-03-04T02:00:00.000Z');
	 INSERT INTO events(id,source,title,starts_on,starts_at,time_precision,event_type,provenance,manual_locked,cancellation_status,cancellation_source_item_id,merged_into,created_at,updated_at) VALUES
	 ('ev-main','x','Synthetic stream','2026-03-03','2026-03-03T11:00:00.000Z','exact','stream','automatic',0,'llm_suspected','ev-cancel',NULL,'2026-03-01T00:00:00.000Z','2026-03-01T00:00:00.000Z'),
	 ('ev-dupe','x','Synthetic stream','2026-03-03',NULL,'unknown','stream','automatic',0,'none',NULL,'ev-main','2026-03-01T00:00:00.000Z','2026-03-01T00:00:00.000Z'),
	 ('ev-lone','manual','Synthetic talk','2026-03-05','2026-03-05T11:00:00.000Z','exact','stream','manual',1,'none',NULL,NULL,'2026-03-01T00:00:00.000Z','2026-03-01T00:00:00.000Z');
	 INSERT INTO event_sources(event_id,source,source_item_id,source_key,url,created_at,updated_at) VALUES
	 ('ev-main','x','ev-src','k1','https://post.example.test/1','2026-03-01T00:00:00.000Z','2026-03-01T00:00:00.000Z'),
	 ('ev-main','youtube','synthetic-video','k2','https://video.example.test/watch','2026-03-01T00:00:00.000Z','2026-03-01T00:00:00.000Z'),
	 ('ev-dupe','x','ev-merged','k3','https://post.example.test/2','2026-03-01T00:00:00.000Z','2026-03-01T00:00:00.000Z')`)

	code, payload := c.call(t, "GET", "/api/admin/events/ev-main/evidence", "")
	if code != 200 {
		t.Fatalf("code=%d payload=%v", code, payload)
	}
	if got := strings.Join(evidenceIDs(t, payload, "posts"), ","); got != "ev-src,ev-merged,ev-cancel" {
		t.Fatalf("posts = %s", got)
	}
	roles := []string{}
	for _, post := range payload["posts"].([]any) {
		for _, role := range post.(map[string]any)["roles"].([]any) {
			roles = append(roles, role.(string))
		}
	}
	if got := strings.Join(roles, ","); got != "source,merged,cancellation" {
		t.Fatalf("roles = %s", got)
	}
	links := payload["links"].([]any)
	if len(links) != 1 || links[0].(map[string]any)["source"] != "youtube" || links[0].(map[string]any)["url"] != "https://video.example.test/watch" {
		t.Fatalf("links = %v", links)
	}
	if got := strings.Join(evidenceIDs(t, payload, "nearby"), ","); got != "near-before,near-after" {
		t.Fatalf("nearby = %s", got)
	}
	if body, _ := json.Marshal(payload); strings.Contains(string(body), "must-not-leak") || strings.Contains(string(body), "raw_json") {
		t.Fatalf("evidence leaks raw source data: %s", body)
	}

	// An entry without a stored post reads its own accounts' lead-up posts.
	code, payload = c.call(t, "GET", "/api/admin/events/ev-lone/evidence", "")
	if code != 200 || len(payload["posts"].([]any)) != 0 {
		t.Fatalf("code=%d payload=%v", code, payload)
	}
	if got := strings.Join(evidenceIDs(t, payload, "nearby"), ","); got != "near-after,lone-near" {
		t.Fatalf("lead-up nearby = %s", got)
	}

	if code, payload = c.call(t, "GET", "/api/admin/events/unknown/evidence", ""); code != 404 || payload["error"] != "event_not_found" {
		t.Fatalf("unknown: code=%d payload=%v", code, payload)
	}
}
