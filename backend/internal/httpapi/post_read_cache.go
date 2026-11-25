package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"
)

// postReadComputeTimeout bounds one shared post read, which runs detached
// from the request that started it so a departing client does not fail the
// readers waiting on it.
const postReadComputeTimeout = 15 * time.Second

// postReadCacheEntries bounds the prepared post reads kept for the current
// revision: the month index and the pages visitors read, each of at most 50
// posts.
const postReadCacheEntries = 64

var errPostReadEncoding = errors.New("post read could not be encoded")

// postReadKey names one post read: the durable revision it was computed at
// and its normalized request.
type postReadKey struct {
	revision int64
	request  string
}

type postReadCall struct {
	done     chan struct{}
	prepared preparedBody
	err      error
}

// postReadCache holds the prepared post archive reads of the newest
// revision it has seen. Posts carry no time-derived fields, so an entry stays
// current until the revision changes; the revision is read from SQLite on
// each request, so writes by another process end it as well. Readers that
// miss together share one computation, and storing a newer revision drops
// every older entry. Once full, the oldest entry gives way.
type postReadCache struct {
	mu       sync.Mutex
	revision int64
	entries  map[string]preparedBody
	order    []string
	calls    map[postReadKey]*postReadCall
}

func newPostReadCache() *postReadCache {
	return &postReadCache{entries: map[string]preparedBody{}, calls: map[postReadKey]*postReadCall{}}
}

// get returns the prepared read for key, or computes it with compute, which
// concurrent callers of the same key share. A failed computation is not kept.
func (c *postReadCache) get(ctx context.Context, key postReadKey, compute func() (preparedBody, error)) (preparedBody, error) {
	if c == nil {
		return compute()
	}
	c.mu.Lock()
	if prepared, ok := c.entries[key.request]; ok && key.revision == c.revision {
		c.mu.Unlock()
		return prepared, nil
	}
	if call := c.calls[key]; call != nil {
		c.mu.Unlock()
		select {
		case <-call.done:
			return call.prepared, call.err
		case <-ctx.Done():
			return preparedBody{}, ctx.Err()
		}
	}
	call := &postReadCall{done: make(chan struct{})}
	c.calls[key] = call
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.calls, key)
		if call.err == nil {
			c.store(key, call.prepared)
		}
		c.mu.Unlock()
		close(call.done)
	}()
	call.prepared, call.err = compute()
	return call.prepared, call.err
}

// store keeps a computed read under the lock. An older revision's read never
// replaces the newer revision's entries.
func (c *postReadCache) store(key postReadKey, prepared preparedBody) {
	if key.revision < c.revision {
		return
	}
	if key.revision > c.revision {
		c.revision = key.revision
		c.entries = map[string]preparedBody{}
		c.order = nil
	}
	if _, ok := c.entries[key.request]; !ok {
		if len(c.order) >= postReadCacheEntries {
			delete(c.entries, c.order[0])
			c.order = c.order[1:]
		}
		c.order = append(c.order, key.request)
	}
	c.entries[key.request] = prepared
}

// servePostRead answers a post archive read named by request from the cache
// at the current revision, or with read's payload when it misses. Without a
// revision nothing is reused.
func (h *Handler) servePostRead(w http.ResponseWriter, r *http.Request, request string, read func(context.Context) (map[string]any, error)) {
	prepare := func(ctx context.Context) (preparedBody, error) {
		payload, err := read(ctx)
		if err != nil {
			return preparedBody{}, err
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return preparedBody{}, errPostReadEncoding
		}
		return preparedBody{body: body, etag: revalidatedETag(body)}, nil
	}
	var prepared preparedBody
	var err error
	if revision, revisionErr := h.store.GetDashboardRevision(r.Context()); revisionErr != nil {
		prepared, err = prepare(r.Context())
	} else {
		prepared, err = h.postReads.get(r.Context(), postReadKey{revision: revision, request: request}, func() (preparedBody, error) {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), postReadComputeTimeout)
			defer cancel()
			return prepare(ctx)
		})
	}
	switch {
	case errors.Is(err, errPostReadEncoding):
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"}, "")
	case err != nil:
		writeArchiveError(w, err)
	default:
		writePreparedJSON(w, r, prepared.body, prepared.etag)
	}
}
