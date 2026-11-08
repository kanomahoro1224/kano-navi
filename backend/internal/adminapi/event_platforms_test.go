package adminapi

import (
	"fmt"
	"testing"
)

// An operator chooses an entry's platforms; a live-room link is never its
// link unless the entry is in someone else's room (other).
func TestManualEventPlatformsAndRoomLinks(t *testing.T) {
	c := newConsole(t, "development", "", &gatedSteps{})
	platforms := func(event map[string]any) string { return fmt.Sprint(event["platforms"]) }
	code, payload := c.call(t, "POST", "/api/admin/events", `{"title":"同時配信","startsOn":"2026-03-02","eventType":"stream","platforms":["bilibili","youtube"]}`)
	if code != 201 {
		t.Fatalf("create=%d %v", code, payload)
	}
	event := payload["event"].(map[string]any)
	id := event["id"].(string)
	// The operator's order is kept: the first is the entry's own platform.
	if platforms(event) != "[bilibili youtube]" || fmt.Sprint(event["shownPlatforms"]) != "[bilibili youtube]" || event["platformsInferred"] != false ||
		c.text(t, `SELECT platform||' '||platforms FROM events WHERE id=?`, id) != `bilibili ["bilibili","youtube"]` {
		t.Fatalf("created=%v", event)
	}
	// A save without platforms keeps them; null clears them.
	_, payload = c.call(t, "PUT", "/api/admin/events/"+id, `{"title":"同時配信","startsOn":"2026-03-02","eventType":"stream"}`)
	if platforms(payload["event"].(map[string]any)) != "[bilibili youtube]" {
		t.Fatalf("kept=%v", payload)
	}
	_, payload = c.call(t, "PUT", "/api/admin/events/"+id, `{"title":"同時配信","startsOn":"2026-03-02","eventType":"stream","platforms":null}`)
	if platforms(payload["event"].(map[string]any)) != "[]" || c.text(t, `SELECT platform FROM events WHERE id=?`, id) != "unknown" {
		t.Fatalf("cleared=%v", payload)
	}
	for body, message := range map[string]string{
		`{"title":"x","startsOn":"2026-03-02","platforms":["twitch"]}`: "platforms is invalid",
		`{"title":"x","startsOn":"2026-03-02","platforms":"youtube"}`:  "platforms must be an array",
	} {
		if code, payload = c.call(t, "POST", "/api/admin/events", body); code != 400 || payload["message"] != message {
			t.Fatalf("%s => %d %v", body, code, payload)
		}
	}
	room := `"url":"https://live.bilibili.com/1000001"`
	for _, body := range []string{
		`{"title":"雑談","startsOn":"2026-03-02","eventType":"stream",` + room + `}`,
		`{"title":"雑談","startsOn":"2026-03-02","eventType":"stream","platforms":["bilibili"],` + room + `}`,
	} {
		if code, payload = c.call(t, "POST", "/api/admin/events", body); code != 400 || payload["error"] != "event_live_room_url" {
			t.Fatalf("%s => %d %v", body, code, payload)
		}
	}
	if code, payload = c.call(t, "PUT", "/api/admin/events/"+id, `{"title":"x","startsOn":"2026-03-02",`+room+`}`); code != 400 || payload["error"] != "event_live_room_url" {
		t.Fatalf("update with a room link=%d %v", code, payload)
	}
	if code, payload = c.call(t, "POST", "/api/admin/events", `{"title":"コラボ","startsOn":"2026-03-02","eventType":"stream","platforms":["other"],`+room+`}`); code != 201 {
		t.Fatalf("collaboration=%d %v", code, payload)
	}
	// A stored manual entry whose link is the room (written before the
	// rule) reads as a Bilibili entry, and confirming it drops that link.
	c.exec(t, `INSERT INTO events(id,source,source_item_id,source_key,title,starts_on,event_type,url,provenance,manual_locked,created_at,updated_at)
 VALUES('manual-legacy','manual','manual-legacy','manual-legacy','Bilibili 限定雑談','2026-03-02','stream','https://live.bilibili.com/1000001','manual',1,'2026-03-01T00:00:00.000Z','2026-03-01T00:00:00.000Z')`)
	code, payload = c.call(t, "POST", "/api/admin/events/manual-legacy/confirm", `{}`)
	if code != 200 {
		t.Fatalf("confirm legacy=%d %v", code, payload)
	}
	if confirmed := payload["event"].(map[string]any); confirmed["url"] != nil || platforms(confirmed) != "[bilibili]" {
		t.Fatalf("confirmed=%v", confirmed)
	}
}

// Platforms are part of the manual lock: extraction never rewrites a
// confirmed entry's platforms.
func TestConfirmingKeepsTheAnnouncedPlatforms(t *testing.T) {
	c := newConsole(t, "development", "", &gatedSteps{})
	c.exec(t, `INSERT INTO events(id,source,source_item_id,source_key,title,starts_on,event_type,provenance,manual_locked,platform,platforms,platforms_inferred,created_at,updated_at)
 VALUES('bilibili-schedule-a','bilibili','bilibili-1','k','杂谈','2026-03-02','stream','automatic',0,'bilibili','["bilibili"]',1,'2026-03-01T00:00:00.000Z','2026-03-01T00:00:00.000Z')`)
	code, payload := c.call(t, "POST", "/api/admin/events/bilibili-schedule-a/confirm", `{}`)
	if code != 200 {
		t.Fatalf("confirm=%d %v", code, payload)
	}
	if event := payload["event"].(map[string]any); fmt.Sprint(event["platforms"]) != "[bilibili]" || event["platformsInferred"] != false || event["manualLocked"] != true {
		t.Fatalf("confirmed=%v", event)
	}
}
