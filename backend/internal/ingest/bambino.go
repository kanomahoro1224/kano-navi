// Package ingest accepts BambinoStatus' normalized updates and stores them as
// kano posts. It is an internal service endpoint, authenticated by an HMAC
// shared with the bridge, not by the operator session.
package ingest

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/yexca/kano-navi/backend/internal/jobs"
	"github.com/yexca/kano-navi/backend/internal/push"
	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
	ytsource "github.com/yexca/kano-navi/backend/internal/source/youtube"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const (
	// MaxBodyBytes bounds a batch; v1 carries URLs rather than image bytes.
	MaxBodyBytes = 2 << 20
	MaxEvents    = 50
	// Chain is the post-processing run submitted after a successful ingest.
	Chain = "media,schedule,translate"
)

// Module serves POST /internal/ingest/bambino.
type Module struct {
	store     *sqlite.Store
	jobs      *jobs.QueueManager
	notifier  *push.Dispatcher
	secret    []byte
	tolerance time.Duration
	now       func() time.Time
}

// Options wires the ingest module. A nil Jobs value disables post-processing
// submission; the catch-up ticker still needs it to drain pending work. A nil
// Notifier leaves push off.
type Options struct {
	Store     *sqlite.Store
	Jobs      *jobs.QueueManager
	Notifier  *push.Dispatcher
	Secret    string
	Tolerance time.Duration
	Now       func() time.Time
}

// New returns nil when the shared secret is empty, so the route stays absent
// instead of accepting unauthenticated writes.
func New(options Options) *Module {
	if strings.TrimSpace(options.Secret) == "" || options.Store == nil {
		return nil
	}
	if options.Tolerance <= 0 {
		options.Tolerance = 5 * time.Minute
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Module{
		store:     options.Store,
		jobs:      options.Jobs,
		notifier:  options.Notifier,
		secret:    []byte(options.Secret),
		tolerance: options.Tolerance,
		now:       options.Now,
	}
}

func (m *Module) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "")
		return
	}
	if err = m.verify(r, body); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_signature", err.Error())
		return
	}
	var batch batchRequest
	if err = json.Unmarshal(body, &batch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "")
		return
	}
	if batch.Schema != 1 {
		writeError(w, http.StatusBadRequest, "unsupported_schema", "")
		return
	}
	if len(batch.Events) == 0 || len(batch.Events) > MaxEvents {
		writeError(w, http.StatusBadRequest, "invalid_event", "events size")
		return
	}

	response := batchResponse{Rejected: []rejection{}}
	retryable := false
	// fresh keeps the events that actually changed storage: a duplicate is not
	// news, and a rejected event never became a row to announce.
	fresh := make([]eventRequest, 0, len(batch.Events))
	for index, event := range batch.Events {
		result, reject := m.ingest(r.Context(), event)
		if reject != nil {
			reject.Index = index
			reject.ID = event.ID
			response.Rejected = append(response.Rejected, *reject)
			if reject.Code == "storage_unavailable" {
				retryable = true
			}
			continue
		}
		if result.Duplicate {
			response.Duplicates++
		} else {
			response.Accepted++
			fresh = append(fresh, event)
		}
		if result.Revision > response.Revision {
			response.Revision = result.Revision
		}
	}

	if response.Accepted > 0 {
		response.RunID = m.submit(r.Context(), "bambino-ingest")
		m.notify(fresh)
	}
	response.OK = len(response.Rejected) == 0 && !retryable
	if retryable {
		writeJSON(w, http.StatusServiceUnavailable, response)
		return
	}
	if response.Accepted > 0 {
		writeJSON(w, http.StatusAccepted, response)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// notify hands the accepted events to the push dispatcher. Delivery runs off
// the request goroutine: a slow push endpoint must never hold up the
// collector's batch, and a failed push must never become a failed ingest.
func (m *Module) notify(events []eventRequest) {
	if m.notifier == nil || len(events) == 0 {
		return
	}
	updates := make([]push.Update, 0, len(events))
	for _, event := range events {
		author := strings.TrimSpace(event.Author.DisplayName)
		if author == "" {
			author = strings.TrimSpace(event.Author.Handle)
		}
		updates = append(updates, push.Update{
			ID:       strings.TrimSpace(event.ID),
			Platform: strings.ToLower(strings.TrimSpace(event.Platform)),
			Kind:     strings.ToLower(strings.TrimSpace(event.Kind)),
			Author:   author,
			Title:    event.Title,
			Text:     event.Text,
			URL:      event.URL,
		})
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Japanese readers see the original wording at once; every other
		// language waits for the translate step to settle the post.
		if err := m.notifier.DispatchPosts(ctx, updates); err != nil {
			log.Printf("kano-go: push dispatch failed: %v", err)
		}
	}()
}

// flushPending releases the posts whose translation settled since the last
// check. The translate step calls it through its own hook; the catch-up tick
// covers a queue that never ran the step.
func (m *Module) flushPending() {
	if m.notifier == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := m.notifier.FlushPending(ctx); err != nil {
			log.Printf("kano-go: push flush failed: %v", err)
		}
	}()
}

