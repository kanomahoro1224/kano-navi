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
