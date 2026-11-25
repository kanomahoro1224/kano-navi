package contentadmin

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The Review queue's schedule work groups by week what the calendar marks,
// and the overview counts the same items.
func TestScheduleAttentionGroupsWhatNeedsALookByWeek(t *testing.T) {
	f := newFixture(t, true)
	// Wednesday 2026-03-11 in Japan.
	f.module.now = func() time.Time { return time.Date(2026, 3, 11, 3, 0, 0, 0, time.UTC) }
	const at = "2026-03-01T00:00:00.000Z"
	f.exec(`INSERT INTO events(id,source,title,starts_on,status,provenance,manual_locked,confidence,cancellation_status,merged_into,deleted_at,created_at,updated_at) VALUES
	 ('low','x','Low confidence','2026-03-12','scheduled','automatic',0,0.4,'none',NULL,NULL,?1,?1),
	 ('tentative','x','Tentative','2026-03-17','tentative','automatic',0,0.9,'none',NULL,NULL,?1,?1),
	 ('suspected','x','Suspected','2026-03-02','scheduled','automatic',1,0.9,'llm_suspected',NULL,NULL,?1,?1),
	 ('sure','x','Sure','2026-03-12','scheduled','automatic',0,0.9,'none',NULL,NULL,?1,?1),
	 ('locked','x','Locked','2026-03-12','tentative','automatic',1,0.4,'none',NULL,NULL,?1,?1),
	 ('past','x','Past','2026-03-10','tentative','automatic',0,0.4,'none',NULL,NULL,?1,?1),
	 ('merged','x','Merged','2026-03-12','tentative','automatic',0,0.4,'llm_suspected','low',NULL,?1,?1),
	 ('deleted','x','Deleted','2026-03-12','tentative','automatic',0,0.4,'llm_suspected',NULL,?1,?1,?1)`, at)
	f.exec(`INSERT INTO assets(id,kind,url,period_start,period_end,period_basis,updated_at) VALUES
	 ('board-week','schedule','https://media.example.test/a.png','2026-03-11','2026-03-15','text',?1),
	 ('board-undated','schedule','https://media.example.test/b.png',NULL,NULL,'legacy',?1),
	 ('board-labelled','schedule','https://media.example.test/c.png','2026-03-09','2026-03-15','text',?1),
	 ('board-no-url','schedule','',NULL,NULL,'legacy',?1);
	 INSERT INTO schedule_asset_reviews(asset_id,llm_status,manual_status,updated_at) VALUES
	 ('board-week','uncertain','unreviewed',?1),
	 ('board-labelled','pending','not_schedule',?1)`, at)

	response := f.request(http.MethodGet, "/schedule-attention", "", nil)
	response.expect(t, http.StatusOK, "")
	if response.json["today"] != "2026-03-11" || response.json["truncated"] != false {
		t.Fatalf("attention = %v", response.json)
	}
	got := []string{}
	for _, item := range list(t, response.json["weeks"]) {
		week := object(t, item)
		line := week["weekStart"].(string) + ":"
		for _, image := range list(t, week["images"]) {
			line += " image " + object(t, image)["id"].(string)
		}
		for _, value := range list(t, week["events"]) {
			event := object(t, value)
			review := object(t, event["review"])
			reasons := []string{}
			for _, reason := range list(t, review["reasons"]) {
				reasons = append(reasons, reason.(string))
			}
			line += " " + event["id"].(string) + "=" + review["kind"].(string) + "[" + strings.Join(reasons, ",") + "]"
		}
		got = append(got, line)
	}
	want := []string{
		"2026-03-02: suspected=suspected[]",
		"2026-03-09: image board-week low=uncertain[lowConfidence]",
		"2026-03-16: tentative=uncertain[tentative]",
	}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Fatalf("weeks = %q, want %q", got, want)
	}
	undated := list(t, response.json["undated"])
	if len(undated) != 1 || object(t, undated[0])["id"] != "board-undated" {
		t.Fatalf("undated = %v", undated)
	}

	counts, err := f.store.AdminCounts(context.Background(), "2026-03-11")
	if err != nil {
		t.Fatal(err)
	}
	if counts.PendingImages != 2 || counts.SuspectedCancellations != 1 || counts.UncertainEvents != 2 {
		t.Fatalf("counts = %+v", counts)
	}
}