// RunCatchUp submits the same post-processing chain outside a webhook, so a
// busy single-flight queue at ingest time cannot strand pending media or
// schedule extraction.
func (m *Module) RunCatchUp(ctx context.Context) {
	if m == nil {
		return
	}
	m.flushPending()
	if m.jobs == nil {
		return
	}
	if _, err := m.jobs.Start(ctx, jobs.KindWorkflow, "bambino-catchup", jobs.Options{Steps: strings.Split(Chain, ",")}); err != nil {
		log.Printf("kano-go: bambino catch-up not submitted: %v", err)
	}
}

func (m *Module) submit(ctx context.Context, triggeredBy string) string {
	if m.jobs == nil {
		return ""
	}
	request, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := m.jobs.Start(request, jobs.KindWorkflow, triggeredBy, jobs.Options{Steps: strings.Split(Chain, ",")})
	if err != nil {
		log.Printf("kano-go: bambino post-processing not submitted: %v", err)
		return ""
	}
	if !result.Accepted || result.Job == nil {
		if result.Reason != nil {
			log.Printf("kano-go: bambino post-processing deferred: %s", *result.Reason)
		}
		return ""
	}
	return result.Job.ID
}

func (m *Module) ingest(ctx context.Context, event eventRequest) (sqlite.BambinoIngestResult, *rejection) {
	switch strings.ToLower(strings.TrimSpace(event.Platform)) {
	case "bilibili", "youtube":
		return m.ingestVideo(ctx, event)
	default:
		return m.ingestPost(ctx, event)
	}
}

func (m *Module) ingestPost(ctx context.Context, event eventRequest) (sqlite.BambinoIngestResult, *rejection) {
	post, contentHash, reject := normalize(event, m.now())
	if reject != nil {
		return sqlite.BambinoIngestResult{}, reject
	}
	post.IngestHash = contentHash
	result, err := m.store.IngestBambinoPost(ctx, post)
	if err == nil {
		return result, nil
	}
	switch {
	case errors.Is(err, sqlite.ErrIngestConflict):
		return sqlite.BambinoIngestResult{}, &rejection{Code: "id_conflict", Message: "id belongs to another account"}
	case errors.Is(err, sqlite.ErrSourceWrite):
		return sqlite.BambinoIngestResult{}, &rejection{Code: "invalid_media", Message: "media URL rejected"}
	default:
		log.Printf("kano-go: bambino ingest storage failed: %v", err)
		return sqlite.BambinoIngestResult{}, &rejection{Code: "storage_unavailable", Message: "storage unavailable"}
	}
}

