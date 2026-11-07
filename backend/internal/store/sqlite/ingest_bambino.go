package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	xsource "github.com/yexca/kano-navi/backend/internal/source/x"
)

// ErrIngestConflict reports an event whose id already belongs to another
// account. The stored row is left unchanged, mirroring the X sync writer.
var ErrIngestConflict = errors.New("ingest post belongs to another account")

// BambinoQuote is the quoted status carried by an ingest event.
type BambinoQuote struct {
	ID           string
	AuthorHandle string
	AuthorName   string
	Text         string
	PublishedAt  string
	URL          string
	MediaURLs    []string
}

// BambinoRepost is the original status a repost event carries.
type BambinoRepost struct {
	ID           string
	AuthorHandle string
	AuthorName   string
	AuthorAvatar string
	PublishedAt  string
	URL          string
}

// BambinoPost is one normalized DynamicUpdate from BambinoStatus.
type BambinoPost struct {
	ID string
	// Source is 'x' or 'bilibili'; empty means x, the only writer before
	// Bilibili dynamics were accepted here.
	Source        string
	AccountHandle string
	Text          string
	PublishedAt   string
	URL           string
	Likes         int
	Reposts       int
	Replies       int
	MediaURLs     []string
	Quote         *BambinoQuote
	Repost        *BambinoRepost
	// RawJSON is the stored raw_json document. It must include ingest_hash,
	// search_text and media_urls for the schedule and archive readers.
	RawJSON string
	// IngestHash covers the content fields, excluding mutable metrics.
	IngestHash string
}

// BambinoIngestResult reports whether the event changed the stored post.
type BambinoIngestResult struct {
	Duplicate bool
	Revision  int64
}

// BambinoVideo is one normalized upload pushed by a local collector. Source is
// 'bilibili' or 'youtube'; the two share the videos table but never the same
// id, and a row keeps the source it was written with.
type BambinoVideo struct {
	ID              string
	Source          string
	Title           string
	PublishedAt     string
	ScheduledAt     string
	URL             string
	ThumbnailURL    string
	Kind            string
	IsUpcoming      bool
	LiveStatus      string
	DurationSeconds int
	// IsShort is the feed's format verdict; nil leaves a stored value alone.
	IsShort *bool
	// RawJSON must include ingest_hash, search_text and media_urls.
	RawJSON    string
	IngestHash string
}

type bambinoQuoteJSON struct {
	ID           string `json:"id"`
	AuthorHandle string `json:"authorHandle,omitempty"`
	AuthorName   string `json:"authorName,omitempty"`
	Text         string `json:"text,omitempty"`
	PublishedAt  string `json:"publishedAt,omitempty"`
	URL          string `json:"url"`
}

type bambinoRepostJSON struct {
	ID           string `json:"id"`
	AuthorHandle string `json:"authorHandle,omitempty"`
	AuthorName   string `json:"authorName,omitempty"`
	AuthorAvatar string `json:"authorAvatar,omitempty"`
	PublishedAt  string `json:"publishedAt,omitempty"`
	URL          string `json:"url"`
}

type ingestHashJSON struct {
	IngestHash string `json:"ingest_hash"`
}

