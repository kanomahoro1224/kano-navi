package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// cachedPosts serves a snapshot the test writes to directly.
type cachedPosts struct {
	t       *testing.T
	path    string
	handler http.Handler
}

func newCachedPosts(t *testing.T) *cachedPosts {
	t.Helper()
	path := testutil.NewSnapshot(t)
	store, err := sqlite.OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	p := &cachedPosts{t: t, path: path}
	p.handler = NewHandlerWithClock(store, func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) })
	p.addPost("post-first", "2026-10-01T03:00:00Z")
	return p
}

// addPost stores a post without advancing the dashboard revision, as a write
// the cache cannot see would.
func (p *cachedPosts) addPost(id, published string) {
	testutil.Exec(p.t, p.path, `INSERT INTO posts (id,source,account_handle,type,text,published_at,url) VALUES (?,'x','kano_2525','daily',?,?,?)`,
		id, id, published, "https://x.com/kano_2525/status/"+id)
}

func (p *cachedPosts) bumpRevision() {
	testutil.Exec(p.t, p.path, `INSERT INTO app_settings (key,value,updated_at) VALUES ('dashboard_revision','1','')
		ON CONFLICT(key) DO UPDATE SET value=CAST(value AS INTEGER)+1`)
}

func (p *cachedPosts) read(path, etag string) *httptest.ResponseRecorder {
	p.t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	recorder := httptest.NewRecorder()
	p.handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK && recorder.Code != http.StatusNotModified {
		p.t.Fatalf("%s status=%d body=%s", path, recorder.Code, recorder.Body)
	}
	return recorder
}

// The post timeline and the month index are answered from their prepared
// reads while the revision stands, a 304 included, and read again once it
// moves; an account's spelling names the same read.
func TestPostReadsFollowTheRevision(t *testing.T) {
	p := newCachedPosts(t)
	const timeline = "/api/posts?page=1&pageSize=30"
	first := p.read(timeline, "")
	etag := first.Header().Get("ETag")
	months := p.read("/api/posts/months", "")
	account := p.read(timeline+"&account=Kano_2525", "")
	p.addPost("post-unseen", "2026-11-02T03:00:00Z")

	again := p.read(timeline, "")
	if again.Body.String() != first.Body.String() || again.Header().Get("ETag") != etag {
		t.Fatal("an unchanged revision read the timeline again")
	}
	if revalidated := p.read(timeline, etag); revalidated.Code != http.StatusNotModified {
		t.Fatalf("revalidation status=%d, want 304", revalidated.Code)
	}
	if p.read("/api/posts/months", "").Body.String() != months.Body.String() {
		t.Fatal("an unchanged revision read the month index again")
	}
	if p.read(timeline+"&account=kano_2525", "").Body.String() != account.Body.String() {
		t.Fatal("another spelling of the account read the timeline again")
	}

	p.bumpRevision()
	changed := p.read(timeline, "")
	if !lists(changed, "post-unseen") || changed.Header().Get("ETag") == etag {
		t.Fatalf("a new revision kept the prepared timeline: %s", changed.Body)
	}
	if !strings.Contains(p.read("/api/posts/months", "").Body.String(), `"month":"2026-11"`) {
		t.Fatal("a new revision kept the prepared month index")
	}
}

// Readers that miss together share one computation; a failed one is not
// kept, a newer revision drops the older entries, an older one never
// replaces them, and a full cache lets its oldest entry go.
func TestPostReadCacheSharesAndBoundsItsReads(t *testing.T) {
	cache := newPostReadCache()
	ctx := context.Background()
	ready := func(body string) func() (preparedBody, error) {
		return func() (preparedBody, error) { return preparedBody{body: []byte(body), etag: `W/"` + body + `"`}, nil }
	}

	var computed atomic.Int32
	release := make(chan struct{})
	shared := postReadKey{revision: 7, request: "post months"}
	var wg sync.WaitGroup
	results := make([]string, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			prepared, err := cache.get(ctx, shared, func() (preparedBody, error) {
				computed.Add(1)
				<-release
				return ready("seven")()
			})
			if err != nil {
				t.Error(err)
			}
			results[i] = string(prepared.body)
		}()
	}
	for computed.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()
	if computed.Load() != 1 {
		t.Fatalf("computed %d times, want 1", computed.Load())
	}
	for _, body := range results {
		if body != "seven" {
			t.Fatalf("a reader received %q", body)
		}
	}

	failing := postReadKey{revision: 7, request: "posts page=2"}
	if _, err := cache.get(ctx, failing, func() (preparedBody, error) { return preparedBody{}, errors.New("unavailable") }); err == nil {
		t.Fatal("a failed computation succeeded")
	}
	retried := false
	if _, err := cache.get(ctx, failing, func() (preparedBody, error) { retried = true; return ready("page two")() }); err != nil || !retried {
		t.Fatalf("a failed computation was kept: retried=%v err=%v", retried, err)
	}

	recomputes := func(key postReadKey) bool {
		t.Helper()
		again := false
		if _, err := cache.get(ctx, key, func() (preparedBody, error) { again = true; return ready("again")() }); err != nil {
			t.Fatal(err)
		}
		return again
	}
	if recomputes(shared) {
		t.Fatal("a stored read was computed again")
	}
	if _, err := cache.get(ctx, postReadKey{revision: 6, request: "post months"}, ready("six")); err != nil {
		t.Fatal(err)
	}
	if prepared, _ := cache.get(ctx, shared, ready("unused")); string(prepared.body) != "seven" {
		t.Fatalf("an older revision replaced the newer read: %q", prepared.body)
	}
	if !recomputes(postReadKey{revision: 8, request: "post months"}) || !recomputes(postReadKey{revision: 8, request: "posts page=2"}) {
		t.Fatal("a newer revision answered from the older one")
	}
	if !recomputes(shared) {
		t.Fatal("an older revision's read stayed after a newer one was stored")
	}

	full := newPostReadCache()
	for i := range postReadCacheEntries + 1 {
		if _, err := full.get(ctx, postReadKey{revision: 1, request: fmt.Sprint("page ", i)}, ready("page")); err != nil {
			t.Fatal(err)
		}
	}
	cache = full
	if !recomputes(postReadKey{revision: 1, request: "page 0"}) {
		t.Fatal("a full cache kept its oldest read")
	}
	if recomputes(postReadKey{revision: 1, request: fmt.Sprint("page ", postReadCacheEntries)}) {
		t.Fatal("a full cache dropped its newest read")
	}
}

// A read that panics keeps nothing: the readers waiting on it fail, and the
// next read computes again.
func TestAPanickedPostReadIsNeitherKeptNorShared(t *testing.T) {
	cache := newPostReadCache()
	ctx := context.Background()
	key := postReadKey{revision: 3, request: "post months"}
	var call *postReadCall
	func() {
		defer func() { _ = recover() }()
		_, _ = cache.get(ctx, key, func() (preparedBody, error) {
			cache.mu.Lock()
			call = cache.calls[key]
			cache.mu.Unlock()
			panic("synthetic read failure")
		})
	}()
	<-call.done
	if call.err == nil {
		t.Fatal("the readers waiting on a panicked read would get no error")
	}
	recomputed := false
	prepared, err := cache.get(ctx, key, func() (preparedBody, error) { recomputed = true; return preparedBody{body: []byte("fresh")}, nil })
	if err != nil || !recomputed || string(prepared.body) != "fresh" {
		t.Fatalf("prepared=%q err=%v recomputed=%v", prepared.body, err, recomputed)
	}
}