func (m *Module) ingestVideo(ctx context.Context, event eventRequest) (sqlite.BambinoIngestResult, *rejection) {
	video, contentHash, reject := normalizeVideo(event, m.now())
	if reject != nil {
		return sqlite.BambinoIngestResult{}, reject
	}
	video.IngestHash = contentHash
	result, err := m.store.IngestBambinoVideo(ctx, video)
	if err == nil {
		return result, nil
	}
	switch {
	case errors.Is(err, sqlite.ErrIngestConflict):
		return sqlite.BambinoIngestResult{}, &rejection{Code: "id_conflict", Message: "id belongs to another source"}
	case errors.Is(err, sqlite.ErrSourceWrite):
		return sqlite.BambinoIngestResult{}, &rejection{Code: "invalid_media", Message: "thumbnail URL rejected"}
	default:
		log.Printf("kano-go: bambino video ingest storage failed: %v", err)
		return sqlite.BambinoIngestResult{}, &rejection{Code: "storage_unavailable", Message: "storage unavailable"}
	}
}

func (m *Module) verify(r *http.Request, body []byte) error {
	timestamp := strings.TrimSpace(r.Header.Get("X-Bambino-Timestamp"))
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errors.New("timestamp missing")
	}
	at := time.Unix(seconds, 0)
	if delta := m.now().Sub(at); delta > m.tolerance || delta < -m.tolerance {
		return errors.New("timestamp outside tolerance")
	}
	signature := strings.TrimSpace(r.Header.Get("X-Bambino-Signature"))
	signature = strings.TrimPrefix(signature, "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return errors.New("signature malformed")
	}
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("\n"))
	mac.Write(body)
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return errors.New("signature mismatch")
	}
	return nil
}

func normalize(event eventRequest, now time.Time) (sqlite.BambinoPost, string, *rejection) {
	if strings.TrimSpace(event.ID) == "" {
		return sqlite.BambinoPost{}, "", reject("invalid_event", "id is required")
	}
	platform := strings.ToLower(strings.TrimSpace(event.Platform))
	if platform != "x" && platform != "twitter" {
		return sqlite.BambinoPost{}, "", reject("unsupported_platform", "posts are accepted for x only")
	}
	kind := strings.ToLower(strings.TrimSpace(event.Kind))
	switch kind {
	case "post", "quote", "repost":
	default:
		return sqlite.BambinoPost{}, "", reject("invalid_event", "unknown kind")
	}
	published, err := time.Parse(time.RFC3339, strings.TrimSpace(event.PublishedAt))
	if err != nil {
		return sqlite.BambinoPost{}, "", reject("invalid_event", "published_at must be RFC3339")
	}
	if published.After(now.Add(10 * time.Minute)) {
		return sqlite.BambinoPost{}, "", reject("future_timestamp", "published_at is in the future")
	}
	handle := strings.TrimPrefix(strings.TrimSpace(event.Author.Handle), "@")
	if handle == "" {
		return sqlite.BambinoPost{}, "", reject("invalid_event", "author.handle is required")
	}
	url := strings.TrimSpace(event.URL)
	if url == "" {
		url = "https://x.com/" + handle + "/status/" + event.ID
	}
	url = strings.ReplaceAll(url, "twitter.com", "x.com")

	text := event.Text
	media := mediaURLs(event.Media)
	var quote *sqlite.BambinoQuote
	switch kind {
	case "quote":
		if event.Quote == nil || strings.TrimSpace(event.Quote.ID) == "" {
			return sqlite.BambinoPost{}, "", reject("invalid_event", "quote is required")
		}
		quote = quoteOf(event.Quote)
	case "repost":
		if event.Retweet != nil && strings.TrimSpace(event.Retweet.ID) != "" {
			text = event.Retweet.Text
			media = mediaURLs(event.Retweet.Media)
		}
	}
	if strings.TrimSpace(text) == "" && len(media) == 0 && quote == nil {
		return sqlite.BambinoPost{}, "", reject("invalid_event", "text, media or quote is required")
	}
	if !validMedia(media) || (quote != nil && !validMedia(quote.MediaURLs)) {
		return sqlite.BambinoPost{}, "", reject("invalid_media", "media URL must be http(s)")
	}

	searchText := text
	if quote != nil && quote.Text != "" {
		searchText = strings.TrimSpace(searchText + "\n" + quote.Text)
	}
	publishedAt := sqlite.ISOTime(published)
	rawEvent := event.Raw
	if len(rawEvent) == 0 || !json.Valid(rawEvent) {
		if body, err := json.Marshal(event); err == nil {
			rawEvent = body
		}
	}
	hashInput := []string{event.ID, text, publishedAt, url, handle}
	hashInput = append(hashInput, media...)
	if quote != nil {
		hashInput = append(hashInput, quote.ID, quote.Text)
		hashInput = append(hashInput, quote.MediaURLs...)
	}
	sum := sha256.Sum256([]byte(strings.Join(hashInput, "\x1f")))
	contentHash := hex.EncodeToString(sum[:])
	raw := map[string]any{
		"search_text": searchText,
		"media_urls":  media,
		"bambino":     rawEvent,
		"ingest_hash": contentHash,
	}
	rawBody, err := json.Marshal(raw)
	if err != nil {
		return sqlite.BambinoPost{}, "", reject("invalid_event", "raw payload")
	}

	return sqlite.BambinoPost{
		ID:            event.ID,
		AccountHandle: handle,
		Text:          text,
		PublishedAt:   publishedAt,
		URL:           url,
		Likes:         max(0, event.Metrics.Likes),
		Reposts:       max(0, event.Metrics.Reposts),
		Replies:       max(0, event.Metrics.Replies),
		MediaURLs:     media,
		Quote:         quote,
		RawJSON:       string(rawBody),
	}, contentHash, nil
}

var youtubeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// videoKinds are the event kinds that describe one videos row. Everything else
// a collector reports (Bilibili text dynamics, YouTube live updates) has no
// stored representation and is rejected per event.
var videoKinds = map[string]bool{
	"video":         true,
	"shorts":        true,
	"live_schedule": true,
	"live_start":    true,
	"live_end":      true,
}

// normalizeVideo maps a collector's upload event onto the videos table. The
// collector runs where the platform APIs are reachable; this side only
// validates and stores. Bilibili ids are BV ids, YouTube ids are the 11
// character video ids, and the two namespaces never overlap.
func normalizeVideo(event eventRequest, now time.Time) (sqlite.BambinoVideo, string, *rejection) {
	source := strings.ToLower(strings.TrimSpace(event.Platform))
	if source != "bilibili" && source != "youtube" {
		return sqlite.BambinoVideo{}, "", reject("unsupported_platform", "video events are bilibili or youtube")
	}
	id := strings.TrimSpace(event.ID)
	if source == "bilibili" {
		if parsed, ok := bilisource.ParseBVID(id); !ok || parsed != id {
			return sqlite.BambinoVideo{}, "", reject("invalid_event", "id must be a BV id")
		}
	} else if !youtubeIDPattern.MatchString(id) {
		return sqlite.BambinoVideo{}, "", reject("invalid_event", "id must be a YouTube video id")
	}
	kind := strings.ToLower(strings.TrimSpace(event.Kind))
	if !videoKinds[kind] {
		return sqlite.BambinoVideo{}, "", reject("invalid_event", "unknown video kind")
	}
	if source == "bilibili" && kind != "video" {
		return sqlite.BambinoVideo{}, "", reject("invalid_event", "bilibili video kind must be video")
	}
	published, err := time.Parse(time.RFC3339, strings.TrimSpace(event.PublishedAt))
	if err != nil {
		return sqlite.BambinoVideo{}, "", reject("invalid_event", "published_at must be RFC3339")
	}
	if published.After(now.Add(10 * time.Minute)) {
		return sqlite.BambinoVideo{}, "", reject("future_timestamp", "published_at is in the future")
	}
	title := strings.TrimSpace(event.Title)
	if title == "" {
		title = strings.TrimSpace(event.Text)
	}
	if title == "" {
		return sqlite.BambinoVideo{}, "", reject("invalid_event", "title is required")
	}
	url := strings.TrimSpace(event.URL)
	if url == "" {
		if source == "bilibili" {
			url = "https://www.bilibili.com/video/" + id
		} else {
			url = "https://www.youtube.com/watch?v=" + id
		}
	}
	if !validMedia([]string{url}) {
		return sqlite.BambinoVideo{}, "", reject("invalid_media", "url must be http(s)")
	}
	thumbnail := strings.TrimSpace(event.ThumbnailURL)
	if thumbnail == "" && len(event.Media) > 0 {
		thumbnail = strings.TrimSpace(event.Media[0].URL)
	}
	if strings.HasPrefix(thumbnail, "//") {
		thumbnail = "https:" + thumbnail
	}
	if strings.HasPrefix(strings.ToLower(thumbnail), "http://") {
		thumbnail = "https://" + thumbnail[len("http://"):]
	}
	if thumbnail != "" && !validMedia([]string{thumbnail}) {
		return sqlite.BambinoVideo{}, "", reject("invalid_media", "thumbnail URL must be http(s)")
	}
	scheduled := ""
	if raw := strings.TrimSpace(event.ScheduledAt); raw != "" {
		at, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			return sqlite.BambinoVideo{}, "", reject("invalid_event", "scheduled_at must be RFC3339")
		}
		scheduled = sqlite.ISOTime(at)
	}
	duration := max(0, event.DurationSeconds)
	isUpcoming, liveStatus := false, ""
	switch kind {
	case "live_schedule":
		isUpcoming, liveStatus = true, ytsource.LiveStatusUpcoming
	case "live_start":
		liveStatus = ytsource.LiveStatusLive
	case "live_end":
		liveStatus = ytsource.LiveStatusEnded
	}
	var isShort *bool
	if event.IsShort != nil {
		isShort = event.IsShort
	} else if source == "youtube" {
		short := kind == "shorts"
		isShort = &short
	}
	// The stored kind is the form, never the content: a collector reports a
	// broadcast as live and everything else as an upload (ADR-0005). Bilibili
	// events are uploads only, so the switch covers both platforms.
	boardKind := ytsource.KindUpload
	switch kind {
	case "live_schedule", "live_start", "live_end":
		boardKind = ytsource.KindStream
	}
	publishedAt := sqlite.ISOTime(published)
	rawEvent := event.Raw
	if len(rawEvent) == 0 || !json.Valid(rawEvent) {
		if body, err := json.Marshal(event); err == nil {
			rawEvent = body
		}
	}
	shortText := ""
	if isShort != nil {
		shortText = strconv.FormatBool(*isShort)
	}
	hashInput := []string{id, source, title, publishedAt, scheduled, url, thumbnail, boardKind, strconv.Itoa(duration), shortText, kind}
	sum := sha256.Sum256([]byte(strings.Join(hashInput, "\x1f")))
	contentHash := hex.EncodeToString(sum[:])
	mediaURLs := []string{}
	if thumbnail != "" {
		mediaURLs = append(mediaURLs, thumbnail)
	}
	rawBody, err := json.Marshal(map[string]any{
		"search_text": title,
		"media_urls":  mediaURLs,
		"bambino":     rawEvent,
		"ingest_hash": contentHash,
	})
	if err != nil {
		return sqlite.BambinoVideo{}, "", reject("invalid_event", "raw payload")
	}
	return sqlite.BambinoVideo{
		ID:              id,
		Source:          source,
		Title:           title,
		PublishedAt:     publishedAt,
		ScheduledAt:     scheduled,
		URL:             url,
		ThumbnailURL:    thumbnail,
		Kind:            boardKind,
		IsUpcoming:      isUpcoming,
		LiveStatus:      liveStatus,
		DurationSeconds: duration,
		IsShort:         isShort,
		RawJSON:         string(rawBody),
	}, contentHash, nil
}

