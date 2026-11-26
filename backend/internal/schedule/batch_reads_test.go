package schedule

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/digest"
	"github.com/yexca/kano-navi/backend/internal/jsvalue"
)

// smallBatches makes runs read two unsettled posts at a time.
func smallBatches(t *testing.T) {
	t.Helper()
	previous := runBatch
	runBatch = 2
	t.Cleanup(func() { runBatch = previous })
}

// Batches smaller than the queue judge and write what reading each post's
// input one row at a time does, across new, changed, failing and requested
// posts.
func TestSmallBatchesMatchPerPostReads(t *testing.T) {
	smallBatches(t)
	tw := newTwin(t)
	tw.scan("first")
	tw.scan("second")
	tw.exec(`UPDATE posts SET text = '明日 22時から配信に変更' WHERE id = 'text-1'`)
	tw.exec(`UPDATE media_assets SET sha256 = ?`, digest.SHA256Hex("replaced"))
	tw.failing["text-2"] = true
	tw.exec(`UPDATE posts SET text = '今日 21時から雑談配信' WHERE id = 'text-2'`)
	tw.scan("changed")
	tw.do(func(f *fixture) { f.requestReprocess("quote-1") })
	tw.scan("requested")
	delete(tw.failing, "text-2")
	// The second failure holds text-2 back for six hours.
	tw.scan("held")
	tw.scan("still held")
	tw.scan("retried")
	tw.scan("recorded")
	tw.scan("settled")
	tw.expect("settled", 0, 5)
}

// A model call drops the batch read before it: a request an operator makes
// for a later post while the call waits is honoured in the same run.
func TestRequestDuringModelCallReachesLaterPost(t *testing.T) {
	f := messageFixture(t)
	f.run.ignoreSettled = true
	f.at(backoffStart)
	f.scan()
	f.at(backoffStart.Add(time.Hour))
	f.scan()
	f.addPost("post-2", "", "今日 20時から雑談配信", "2026-04-28T04:00:00.000Z", "")
	reply := chatReply(scheduleReply(songTitle, songDate, songClock, "scheduled"))
	f.provider.respond = func(request recordedRequest) (int, string) {
		if strings.Contains(request.body, "/status/post-2") {
			f.requestReprocess("post-1")
		}
		return http.StatusOK, reply
	}
	f.at(backoffStart.Add(2 * time.Hour))
	before := f.provider.count()
	result := f.scan()
	if got := calls(f, before); len(got) != 2 || !strings.HasSuffix(got[0], "/post-2") || !strings.HasSuffix(got[1], "/post-1") {
		t.Fatalf("calls %v, counts %v", got, result.Counts)
	}
	if flag := f.number(`SELECT reprocess_requested FROM post_llm_states WHERE post_id = 'post-1'`); flag != 0 {
		t.Fatalf("request flag %d after the requested call", flag)
	}
}

// A cache hit keeps a request flag stored after its post was read, so the
// next scan sends the post to a model.
func TestCacheHitKeepsALaterRequest(t *testing.T) {
	f := messageFixture(t)
	f.run.ignoreSettled = true
	f.at(backoffStart)
	f.scan()
	f.at(backoffStart.Add(time.Hour))
	f.scan()
	ctx := context.Background()
	read, err := getPostState(ctx, f.db, "post-1")
	if err != nil || read == nil {
		t.Fatalf("state %v, %v", read, err)
	}
	f.requestReprocess("post-1")
	cached := &extractionRow{
		id:        int64(f.number(`SELECT last_extraction_id FROM post_llm_states WHERE post_id = 'post-1'`)),
		status:    "success",
		updatedAt: "2026-04-28T05:00:00.000Z",
	}
	if _, err = f.run.useCachedExtraction(ctx, post{id: "post-1"}, routeMessage, read, cached); err != nil {
		t.Fatal(err)
	}
	if flag := f.number(`SELECT reprocess_requested FROM post_llm_states WHERE post_id = 'post-1'`); flag != 1 {
		t.Fatalf("request flag %d, want the later request kept", flag)
	}
	if got := f.text(`SELECT last_processed_at FROM post_llm_states WHERE post_id = 'post-1'`); got != cached.updatedAt {
		t.Fatalf("cache hit not written: %s", got)
	}
	f.at(backoffStart.Add(2 * time.Hour))
	before := f.provider.count()
	f.scan()
	if f.provider.count() != before+1 {
		t.Fatalf("requested post made %d calls", f.provider.count()-before)
	}
}

// A skip keeps a request flag stored after its post was read: a request an
// operator makes for a later post of the batch while the run skips an earlier
// one stays for the next scan.
func TestSkipKeepsALaterRequest(t *testing.T) {
	f := newFixture(t)
	f.addProvider("p-vision", protocolChat, "https://provider.example.test/v1", "model-v", `["text","image"]`)
	f.queue("p-vision")
	for i, id := range []string{"post-a", "post-b"} {
		imageURL := "https://pbs.twimg.com/media/synthetic-" + id + ".png"
		f.addPost(id, "", "今週の配信予定です", "2026-04-28T0"+strconv.Itoa(3+i)+":00:00.000Z", jsvalue.Stringify(jsvalue.Obj("media_urls", []any{imageURL})))
		f.exec(`INSERT INTO media_assets (id, source, source_url, status, created_at, updated_at) VALUES (?, 'x', ?, 'pending', ?, ?)`,
			mediaIDForSourceURL(imageURL), imageURL, testTimestamp, testTimestamp)
		f.exec(`INSERT INTO media_links (media_id, owner_type, owner_id, role, position, created_at, updated_at) VALUES (?, 'post', ?, 'post-image', 0, ?, ?)`,
			mediaIDForSourceURL(imageURL), id, testTimestamp, testTimestamp)
	}
	// The first skip the run writes requests the post it has not reached.
	f.exec(`CREATE TRIGGER request_later_post AFTER INSERT ON post_llm_states
		WHEN NEW.status = 'skipped' BEGIN
			INSERT INTO post_llm_states (post_id, status, reprocess_requested, updated_at)
			SELECT id, 'queued', 1, '` + testTimestamp + `' FROM posts
			WHERE id IN ('post-a', 'post-b') AND id NOT IN (SELECT post_id FROM post_llm_states);
		END`)
	result := f.scan()
	if countOf(result.Counts, "skipped") != 2 || f.provider.count() != 0 {
		t.Fatalf("counts %v, %d calls", result.Counts, f.provider.count())
	}
	got := f.text(`SELECT group_concat(status || '|' || reprocess_requested, ',') FROM (
		SELECT status, reprocess_requested FROM post_llm_states ORDER BY reprocess_requested)`)
	if got != "skipped|0,skipped|1" {
		t.Fatalf("states %q, want the later request kept", got)
	}
}
