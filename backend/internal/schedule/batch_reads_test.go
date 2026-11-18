package schedule

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
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
	tw.exec(`UPDATE media_assets SET sha256 = ?`, sha256Hex("replaced"))
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