func quoteOf(nested *nestedRequest) *sqlite.BambinoQuote {
	url := strings.TrimSpace(nested.URL)
	if url == "" {
		url = "https://x.com/" + strings.TrimPrefix(strings.TrimSpace(nested.Author.Handle), "@") + "/status/" + nested.ID
	}
	return &sqlite.BambinoQuote{
		ID:           strings.TrimSpace(nested.ID),
		AuthorHandle: strings.TrimPrefix(strings.TrimSpace(nested.Author.Handle), "@"),
		AuthorName:   nested.Author.DisplayName,
		Text:         nested.Text,
		PublishedAt:  nested.PublishedAt,
		URL:          strings.ReplaceAll(url, "twitter.com", "x.com"),
		MediaURLs:    mediaURLs(nested.Media),
	}
}

func mediaURLs(items []mediaRequest) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if url := strings.TrimSpace(item.URL); url != "" {
			out = append(out, url)
		}
	}
	return out
}

func validMedia(urls []string) bool {
	for _, raw := range urls {
		lower := strings.ToLower(raw)
		if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") {
			return false
		}
	}
	return true
}

func reject(code, message string) *rejection {
	return &rejection{Code: code, Message: message}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-transform")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": code, "message": message})
}

type batchRequest struct {
	Schema  int            `json:"schema"`
	BatchID string         `json:"batch_id"`
	SentAt  string         `json:"sent_at"`
	Source  string         `json:"source"`
	Events  []eventRequest `json:"events"`
}