// A replaced image under an operator's label and an image whose re-check
// failed wait in the Review queue with what the operator needs to answer; an
// entry whose only post was read again as no schedule waits as unlisted. The
// operator's label settles the image.
func TestScheduleAttentionAsksAgainWhatTheModelCannotSettle(t *testing.T) {
	f := newFixture(t, true)
	f.module.now = func() time.Time { return time.Date(2026, 3, 11, 3, 0, 0, 0, time.UTC) }
	const at = "2026-03-01T00:00:00.000Z"
	f.exec(`INSERT INTO assets(id,kind,url,period_start,period_end,period_basis,updated_at) VALUES
	 ('replaced','schedule','https://media.example.test/new.png','2026-03-09','2026-03-15','text',?1),
	 ('rechecked','schedule','https://media.example.test/same.png','2026-03-09','2026-03-15','text',?1),
	 ('settled','schedule','https://media.example.test/settled.png','2026-03-09','2026-03-15','text',?1);
	 INSERT INTO schedule_asset_reviews(asset_id,llm_status,manual_status,previous_manual_status,previous_manual_reason,previous_manual_checked_at,check_status,check_reason,check_at,updated_at) VALUES
	 ('replaced','schedule','unreviewed','schedule','confirmed board','2026-03-02T00:00:00.000Z',NULL,NULL,NULL,?1),
	 ('rechecked','schedule','unreviewed',NULL,NULL,NULL,'failed','LLM request failed (HTTP 429)','2026-03-10T00:00:00.000Z',?1),
	 ('settled','schedule','unreviewed',NULL,NULL,NULL,NULL,NULL,NULL,?1)`, at)
	f.exec(`INSERT INTO events(id,source,title,starts_on,status,provenance,manual_locked,confidence,created_at,updated_at) VALUES
	 ('unlisted','x','Unlisted','2026-03-12','scheduled','automatic',0,0.9,?1,?1);
	 INSERT INTO event_sources(event_id,source,source_item_id,source_key,unlisted_at,created_at,updated_at) VALUES
	 ('unlisted','x','post-1','2026-03-12T20:00:synthetic',?1,?1,?1)`, at)

	response := f.request(http.MethodGet, "/schedule-attention", "", nil)
	response.expect(t, http.StatusOK, "")
	weeks := list(t, response.json["weeks"])
	if len(weeks) != 1 {
		t.Fatalf("weeks = %v", weeks)
	}
	week := object(t, weeks[0])
	images := map[string]map[string]any{}
	for _, value := range list(t, week["images"]) {
		image := object(t, value)
		images[image["id"].(string)] = image
	}
	if len(images) != 2 || images["settled"] != nil {
		t.Fatalf("images = %v", images)
	}
	replaced := images["replaced"]
	if previous := object(t, replaced["previousDecision"]); previous["status"] != "schedule" || previous["reason"] != "confirmed board" ||
		previous["at"] != "2026-03-02T00:00:00.000Z" || replaced["needsReview"] != true || replaced["approved"] != true {
		t.Fatalf("replaced image = %v", replaced)
	}
	rechecked := images["rechecked"]
	if check := object(t, rechecked["lastCheck"]); check["status"] != "failed" || check["reason"] != "LLM request failed (HTTP 429)" ||
		rechecked["approved"] != true || rechecked["llmStatus"] != "schedule" {
		t.Fatalf("rechecked image = %v", rechecked)
	}
	events := list(t, week["events"])
	if len(events) != 1 || object(t, object(t, events[0])["review"])["kind"] != "unlisted" {
		t.Fatalf("events = %v", events)
	}
	counts, err := f.store.AdminCounts(context.Background(), "2026-03-11")
	if err != nil || counts.PendingImages != 2 || counts.UnlistedEvents != 1 {
		t.Fatalf("counts = %+v, %v", counts, err)
	}

	labelled := f.post("/schedule-images/replaced/review", map[string]any{"status": "schedule", "reason": "same board"})
	labelled.expect(t, http.StatusOK, "")
	if image := object(t, labelled.json["image"]); image["previousDecision"] != nil || image["needsReview"] != false || image["lastCheck"] != nil {
		t.Fatalf("labelled image = %v", image)
	}
}
