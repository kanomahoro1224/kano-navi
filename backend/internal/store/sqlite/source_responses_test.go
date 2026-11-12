package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/sourcecapture"
)

func TestSourceResponseCaptureRoundTripAndIndependentRetention(t *testing.T) {
	store, _ := fixture(t)
	defer store.Close()
	ctx := context.Background()
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	body := []byte(strings.Repeat("x", 4096))
	entries := []sourcecapture.Response{
		{Source: "x", Endpoint: "x://api.x.com/2/tweets", Method: "GET", StatusCode: 200, ContentType: "application/json", CapturedAt: now.AddDate(0, 0, -31), Body: body, Execution: sourcecapture.Execution{RunID: "expired-run", StepID: "x", Attempt: 1}},
		{Source: "bilibili", Endpoint: "bilibili://api.bilibili.com/x/web-interface/view", Method: "GET", StatusCode: 503, ContentType: "application/json", CapturedAt: now.AddDate(0, 0, -31), Body: body},
		{Source: "shop", Endpoint: "shop://shop.milpr.com/products.json", Method: "GET", StatusCode: 429, ContentType: "text/html", CapturedAt: now.AddDate(0, 0, -91), Body: body},
		{Source: "youtube", Endpoint: "youtube://www.youtube.com/feeds/videos.xml", Method: "GET", StatusCode: 200, ContentType: "application/atom+xml", CapturedAt: now.Add(-time.Hour), Body: body},
	}
	for _, entry := range entries {
		if err := store.RecordSourceResponse(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := store.ListSourceResponseCaptures(ctx, "", 20)
	if err != nil || len(listed) != 4 {
		t.Fatalf("captures=%d err=%v", len(listed), err)
	}
	if listed[0].BodyBytes != int64(len(body)) || listed[0].StoredBytes >= listed[0].BodyBytes {
		t.Fatalf("capture metadata=%+v", listed[0])
	}
	loaded, err := store.ReadSourceResponseCapture(ctx, listed[0].ID)
	if err != nil || string(loaded.Body) != string(body) || loaded.SHA256 == "" {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
	deleted, err := store.PruneSourceResponseCaptures(ctx, now)
	if err != nil || deleted != 2 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	listed, err = store.ListSourceResponseCaptures(ctx, "", 20)
	if err != nil || len(listed) != 2 {
		t.Fatalf("remaining=%d err=%v", len(listed), err)
	}
	if listed[0].Source != "youtube" || listed[1].Source != "bilibili" {
		t.Fatalf("remaining captures=%+v", listed)
	}
}
