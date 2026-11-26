package schedule

import (
	"net/http"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/digest"
	"github.com/yexca/kano-navi/backend/internal/jsvalue"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// A check that cannot run or fails leaves the verdict, and with it the
// image's public visibility, in place, records the check apart, and puts
// the image in the Review queue until a check settles it again.
func TestScheduleImageRecheckKeepsTheVerdict(t *testing.T) {
	f, _ := boardFixture(t)
	f.scan()
	const asset = "schedule-2026-04-27"
	verdict := `SELECT llm_status || '|' || llm_confidence || '|' || llm_text || '|' || llm_model || '|' || llm_checked_at FROM schedule_asset_reviews WHERE asset_id=?`
	check := `SELECT COALESCE(check_status, '-') || '|' || COALESCE(check_reason, '-') || '|' || (check_at IS NOT NULL) FROM schedule_asset_reviews WHERE asset_id=?`
	pending := func() int {
		t.Helper()
		return f.number(`SELECT COUNT(*) FROM assets a LEFT JOIN schedule_asset_reviews r ON r.asset_id=a.id WHERE a.id=? AND `+sqlite.ScheduleImagePendingSQL("a", "r"), asset)
	}
	approved := f.text(verdict, asset)
	if f.text(`SELECT llm_status FROM schedule_asset_reviews WHERE asset_id=?`, asset) != "schedule" || pending() != 0 {
		t.Fatalf("setup verdict = %s", approved)
	}
	kept := func(label, want string) {
		t.Helper()
		if got := f.text(verdict, asset); got != approved {
			t.Fatalf("%s replaced the verdict: %s", label, got)
		}
		if got := f.text(check, asset); got != want {
			t.Fatalf("%s check = %s, want %s", label, got, want)
		}
		if pending() != 1 {
			t.Fatalf("%s left the image out of the Review queue", label)
		}
	}

	// An input the verdict does not cover (as after an extractor version
	// change) with an empty image queue.
	f.exec(`UPDATE schedule_asset_reviews SET llm_input_fingerprint='older-input' WHERE asset_id=?`, asset)
	f.exec(`DELETE FROM llm_route_targets WHERE route = ?`, routeVerify)
	f.scan()
	kept("an empty queue", "skipped|no_route_target|1")

	// A running check and a failed one.
	f.queueVerify("p-vision")
	running := ""
	f.provider.respond = func(recordedRequest) (int, string) {
		running = f.text(`SELECT llm_status || '|' || check_status FROM schedule_asset_reviews WHERE asset_id=?`, asset)
		return http.StatusTooManyRequests, `{}`
	}
	f.scan()
	if running != "schedule|running" {
		t.Fatalf("a running check = %s", running)
	}
	kept("a failed check", "failed|LLM request failed (HTTP 429)|1")

	// A check that returns a verdict settles the image.
	f.provider.respond = func(recordedRequest) (int, string) {
		return http.StatusOK, jsvalue.Stringify(jsvalue.Obj("output_text", `{"classification":"schedule","text":"火曜 21:00 配信","confidence":0.8,"evidence":"grid","reason":"table"}`))
	}
	f.scan()
	if got := f.text(`SELECT llm_status || '|' || llm_text FROM schedule_asset_reviews WHERE asset_id=?`, asset); got != "schedule|火曜 21:00 配信" {
		t.Fatalf("new verdict = %s", got)
	}
	if got := f.text(check, asset); got != "-|-|0" || pending() != 0 {
		t.Fatalf("a settled check stays: %s", got)
	}
	approved = f.text(verdict, asset)

	// A cache that is not ready yet, then ready again with the same image.
	mediaID := mediaIDForSourceURL(f.text(`SELECT url FROM assets WHERE id=?`, asset))
	f.exec(`UPDATE media_assets SET status='pending' WHERE id=?`, mediaID)
	f.scan()
	kept("a pending cache", "skipped|media_pending|1")
	f.exec(`UPDATE media_assets SET status='ready' WHERE id=?`, mediaID)
	requests := f.provider.count()
	f.scan()
	if f.provider.count() != requests || f.text(check, asset) != "-|-|0" || pending() != 0 {
		t.Fatalf("the same image did not settle from its verdict: %s after %d requests", f.text(check, asset), f.provider.count()-requests)
	}
}

// Two assets share one image. The one checked last comes from a post that
// fails the source-text gate, so it is only ever skipped; the image is still
// verified through the sibling whose post reads as a board notice.
func TestSharedScheduleImageIsVerifiedThroughAPostThatPassesTheGate(t *testing.T) {
	f, imageURL := boardFixture(t)
	streamPost := f.addPost("stream-post", "", "今日は22時から配信します", "2026-04-27T03:00:00.000Z", "")
	f.exec(`INSERT INTO assets (id, kind, url, source_url, week_start) VALUES ('schedule-2026-04-20', 'schedule', ?, ?, '2026-04-20')`, imageURL, streamPost)
	f.exec(`INSERT INTO schedule_asset_reviews (asset_id, llm_status, llm_reason, manual_status, llm_checked_at, updated_at) VALUES ('schedule-2026-04-20', 'skipped', 'source_not_board', 'unreviewed', ?, ?)`, testTimestamp, testTimestamp)
	f.scan()
	if f.provider.count() != 1 {
		t.Fatalf("provider requests = %d, want 1", f.provider.count())
	}
	if got := f.text(`SELECT llm_status FROM schedule_asset_reviews WHERE asset_id='schedule-2026-04-27'`); got != "schedule" {
		t.Fatalf("board asset = %q", got)
	}
}

// A verdict belongs to the image it judged. When the cache under the same URL
// holds a different image, a check of it that fails or cannot run never
// lets the old approval publish the new image.
func TestScheduleImageVerdictDoesNotStandForAReplacedImage(t *testing.T) {
	f, imageURL := boardFixture(t)
	f.scan()
	const asset = "schedule-2026-04-27"
	status := `SELECT llm_status || '|' || COALESCE(check_status, '-') FROM schedule_asset_reviews WHERE asset_id=?`
	if got := f.text(status, asset); got != "schedule|-" {
		t.Fatalf("setup = %s", got)
	}
	// The same URL now serves other bytes.
	mediaID := mediaIDForSourceURL(imageURL)
	f.exec(`DELETE FROM media_assets WHERE id=?`, mediaID)
	f.addCachedImage(imageURL, []byte(pngHeader+"replaced"))

	f.provider.respond = func(recordedRequest) (int, string) { return http.StatusTooManyRequests, `{}` }
	f.scan()
	if got := f.text(status, asset); got != "failed|-" {
		t.Fatalf("a failed check of the replaced image = %s, want failed|-", got)
	}

	// Approved again, then the replacement is still downloading: the cache
	// already names the next image, so the approval does not stand for it.
	f.provider.respond = func(recordedRequest) (int, string) {
		return http.StatusOK, jsvalue.Stringify(jsvalue.Obj("output_text", `{"classification":"schedule","text":"火曜 21:00 配信","confidence":0.8,"evidence":"grid","reason":"table"}`))
	}
	f.scan()
	if got := f.text(status, asset); got != "schedule|-" {
		t.Fatalf("re-approval = %s", got)
	}
	f.exec(`UPDATE media_assets SET status='pending', sha256=? WHERE id=?`, digest.SHA256Hex(pngHeader+"next"), mediaID)
	f.scan()
	if got := f.text(status, asset); got != "skipped|-" {
		t.Fatalf("a pending download of another image = %s, want skipped|-", got)
	}
}

// A verdict stored before the image hash was recorded learns it from a cache
// hit on its own input, so a later failed check of that image keeps it.
func TestScheduleImageCacheHitRecordsTheJudgedImage(t *testing.T) {
	f, imageURL := boardFixture(t)
	f.scan()
	const asset = "schedule-2026-04-27"
	f.exec(`UPDATE schedule_asset_reviews SET llm_image_sha256=NULL WHERE asset_id=?`, asset)
	requests := f.provider.count()
	f.scan()
	hash := f.text(`SELECT sha256 FROM media_assets WHERE id=?`, mediaIDForSourceURL(imageURL))
	if f.provider.count() != requests || f.text(`SELECT COALESCE(llm_image_sha256, '-') FROM schedule_asset_reviews WHERE asset_id=?`, asset) != hash {
		t.Fatalf("the cache hit did not record the judged image")
	}
	f.exec(`UPDATE schedule_asset_reviews SET llm_input_fingerprint='older-input' WHERE asset_id=?`, asset)
	f.provider.respond = func(recordedRequest) (int, string) { return http.StatusTooManyRequests, `{}` }
	f.scan()
	if got := f.text(`SELECT llm_status || '|' || COALESCE(check_status, '-') FROM schedule_asset_reviews WHERE asset_id=?`, asset); got != "schedule|failed" {
		t.Fatalf("a failed check of the judged image = %s, want schedule|failed", got)
	}
}