type eventRequest struct {
	ID              string          `json:"id"`
	Platform        string          `json:"platform"`
	Kind            string          `json:"kind"`
	PublishedAt     string          `json:"published_at"`
	ScheduledAt     string          `json:"scheduled_at"`
	URL             string          `json:"url"`
	Author          authorRequest   `json:"author"`
	Text            string          `json:"text"`
	Title           string          `json:"title"`
	Media           []mediaRequest  `json:"media"`
	ThumbnailURL    string          `json:"thumbnail_url"`
	DurationSeconds int             `json:"duration_seconds"`
	IsShort         *bool           `json:"is_short"`
	Quote           *nestedRequest  `json:"quote"`
	Retweet         *nestedRequest  `json:"retweet"`
	Metrics         metricRequest   `json:"metrics"`
	Raw             json.RawMessage `json:"raw"`
}

type authorRequest struct {
	Handle      string `json:"handle"`
	ExternalID  string `json:"external_id"`
	DisplayName string `json:"display_name"`
	ProfileURL  string `json:"profile_url"`
}

type mediaRequest struct {
	Type       string `json:"type"`
	URL        string `json:"url"`
	PreviewURL string `json:"preview_url"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

type nestedRequest struct {
	ID          string         `json:"id"`
	URL         string         `json:"url"`
	Author      authorRequest  `json:"author"`
	PublishedAt string         `json:"published_at"`
	Text        string         `json:"text"`
	Media       []mediaRequest `json:"media"`
}

type metricRequest struct {
	Likes   int `json:"likes"`
	Reposts int `json:"reposts"`
	Replies int `json:"replies"`
}

type rejection struct {
	Index   int    `json:"index"`
	ID      string `json:"id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type batchResponse struct {
	OK         bool        `json:"ok"`
	Accepted   int         `json:"accepted"`
	Duplicates int         `json:"duplicates"`
	Rejected   []rejection `json:"rejected"`
	Revision   int64       `json:"revision"`
	RunID      string      `json:"runId,omitempty"`
}
