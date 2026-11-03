package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const (
	// MaxWebPushBodyBytes bounds a subscription: an endpoint, two keys and
	// three flags, never media.
	MaxWebPushBodyBytes = 8 << 10
	maxWebPushEndpoint  = 2048
	// webPushRateLimit is how many registration writes one address may make
	// per window. A browser subscribes once and then only on a switch change,
	// so this is far above normal use and only stops a script.
	webPushRateLimit  = 30
	webPushRateWindow = time.Hour
)

// webPushRequest is the browser's registration payload. The switches are
// pointers so an omitted field keeps the stored value instead of reading as
// "the reader turned it off".
type webPushRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	Locale  string `json:"locale"`
	Enabled *bool  `json:"enabled"`
	Live    *bool  `json:"live"`
	Video   *bool  `json:"video"`
	Post    *bool  `json:"post"`
}

// webPushKey answers the applicationServerKey a browser subscribes with. It is
// no-store: a browser may ask before every subscribe, and the key changes only
// with the deployment's configuration.
func (h *Handler) webPushKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.webPush.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false}, "no-store")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":   true,
		"publicKey": h.webPush.PublicKey(),
	}, "no-store")
}

// webPushSubscribe registers or refreshes one browser. The call is idempotent:
// the App's own registration works the same way, because a browser re-posts
// after a key rotation or a switch change.
func (h *Handler) webPushSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	if !h.webPush.Enabled() || h.store == nil || !h.store.Writable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "push_unavailable"}, "no-store")
		return
	}
	if !h.webPushAllows(r) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too_many_requests"}, "no-store")
		return
	}
	var request webPushRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxWebPushBodyBytes)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"}, "")
		return
	}
	endpoint := strings.TrimSpace(request.Endpoint)
	if endpoint == "" || utf8.RuneCountInString(endpoint) > maxWebPushEndpoint || !h.webPush.AllowsEndpoint(endpoint) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_endpoint"}, "")
		return
	}
	p256dh, ok := decodeWebPushKey(request.Keys.P256DH, 65)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_keys"}, "")
		return
	}
	auth, ok := decodeWebPushKey(request.Keys.Auth, 16)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_keys"}, "")
		return
	}
	subscription := sqlite.WebPushSubscription{
		Endpoint:    endpoint,
		P256DH:      p256dh,
		Auth:        auth,
		Locale:      clipRunes(request.Locale, 32),
		UserAgent:   clipRunes(r.UserAgent(), 128),
		Enabled:     booleanOr(request.Enabled, true),
		NotifyLive:  booleanOr(request.Live, true),
		NotifyVideo: booleanOr(request.Video, true),
		NotifyPost:  booleanOr(request.Post, true),
	}
	// An omitted switch keeps what the row already had; a first registration
	// has no row and takes the defaults above.
	if existing, found, err := h.store.WebPushSubscription(r.Context(), endpoint); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "storage_unavailable"}, "no-store")
		return
	} else if found {
		if request.Live == nil {
			subscription.NotifyLive = existing.NotifyLive
		}
		if request.Video == nil {
			subscription.NotifyVideo = existing.NotifyVideo
		}
		if request.Post == nil {
			subscription.NotifyPost = existing.NotifyPost
		}
		if subscription.Locale == "" {
			subscription.Locale = existing.Locale
		}
	}
	if err := h.store.RegisterWebPushSubscription(r.Context(), subscription); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "storage_unavailable"}, "no-store")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

// webPushUnsubscribe retires one browser. Knowing the endpoint is the
// capability, so no other credential is asked for.
func (h *Handler) webPushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	if h.store == nil || !h.store.Writable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "storage_unavailable"}, "no-store")
		return
	}
	if !h.webPushAllows(r) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too_many_requests"}, "no-store")
		return
	}
	var request struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxWebPushBodyBytes)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"}, "")
		return
	}
	endpoint := strings.TrimSpace(request.Endpoint)
	if endpoint == "" || utf8.RuneCountInString(endpoint) > maxWebPushEndpoint {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_endpoint"}, "")
		return
	}
	if err := h.store.DeleteWebPushSubscription(r.Context(), endpoint); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "storage_unavailable"}, "no-store")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

// decodeWebPushKey accepts the unpadded base64url a browser returns and the
// padded form a manual test may paste, then checks the decoded length.
func decodeWebPushKey(value string, size int) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(value)
	}
	if err != nil || len(raw) != size {
		return "", false
	}
	return value, true
}

// webPushAllows applies the registration rate limit to one address. The
// forwarded header is used when a reverse proxy sets it; without one every
// request shares the proxy address, which still bounds a script.
func (h *Handler) webPushAllows(r *http.Request) bool {
	if h.webPushLimits == nil {
		return true
	}
	address := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		address = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	} else if host, _, err := net.SplitHostPort(address); err == nil {
		address = host
	}
	return h.webPushLimits.allow(address, h.now(), webPushRateLimit, webPushRateWindow)
}

// webPushLimiter is a small in-memory sliding window. It is not shared between
// processes, which is enough: the deployment serves the site from one process.
type webPushLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newWebPushLimiter() *webPushLimiter {
	return &webPushLimiter{hits: map[string][]time.Time{}}
}

func (l *webPushLimiter) allow(key string, now time.Time, limit int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-window)
	kept := l.hits[key][:0]
	for _, at := range l.hits[key] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) >= limit {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}