// IngestBambinoPost upserts one event and bumps the dashboard revision when
// the content changed. Duplicate content is a no-op.
func (s *Store) IngestBambinoPost(ctx context.Context, p BambinoPost) (BambinoIngestResult, error) {
	p.ID = strings.TrimSpace(p.ID)
	p.Source = strings.ToLower(strings.TrimSpace(p.Source))
	if p.Source == "" {
		p.Source = "x"
	}
	if p.Source == "x" {
		// Only X handles are configured accounts with a canonical spelling;
		// a Bilibili UP name is whatever the collector read.
		p.AccountHandle = s.canonicalXHandle(ctx, p.AccountHandle)
		p.URL = strings.ReplaceAll(p.URL, "twitter.com", "x.com")
	}
	p.MediaURLs = nonEmptyURLs(p.MediaURLs)
	if p.Quote != nil {
		p.Quote.MediaURLs = nonEmptyURLs(p.Quote.MediaURLs)
		if p.Source == "x" && (p.Quote.URL == "" || !strings.Contains(p.Quote.URL, p.Quote.ID)) {
			p.Quote.URL = "https://x.com/" + p.Quote.AuthorHandle + "/status/" + p.Quote.ID
		}
	}
	if p.Repost != nil && p.Repost.AuthorHandle != "" && (p.Repost.URL == "" || !strings.Contains(p.Repost.URL, p.Repost.ID)) {
		if p.Source == "bilibili" {
			p.Repost.URL = "https://t.bilibili.com/" + p.Repost.ID
		} else {
			p.Repost.URL = "https://x.com/" + p.Repost.AuthorHandle + "/status/" + p.Repost.ID
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BambinoIngestResult{}, dbErr(err)
	}
	defer tx.Rollback()

	var storedRaw, storedAccount sql.NullString
	lookup := tx.QueryRowContext(ctx, `SELECT raw_json,account_handle FROM posts WHERE id=?`, p.ID)
	switch err = lookup.Scan(&storedRaw, &storedAccount); {
	case err == nil:
		if storedAccount.Valid && p.AccountHandle != "" && !strings.EqualFold(storedAccount.String, p.AccountHandle) {
			return BambinoIngestResult{}, ErrIngestConflict
		}
		if p.IngestHash != "" && ingestHashOf(storedRaw.String) == p.IngestHash {
			revision, revErr := revisionTx(ctx, tx)
			if revErr != nil {
				return BambinoIngestResult{}, revErr
			}
			return BambinoIngestResult{Duplicate: true, Revision: revision}, nil
		}
	case errors.Is(err, sql.ErrNoRows):
	default:
		return BambinoIngestResult{}, dbErr(err)
	}

	now := ISOTime(time.Now())
	postType, label := xsource.ClassifyText(p.Text)
	media := ""
	if len(p.MediaURLs) > 0 {
		media = p.MediaURLs[0]
	}
	quote := ""
	if p.Quote != nil {
		body, marshalErr := json.Marshal(bambinoQuoteJSON{
			ID:           p.Quote.ID,
			AuthorHandle: p.Quote.AuthorHandle,
			AuthorName:   p.Quote.AuthorName,
			Text:         p.Quote.Text,
			PublishedAt:  p.Quote.PublishedAt,
			URL:          p.Quote.URL,
		})
		if marshalErr != nil {
			return BambinoIngestResult{}, dbErr(marshalErr)
		}
		quote = string(body)
	}
	repost := ""
	if p.Repost != nil {
		body, marshalErr := json.Marshal(bambinoRepostJSON{
			ID:           p.Repost.ID,
			AuthorHandle: p.Repost.AuthorHandle,
			AuthorName:   p.Repost.AuthorName,
			AuthorAvatar: validRepostAvatar(p.Repost.AuthorAvatar),
			PublishedAt:  p.Repost.PublishedAt,
			URL:          p.Repost.URL,
		})
		if marshalErr != nil {
			return BambinoIngestResult{}, dbErr(marshalErr)
		}
		repost = string(body)
	}

	written, err := tx.ExecContext(ctx, `INSERT INTO posts(id,source,account_handle,type,label,text,published_at,url,likes,reposts,replies,media_url,media_alt,raw_json,quote_json,repost_json)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET account_handle=excluded.account_handle,type=excluded.type,label=excluded.label,text=excluded.text,published_at=excluded.published_at,url=excluded.url,likes=excluded.likes,reposts=excluded.reposts,replies=excluded.replies,media_url=excluded.media_url,media_alt=excluded.media_alt,raw_json=excluded.raw_json,quote_json=excluded.quote_json,repost_json=excluded.repost_json
WHERE lower(posts.account_handle)=lower(excluded.account_handle) OR posts.account_handle IS NULL`,
		p.ID, p.Source, nullable(p.AccountHandle), postType, label, p.Text, p.PublishedAt, p.URL, p.Likes, p.Reposts, p.Replies, nullable(media), nil, nullable(p.RawJSON), nullable(quote), nullable(repost))
	if err != nil {
		return BambinoIngestResult{}, dbErr(err)
	}
	if affected, rowErr := written.RowsAffected(); rowErr != nil {
		return BambinoIngestResult{}, dbErr(rowErr)
	} else if affected != 1 {
		return BambinoIngestResult{}, ErrIngestConflict
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM media_links WHERE owner_type='post' AND owner_id=?`, p.ID); err != nil {
		return BambinoIngestResult{}, dbErr(err)
	}
	for i, url := range p.MediaURLs {
		if err = replaceMediaLink(ctx, tx, p.Source, url, "post", p.ID, "post-image", i, now); err != nil {
			return BambinoIngestResult{}, dbErr(err)
		}
	}
	if p.Quote != nil {
		for i, url := range p.Quote.MediaURLs {
			if err = replaceMediaLink(ctx, tx, p.Source, url, "post", p.ID, roleQuoteImage, i, now); err != nil {
				return BambinoIngestResult{}, dbErr(err)
			}
		}
	}
	if p.Repost != nil {
		if avatar := validRepostAvatar(p.Repost.AuthorAvatar); avatar != "" {
			if err = replaceMediaLink(ctx, tx, p.Source, avatar, "post", p.ID, roleRepostAvatar, 0, now); err != nil {
				return BambinoIngestResult{}, dbErr(err)
			}
		}
	}
	if err = bumpRevision(ctx, tx, now); err != nil {
		return BambinoIngestResult{}, dbErr(err)
	}
	revision, err := revisionTx(ctx, tx)
	if err != nil {
		return BambinoIngestResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return BambinoIngestResult{}, dbErr(err)
	}
	return BambinoIngestResult{Revision: revision}, nil
}

// IngestBambinoVideo upserts one collector-pushed upload and bumps the
// dashboard revision when the content changed. Duplicate content is a no-op.
func (s *Store) IngestBambinoVideo(ctx context.Context, v BambinoVideo) (BambinoIngestResult, error) {
	v.ID = strings.TrimSpace(v.ID)
	v.Source = strings.ToLower(strings.TrimSpace(v.Source))
	v.Title = strings.TrimSpace(v.Title)
	v.URL = strings.TrimSpace(v.URL)
	v.ThumbnailURL = strings.TrimSpace(v.ThumbnailURL)
	v.Kind = strings.TrimSpace(v.Kind)
	v.LiveStatus = strings.TrimSpace(v.LiveStatus)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BambinoIngestResult{}, dbErr(err)
	}
	defer tx.Rollback()

	var storedRaw, storedSource sql.NullString
	lookup := tx.QueryRowContext(ctx, `SELECT raw_json,source FROM videos WHERE id=?`, v.ID)
	switch err = lookup.Scan(&storedRaw, &storedSource); {
	case err == nil:
		if storedSource.Valid && storedSource.String != v.Source {
			return BambinoIngestResult{}, ErrIngestConflict
		}
		if v.IngestHash != "" && ingestHashOf(storedRaw.String) == v.IngestHash {
			revision, revErr := revisionTx(ctx, tx)
			if revErr != nil {
				return BambinoIngestResult{}, revErr
			}
			return BambinoIngestResult{Duplicate: true, Revision: revision}, nil
		}
	case errors.Is(err, sql.ErrNoRows):
	default:
		return BambinoIngestResult{}, dbErr(err)
	}

	now := ISOTime(time.Now())
	var duration any
	if v.DurationSeconds > 0 {
		duration = v.DurationSeconds
	}
	var short any
	if v.IsShort != nil {
		short = *v.IsShort
	}
	written, err := tx.ExecContext(ctx, `INSERT INTO videos(id,source,title,published_at,scheduled_at,url,thumbnail_url,kind,is_upcoming,live_status,live_checked_at,duration_seconds,is_short,raw_json)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET title=excluded.title,published_at=excluded.published_at,scheduled_at=COALESCE(excluded.scheduled_at,videos.scheduled_at),url=excluded.url,thumbnail_url=COALESCE(excluded.thumbnail_url,videos.thumbnail_url),kind=excluded.kind,is_upcoming=excluded.is_upcoming,live_status=excluded.live_status,live_checked_at=excluded.live_checked_at,duration_seconds=COALESCE(excluded.duration_seconds,videos.duration_seconds),is_short=COALESCE(excluded.is_short,videos.is_short),raw_json=excluded.raw_json,refresh_requested_at=NULL
WHERE videos.source=?`,
		v.ID, v.Source, v.Title, v.PublishedAt, nullable(v.ScheduledAt), v.URL, nullable(v.ThumbnailURL), nullable(v.Kind), v.IsUpcoming, nullable(v.LiveStatus), now, duration, short, nullable(v.RawJSON), v.Source)
	if err != nil {
		return BambinoIngestResult{}, dbErr(err)
	}
	if affected, rowErr := written.RowsAffected(); rowErr != nil {
		return BambinoIngestResult{}, dbErr(rowErr)
	} else if affected != 1 {
		return BambinoIngestResult{}, ErrIngestConflict
	}

	if v.ThumbnailURL != "" {
		if err = replaceMediaLink(ctx, tx, v.Source, v.ThumbnailURL, "video", v.ID, "thumbnail", 0, now); err != nil {
			return BambinoIngestResult{}, dbErr(err)
		}
	}
	if err = bumpRevision(ctx, tx, now); err != nil {
		return BambinoIngestResult{}, dbErr(err)
	}
	revision, err := revisionTx(ctx, tx)
	if err != nil {
		return BambinoIngestResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return BambinoIngestResult{}, dbErr(err)
	}
	return BambinoIngestResult{Revision: revision}, nil
}

func (s *Store) canonicalXHandle(ctx context.Context, handle string) string {
	handle = strings.TrimPrefix(strings.TrimSpace(handle), "@")
	if handle == "" {
		return ""
	}
	accounts, err := s.getXAccounts(ctx)
	if err != nil {
		return handle
	}
	for _, account := range accounts {
		if strings.EqualFold(account, handle) {
			return account
		}
	}
	return handle
}

func revisionTx(ctx context.Context, tx *sql.Tx) (int64, error) {
	var value string
	err := tx.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key='dashboard_revision'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, dbErr(err)
	}
	revision, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, nil
	}
	return revision, nil
}

func ingestHashOf(raw string) string {
	if raw == "" {
		return ""
	}
	var doc ingestHashJSON
	if json.Unmarshal([]byte(raw), &doc) != nil {
		return ""
	}
	return doc.IngestHash
}

func nonEmptyURLs(urls []string) []string {
	out := make([]string, 0, len(urls))
	for _, url := range urls {
		if url = strings.TrimSpace(url); url != "" {
			out = append(out, url)
		}
	}
	return out
}
