package adminapi

import "testing"

func TestOnlyASaleTakesACloseDate(t *testing.T) {
	c := newConsole(t, "development", "", &gatedSteps{})
	for body, message := range map[string]string{
		`{"startsOn":"2026-03-02","title":"x","endsOn":"2026-03-09"}`:                                                        "endsOn is only for a sale",
		`{"startsOn":"2026-03-02","title":"x","eventType":"sale","endsOn":"2026-03-01"}`:                                     "endsOn is invalid",
		`{"startsOn":"2026-03-02","title":"x","eventType":"sale","endsOn":"2026-02-30"}`:                                     "endsOn is invalid",
		`{"startsOn":"2026-03-02","title":"x","eventType":"sale","endsOn":"2026-03-09","endsAt":"2026-03-10T00:00:00.000Z"}`: "endsAt must fall on endsOn in Asia/Tokyo",
		// A close time weeks after the opening, without its close date.
		`{"startsOn":"2026-03-02","title":"x","eventType":"sale","startsAt":"2026-03-02T03:00:00.000Z","endsAt":"2026-03-30T14:59:00.000Z"}`: "endsAt requires endsOn for a sale",
	} {
		if code, payload := c.call(t, "POST", "/api/admin/events", body); code != 400 || payload["message"] != message {
			t.Fatalf("%s => %d %v", body, code, payload)
		}
	}
	// A period that opens at noon and closes at 23:59 on a later date.
	code, payload := c.call(t, "POST", "/api/admin/events", `{"title":"Synthetic goods","startsOn":"2026-03-02","startsAt":"2026-03-02T03:00:00.000Z","endsOn":"2026-03-09","endsAt":"2026-03-09T14:59:00.000Z","eventType":"sale"}`)
	if code != 201 {
		t.Fatalf("create=%d %v", code, payload)
	}
	event := payload["event"].(map[string]any)
	if event["eventType"] != "sale" || event["endsOn"] != "2026-03-09" || event["endsAt"] != "2026-03-09T14:59:00.000Z" {
		t.Fatalf("created=%v", event)
	}
	// A deadline whose opening is not known needs no start time.
	code, payload = c.call(t, "POST", "/api/admin/events", `{"title":"Synthetic deadline","startsOn":"2026-03-09","endsOn":"2026-03-09","endsAt":"2026-03-09T14:59:00.000Z","eventType":"sale"}`)
	if code != 201 || payload["event"].(map[string]any)["startsAt"] != nil {
		t.Fatalf("deadline=%d %v", code, payload)
	}
	// Changing the type away from a sale drops the close date.
	id := event["id"].(string)
	code, payload = c.call(t, "PUT", "/api/admin/events/"+id, `{"title":"Synthetic goods","startsOn":"2026-03-02","eventType":"event"}`)
	if edited := payload["event"].(map[string]any); code != 200 || edited["endsOn"] != nil {
		t.Fatalf("edit=%d %v", code, edited)
	}
}
