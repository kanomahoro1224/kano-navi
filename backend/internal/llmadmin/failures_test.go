package llmadmin

import (
	"net/http"
	"testing"
)

func TestRecentFailuresListsStoredModelFailuresNewestFirst(t *testing.T) {
	h := newHarness(t, harnessOptions{})
	equal(t, "empty", len(h.ok(http.MethodGet, "/llm/failures", nil, http.StatusOK).JSON["items"].([]any)), 0)

	postURL := "https://x.example.test/creator/status/1001"
	h.exec(`INSERT INTO posts (id, source, text, published_at, url) VALUES ('1001', 'x', 'synthetic', '2026-01-01T00:00:00Z', ?)`, postURL)
	extraction := `INSERT INTO schedule_extractions (source, source_item_id, content_fingerprint, extractor_version, model, status, error, created_at, updated_at)
		VALUES ('x', '1001', ?, ?, 'model-a', ?, ?, ?, ?)`
	version := "openai-schedule-v5:schedule_message:text:provider-a:anthropic-messages:0123abcd"
	h.exec(extraction, "fp-1", version, "failed", "schedule extraction contained an invalid event", "2026-01-02T05:00:00Z", "2026-01-02T05:00:00Z")
	// A stopped run and a success are not failures.
	h.exec(extraction, "fp-2", version, "failed", "LLM request cancelled", "2026-01-02T07:00:00Z", "2026-01-02T07:00:00Z")
	h.exec(extraction, "fp-3", version, "success", nil, "2026-01-02T08:00:00Z", "2026-01-02T08:00:00Z")

	h.exec(`INSERT INTO assets (id, kind, url, source_url) VALUES ('asset-1', 'schedule', '/media/asset-1', ?)`, postURL)
	h.exec(`INSERT INTO schedule_asset_reviews (asset_id, llm_status, llm_reason, llm_model, llm_checked_at, updated_at)
		VALUES ('asset-1', 'failed', 'all configured LLM providers failed', 'model-b', '2026-01-02T04:00:00Z', '2026-01-02T04:00:00Z')`)

	translation := `INSERT INTO translation_states (owner_type, owner_id, field, status, last_attempt_at, last_error, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	h.exec(translation, "post", "1001", "text", "failed", "2026-01-02T06:00:00+09:00", "LLM output was truncated", "2026-01-02T06:00:00+09:00")
	h.exec(translation, "video", "video-1", "title", "success", "2026-01-02T09:00:00Z", nil, "2026-01-02T09:00:00Z")

	items := h.ok(http.MethodGet, "/llm/failures", nil, http.StatusOK).JSON["items"].([]any)
	equal(t, "count", len(items), 3)
	// The translation's +09:00 attempt is 21:00 UTC the day before.
	want := []map[string]any{
		{"route": "schedule", "subject": "post", "subjectId": "1001", "url": postURL, "providerId": "provider-a", "model": "model-a",
			"message": "schedule extraction contained an invalid event", "at": "2026-01-02T05:00:00Z"},
		{"route": "verify", "subject": "image", "subjectId": "asset-1", "url": postURL, "providerId": nil, "model": "model-b",
			"message": "all configured LLM providers failed", "at": "2026-01-02T04:00:00Z"},
		{"route": "translate", "subject": "post", "subjectId": "1001", "url": postURL, "providerId": nil, "model": nil,
			"message": "LLM output was truncated", "at": "2026-01-02T06:00:00+09:00"},
	}
	for i, item := range items {
		equal(t, "item", object(t, item), want[i])
	}
}

func TestExtractionProviderNeedsTheWholeVersion(t *testing.T) {
	for version, want := range map[string]any{
		"openai-schedule-v5:schedule_board:mixed:openai:openai-responses:abc": "openai",
		"openai-schedule-v5:schedule_board:mixed:":                            nil,
		"": nil,
	} {
		got := extractionProvider(version)
		if want == nil {
			equal(t, version, got == nil, true)
			continue
		}
		equal(t, version, got != nil && *got == want, true)
	}
}
