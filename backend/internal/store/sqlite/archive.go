package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
)

// ErrInvalidAccount rejects a post archive filter that is not an X handle.
var ErrInvalidAccount = errors.New("invalid account")

// ErrInvalidPostKind rejects a post archive category filter outside the
// supported values (empty, all, post, reply, repost).
var ErrInvalidPostKind = errors.New("invalid post kind")

const (
	PostKindAll    = "all"
	PostKindPost   = "post"
	PostKindReply  = "reply"
	PostKindRepost = "repost"
)

func validPostKind(value string) bool {
	switch value {
	case "", PostKindAll, PostKindPost, PostKindReply, PostKindRepost:
		return true
	}
	return false
}

var accountPattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,30}$`)

// ErrInvalidVideoCategory rejects a video archive filter outside
// VideoCategories.
var ErrInvalidVideoCategory = errors.New("invalid video category")

// VideoCategories are the video archive's filter groups: the content tags
// and other. A video is not stored with one; videoCategorySQL derives it on
// each read (video_tags.go).
var VideoCategories = append(append([]string{}, VideoTags...), "other")

func validVideoCategory(value string) bool {
	for _, category := range VideoCategories {
		if category == value {
			return true
		}
	}
	return false
}

// maxArchivePageSize bounds one archive page; every item resolves its media.
const maxArchivePageSize = 50

// ArchivePage is a bounded public read of stored posts or videos whose Japan
// date falls inside an inclusive range, newest first. Items use the same store
// types as the dashboard, so both routes share one public field boundary.
type ArchivePage[T any] struct {
	Items      []T
	Page       int
	PageSize   int
	Total      int64
	TotalPages int
	HasNext    bool
	Revision   int64
	// Previous and Next are the nearest Japan dates outside the range that
	// hold an item, so an empty month can offer a jump.
	Previous *string
	Next     *string
	// AccountCounts counts the range's posts per lowercase handle, ignoring
	// the account filter, so the filter can show every account's total.
	AccountCounts  map[string]int64
	AccountAvatars map[string]*string
	// CategoryCounts counts the range's videos per category the same way.
	CategoryCounts map[string]int64
}

// archiveBounds turns an inclusive Japan-date range into the UTC instants
// [start, end) that julianday comparisons use.
func archiveBounds(from, to string) (string, string, error) {
	if err := validateCalendarRange(from, to); err != nil {
		return "", "", err
	}
	japan := time.FixedZone("JST", 9*60*60)
	start, _ := time.ParseInLocation("2006-01-02", from, japan)
	end, _ := time.ParseInLocation("2006-01-02", to, japan)
	return start.UTC().Format(time.RFC3339), end.AddDate(0, 0, 1).UTC().Format(time.RFC3339), nil
}

func archivePaging(total int64, page, pageSize int) (int, int, int) {
	if pageSize < 1 {
		pageSize = 1
	}
	if pageSize > maxArchivePageSize {
		pageSize = maxArchivePageSize
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages < 1 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}
	return page, pageSize, totalPages
}

// adjacentDates returns the Japan dates of the nearest items before start and
// at or after end. instant is the SQL expression of an item's time.
func (s *Store) adjacentDates(ctx context.Context, table, instant, filter string, args []any, start, end string) (*string, *string, error) {
	return s.adjacentDatesWithPrefix(ctx, "", table, instant, filter, args, start, end)
}

func (s *Store) adjacentDatesWithPrefix(ctx context.Context, prefix, table, instant, filter string, args []any, start, end string) (*string, *string, error) {
	var previous, next sql.NullString
	if start != "" {
		query := prefix + `SELECT date(MAX(` + instant + `) + 0.375) FROM ` + table + ` WHERE ` + instant + ` < julianday(?)` + filter
		if err := s.db.QueryRowContext(ctx, query, append([]any{start}, args...)...).Scan(&previous); err != nil {
			return nil, nil, dbErr(err)
		}
	}
	if end != "" {
		query := prefix + `SELECT date(MIN(` + instant + `) + 0.375) FROM ` + table + ` WHERE ` + instant + ` >= julianday(?)` + filter
		if err := s.db.QueryRowContext(ctx, query, append([]any{end}, args...)...).Scan(&next); err != nil {
			return nil, nil, dbErr(err)
		}
	}
	var before, after *string
	if previous.Valid {
		before = &previous.String
	}
	if next.Valid {
		after = &next.String
	}
	return before, after, nil
}

// timelineEnd turns a timeline's inclusive Japan date until into the UTC
// instant that ends it, or "" for an open end when until is empty.
func timelineEnd(until string) (string, error) {
	if until == "" {
		return "", nil
	}
	if !validCalendarDate(until) {
		return "", ErrInvalidCalendarRange
	}
	day, _ := time.ParseInLocation("2006-01-02", until, time.FixedZone("JST", 9*60*60))
	return day.AddDate(0, 0, 1).UTC().Format(time.RFC3339), nil
}

// GetPostArchive reads stored posts published in a Japan-date range, newest
// first, optionally for one account.
func (s *Store) GetPostArchive(ctx context.Context, from, to, account string, page, pageSize int) (*ArchivePage[Post], error) {
	return s.GetPostArchiveWithKind(ctx, from, to, account, "", page, pageSize)
}

// GetPostArchiveWithKind reads stored posts (and pooled reposts when kind is
// "all" or "repost") published in a Japan-date range, newest first.
func (s *Store) GetPostArchiveWithKind(ctx context.Context, from, to, account, kind string, page, pageSize int) (*ArchivePage[Post], error) {
	if s != nil && s.readDB != nil {
		return s.publicReader().GetPostArchiveWithKind(ctx, from, to, account, kind, page, pageSize)
	}
	start, end, err := archiveBounds(from, to)
	if err != nil {
		return nil, err
	}
	return s.postArchive(ctx, start, end, account, kind, page, pageSize)
}

// GetPostTimeline reads stored posts published on or before an inclusive
// Japan date, or every dated post when until is empty, newest first,
// optionally for one account. Its pages continue through every month, so a
// client can scroll back without a date range.
func (s *Store) GetPostTimeline(ctx context.Context, until, account string, page, pageSize int) (*ArchivePage[Post], error) {
	return s.GetPostTimelineWithKind(ctx, until, account, "", page, pageSize)
}

// GetPostTimelineWithKind reads stored posts (and pooled reposts when kind is
// "all" or "repost") published on or before an inclusive Japan date, newest
// first, optionally narrowed by account and category kind.
func (s *Store) GetPostTimelineWithKind(ctx context.Context, until, account, kind string, page, pageSize int) (*ArchivePage[Post], error) {
	if s != nil && s.readDB != nil {
		return s.publicReader().GetPostTimelineWithKind(ctx, until, account, kind, page, pageSize)
	}
	end, err := timelineEnd(until)
	if err != nil {
		return nil, err
	}
	return s.postArchive(ctx, "", end, account, kind, page, pageSize)
}

const (
	postShownBaseSQL = `p.deleted_at IS NULL AND (p.shown_at IS NOT NULL OR (p.hidden_at IS NULL AND p.upstream_deleted_at IS NULL))`
	ownRepostSQL     = `(p.source='x' AND (COALESCE(p.repost_json,'')<>'' OR COALESCE(p.upstream_kind,'')='repost'))`
	xRepostPublished = `COALESCE(
		CASE WHEN length(COALESCE(r.retweet_id,'')) BETWEEN 15 AND 22 AND r.retweet_id GLOB '[0-9]*'
			THEN strftime('%Y-%m-%dT%H:%M:%SZ', ((CAST(r.retweet_id AS INTEGER) >> 22) + 1288834974657) / 1000.0)
		END,
		NULLIF(s.published_at,''),
		NULLIF(orig.published_at,''),
		r.first_seen_at
	)`
	xRepostOrigURL = `COALESCE(NULLIF(s.url,''), orig.url, 'https://x.com/' || COALESCE(NULLIF(COALESCE(s.author_handle, r.author_handle, orig.account_handle),''), 'i') || '/status/' || r.status_id)`
)

// postArchivePostsScope returns the WHERE predicate (on aliased posts p)
// matching the posts-table slice of kind.
func postArchivePostsScope(kind string) string {
	switch kind {
	case PostKindPost:
		return `p.deleted_at IS NULL AND ((p.promoted_at IS NOT NULL AND (p.shown_at IS NOT NULL OR (p.hidden_at IS NULL AND p.upstream_deleted_at IS NULL AND NOT ` + postDemotedSQL("p") + `))) OR (p.promoted_at IS NULL AND (p.shown_at IS NOT NULL OR (p.hidden_at IS NULL AND p.upstream_deleted_at IS NULL AND NOT ` + ownRepostSQL + ` AND COALESCE(p.upstream_kind,'') NOT IN ('reply','foreign','repost','unlisted')))))`
	case PostKindReply:
		return `p.source='x' AND p.deleted_at IS NULL AND p.promoted_at IS NULL AND COALESCE(p.upstream_kind,'')='reply' AND (p.shown_at IS NOT NULL OR (p.hidden_at IS NULL AND p.upstream_deleted_at IS NULL))`
	case PostKindRepost:
		return postShownBaseSQL + ` AND ` + ownRepostSQL
	case PostKindAll:
		return `p.deleted_at IS NULL AND ((p.promoted_at IS NOT NULL AND (p.shown_at IS NOT NULL OR (p.hidden_at IS NULL AND p.upstream_deleted_at IS NULL AND NOT ` + postDemotedSQL("p") + `))) OR (p.promoted_at IS NULL AND (p.shown_at IS NOT NULL OR (p.hidden_at IS NULL AND p.upstream_deleted_at IS NULL AND COALESCE(p.upstream_kind,'') NOT IN ('foreign','unlisted')))))`
	default:
		return ``
	}
}

func postArchiveCTEPrefix(kind string) string {
	if kind == PostKindAll || kind == PostKindRepost {
		return `WITH existing_repost(h, sid) AS MATERIALIZED (
 SELECT lower(COALESCE(account_handle,'')), json_extract(repost_json,'$.id')
 FROM posts
 WHERE deleted_at IS NULL AND source='x' AND repost_json IS NOT NULL AND repost_json<>''
) `
	}
	return ``
}

func postArchiveUnionSelect(kind string) string {
	postsWhere := postArchivePostsScope(kind)
	postsSelect := `SELECT p.id,p.source,p.account_handle,p.type,p.label,p.text,p.published_at,p.url,p.likes,p.reposts,p.replies,p.media_url,p.media_alt,CASE WHEN p.quote_hidden_at IS NULL THEN p.quote_json END AS quote_json,p.promoted_at IS NOT NULL AS promoted,
 CASE WHEN p.source='bilibili' AND instr(p.raw_json,'` + bilisource.EmojiNode + `')>0 THEN p.raw_json END AS sticker_raw,p.repost_json,p.upstream_kind,
 CASE WHEN p.source='x' AND COALESCE(p.upstream_kind,'') IN ('reply','self_reply') AND json_valid(p.raw_json) THEN json_object('handle',COALESCE(json_extract(p.raw_json,'$.replying_to.screen_name'),json_extract(p.raw_json,'$.replyingTo'),json_extract(p.raw_json,'$.in_reply_to_screen_name')),'statusId',COALESCE(json_extract(p.raw_json,'$.replying_to.post'),json_extract(p.raw_json,'$.replying_to.status'),json_extract(p.raw_json,'$.replyingToID'),json_extract(p.raw_json,'$.replying_to_status')),'url',json_extract(p.raw_json,'$.replying_to.url')) END AS reply_to_json,
 p.id AS media_owner_id,NULL AS pool_raw_json
 FROM posts p WHERE ` + postsWhere

	if kind == PostKindPost || kind == PostKindReply {
		return `(` + postsSelect + `)`
	}
	repostSelect := `SELECT
 'repost-' || r.account_handle || '-' || r.status_id AS id,
 'x' AS source,
 r.account_handle AS account_handle,
 'daily' AS type,
 NULL AS label,
 COALESCE(NULLIF(s.text,''), orig.text, '') AS text,
 ` + xRepostPublished + ` AS published_at,
 CASE WHEN COALESCE(r.retweet_id,'')<>'' THEN 'https://x.com/' || r.account_handle || '/status/' || r.retweet_id ELSE ` + xRepostOrigURL + ` END AS url,
 COALESCE(orig.likes, 0) AS likes,
 COALESCE(orig.reposts, 0) AS reposts,
 COALESCE(orig.replies, 0) AS replies,
 COALESCE(orig.media_url, CASE WHEN json_valid(s.raw_json) THEN json_extract(s.raw_json, '$.mediaURLs[0]') END) AS media_url,
 orig.media_alt AS media_alt,
 CASE WHEN orig.quote_hidden_at IS NULL THEN orig.quote_json END AS quote_json,
 0 AS promoted,
 NULL AS sticker_raw,
 json_object(
   'id', r.status_id,
   'authorHandle', COALESCE(NULLIF(s.author_handle,''), NULLIF(r.author_handle,''), NULLIF(orig.account_handle,''), ''),
   'authorName', COALESCE(s.author_name, ''),
   'authorAvatar', COALESCE(json_extract(s.raw_json, '$.author.avatar_url'), ''),
   'publishedAt', COALESCE(NULLIF(s.published_at,''), NULLIF(orig.published_at,''), ''),
   'url', ` + xRepostOrigURL + `
 ) AS repost_json,
 'repost' AS upstream_kind,
 NULL AS reply_to_json,
 CASE WHEN orig.id IS NOT NULL THEN orig.id ELSE '' END AS media_owner_id,
 s.raw_json AS pool_raw_json
 FROM x_reposts r
 LEFT JOIN x_statuses s ON s.id = r.status_id
 LEFT JOIN posts orig ON orig.id = r.status_id AND orig.deleted_at IS NULL
 WHERE NOT EXISTS (SELECT 1 FROM existing_repost e WHERE e.h=r.account_handle AND e.sid=r.status_id)`
	return `(` + postsSelect + ` UNION ALL ` + repostSelect + `)`
}

// postArchive reads one page of shown posts in the UTC instants [start, end);
// an empty bound leaves that side open.
func (s *Store) postArchive(ctx context.Context, start, end, account, kind string, page, pageSize int) (*ArchivePage[Post], error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	ctx = withReadMemo(ctx)
	if account != "" && !accountPattern.MatchString(account) {
		return nil, ErrInvalidAccount
	}
	if !validPostKind(kind) {
		return nil, ErrInvalidPostKind
	}
	const instant = `julianday(published_at)`
	useUnion := kind != ""
	ctePrefix := postArchiveCTEPrefix(kind)
	var fromTable, baseCondition, selectCols string
	if !useUnion {
		fromTable = `posts`
		baseCondition = wallPostSQL
		selectCols = postColumns
	} else {
		fromTable = postArchiveUnionSelect(kind) + ` AS pa`
		baseCondition = `1=1`
		selectCols = `id,source,account_handle,type,label,text,published_at,url,likes,reposts,replies,media_url,media_alt,quote_json,promoted,sticker_raw,repost_json,upstream_kind,reply_to_json,media_owner_id,pool_raw_json`
	}
	conditions, bounds := []string{baseCondition, instant + ` IS NOT NULL`}, []any{}
	if start != "" {
		conditions, bounds = append(conditions, instant+` >= julianday(?)`), append(bounds, start)
	}
	if end != "" {
		conditions, bounds = append(conditions, instant+` < julianday(?)`), append(bounds, end)
	}
	inRange := ` WHERE ` + strings.Join(conditions, " AND ")
	filter, filterArgs := "", []any{}
	if account != "" {
		filter, filterArgs = ` AND lower(account_handle)=lower(?)`, []any{account}
	}
	args := append(append([]any{}, bounds...), filterArgs...)

	counts := map[string]int64{}
	countRows, err := s.db.QueryContext(ctx, ctePrefix+`SELECT lower(COALESCE(account_handle,'')), COUNT(*) FROM `+fromTable+inRange+` GROUP BY 1`, bounds...)
	if err != nil {
		return nil, dbErr(err)
	}
	for countRows.Next() {
		var handle string
		var count int64
		if err := countRows.Scan(&handle, &count); err != nil {
			_ = countRows.Close()
			return nil, dbErr(err)
		}
		if handle != "" {
			counts[handle] = count
		}
	}
	if err := countRows.Err(); err != nil {
		_ = countRows.Close()
		return nil, dbErr(err)
	}
	_ = countRows.Close()

	var total int64
	if err := s.db.QueryRowContext(ctx, ctePrefix+`SELECT COUNT(*) FROM `+fromTable+inRange+filter, args...).Scan(&total); err != nil {
		return nil, dbErr(err)
	}
	page, pageSize, totalPages := archivePaging(total, page, pageSize)
	rows, err := s.db.QueryContext(ctx, ctePrefix+`SELECT `+selectCols+` FROM `+fromTable+inRange+filter+` ORDER BY `+instant+` DESC,id ASC LIMIT ? OFFSET ?`, append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, dbErr(err)
	}
	postRows := make([]postRow, 0, pageSize)
	for rows.Next() {
		var r postRow
		if !useUnion {
			r, err = scanPostRow(rows)
		} else {
			var accountH, label, mu, ma, mediaOwner sql.NullString
			err = rows.Scan(&r.id, &r.source, &accountH, &r.typ, &label, &r.text, &r.publishedAt, &r.url, &r.likes, &r.reposts, &r.replies, &mu, &ma, &r.quoteJSON, &r.promoted, &r.stickerRaw, &r.repostJSON, &r.upstreamKind, &r.replyToJSON, &mediaOwner, &r.poolRawJSON)
			r.accountHandle, r.hasAccount = accountH.String, accountH.Valid
			r.label, r.hasLabel = label.String, label.Valid
			r.mediaURL, r.hasMediaURL = mu.String, mu.Valid
			r.mediaAlt, r.hasMediaAlt = ma.String, ma.Valid
			r.mediaOwnerID = mediaOwner.String
		}
		if err != nil {
			_ = rows.Close()
			return nil, dbErr(err)
		}
		postRows = append(postRows, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, dbErr(err)
	}
	// Media resolution runs its own queries; release this cursor first.
	_ = rows.Close()
	if err = s.prefetchPostRows(ctx, postRows); err != nil {
		return nil, err
	}
	accountCasing := s.archiveAccountCasing(ctx)
	items := make([]Post, 0, len(postRows))
	for _, row := range postRows {
		if row.hasAccount && strings.HasPrefix(row.id, "repost-") {
			if proper, ok := accountCasing[strings.ToLower(row.accountHandle)]; ok {
				row.accountHandle = proper
			}
		}
		post, mapErr := s.mapPost(ctx, row)
		if mapErr != nil {
			return nil, mapErr
		}
		items = append(items, post)
	}
	refs := make([]*Post, len(items))
	for i := range items {
		refs[i] = &items[i]
	}
	if err = s.attachPostTranslations(ctx, refs...); err != nil {
		return nil, err
	}

	// An open bound has no neighbor: julianday('') is NULL and matches nothing.
	var previous, next *string
	if start != "" || end != "" {
		adjFilter := filter
		if !useUnion {
			adjFilter = " AND " + wallPostSQL + filter
		}
		previous, next, err = s.adjacentDatesWithPrefix(ctx, ctePrefix, fromTable, instant, adjFilter, filterArgs, start, end)
		if err != nil {
			return nil, err
		}
	}
	revision, err := s.GetDashboardRevision(ctx)
	if err != nil {
		return nil, err
	}
	accountAvatars, err := s.AccountAvatars(ctx)
	if err != nil {
		return nil, err
	}
	return &ArchivePage[Post]{
		Items: items, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages,
		HasNext: page < totalPages, Revision: revision, Previous: previous, Next: next, AccountCounts: counts, AccountAvatars: accountAvatars,
	}, nil
}

func (s *Store) archiveAccountCasing(ctx context.Context) map[string]string {
	out := map[string]string{}
	accounts, err := s.GetPostArchiveAccounts(ctx)
	if err != nil {
		return out
	}
	for _, h := range accounts.X {
		out[strings.ToLower(h)] = h
	}
	return out
}

// ArchiveMonth is one Japan month that holds stored posts or videos: its
// total and its records per key, a lowercase handle for posts and a category
// for videos.
type ArchiveMonth struct {
	Month  string
	Total  int64
	Counts map[string]int64
}

// GetPostMonths lists every Japan month holding a shown post, oldest first,
// so the archive can offer a jump to any of them. Accounts are not filtered:
// the counts let the board narrow each month to the chosen account.
func (s *Store) GetPostMonths(ctx context.Context) ([]ArchiveMonth, int64, error) {
	return s.GetPostMonthsWithKind(ctx, "")
}

// GetPostMonthsWithKind lists every Japan month holding a shown post (or
// repost) matching kind, oldest first.
func (s *Store) GetPostMonthsWithKind(ctx context.Context, kind string) ([]ArchiveMonth, int64, error) {
	if s != nil && s.readDB != nil {
		return s.publicReader().GetPostMonthsWithKind(ctx, kind)
	}
	if !validPostKind(kind) {
		return nil, 0, ErrInvalidPostKind
	}
	if kind == "" {
		return s.archiveMonths(ctx, `SELECT strftime('%Y-%m', julianday(published_at) + 0.375) AS month,
	     lower(COALESCE(account_handle,'')), COUNT(*)
	   FROM posts WHERE `+wallPostSQL+` AND julianday(published_at) IS NOT NULL
	   GROUP BY 1, 2 ORDER BY 1, 2`)
	}
	source := postArchiveUnionSelect(kind) + ` AS pa`
	return s.archiveMonths(ctx, postArchiveCTEPrefix(kind)+`SELECT strftime('%Y-%m', julianday(published_at) + 0.375) AS month,
	     lower(COALESCE(account_handle,'')), COUNT(*)
	   FROM `+source+` WHERE julianday(published_at) IS NOT NULL
	   GROUP BY 1, 2 ORDER BY 1, 2`)
}

// GetVideoMonths lists every Japan month holding a public video, oldest
// first, with its videos per category, so the video timeline can offer a jump
// to any month and narrow the counts to the chosen category.
func (s *Store) GetVideoMonths(ctx context.Context) ([]ArchiveMonth, int64, error) {
	if s != nil && s.readDB != nil {
		return s.publicReader().GetVideoMonths(ctx)
	}
	return s.archiveMonths(ctx, `SELECT strftime('%Y-%m', `+videoInstant+` + 0.375) AS month,
	     `+videoCategorySQL+`, COUNT(*)
	   FROM videos WHERE `+publicVideoScope+` AND `+videoInstant+` IS NOT NULL
	   GROUP BY 1, 2 ORDER BY 1, 2`)
}

// archiveMonths folds (month, key, count) rows into months; an empty key adds
// to the total only.
func (s *Store) archiveMonths(ctx context.Context, query string) ([]ArchiveMonth, int64, error) {
	if s == nil || s.db == nil {
		return nil, 0, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, 0, dbErr(err)
	}
	months := []ArchiveMonth{}
	for rows.Next() {
		var month, key string
		var count int64
		if err := rows.Scan(&month, &key, &count); err != nil {
			_ = rows.Close()
			return nil, 0, dbErr(err)
		}
		if n := len(months); n == 0 || months[n-1].Month != month {
			months = append(months, ArchiveMonth{Month: month, Counts: map[string]int64{}})
		}
		current := &months[len(months)-1]
		current.Total += count
		if key != "" {
			current.Counts[key] = count
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, 0, dbErr(err)
	}
	_ = rows.Close()
	revision, err := s.GetDashboardRevision(ctx)
	if err != nil {
		return nil, 0, err
	}
	return months, revision, nil
}

// videoInstant is the SQL time of a video: its scheduled start, else its
// publication.
const videoInstant = `julianday(COALESCE(scheduled_at,published_at))`

// GetVideoArchive reads stored videos whose scheduled or published time falls
// in a Japan-date range, newest first, optionally of one category.
func (s *Store) GetVideoArchive(ctx context.Context, from, to, category string, page, pageSize int, now time.Time) (*ArchivePage[Video], error) {
	if s != nil && s.readDB != nil {
		return s.publicReader().GetVideoArchive(ctx, from, to, category, page, pageSize, now)
	}
	start, end, err := archiveBounds(from, to)
	if err != nil {
		return nil, err
	}
	return s.videoArchive(ctx, start, end, category, page, pageSize, now)
}

// GetVideoTimeline reads stored videos whose scheduled or published time
// falls on or before an inclusive Japan date, or every dated video when until
// is empty, newest first, optionally of one category. Its pages continue
// through every month, so the board can scroll back without a date range.
func (s *Store) GetVideoTimeline(ctx context.Context, until, category string, page, pageSize int, now time.Time) (*ArchivePage[Video], error) {
	if s != nil && s.readDB != nil {
		return s.publicReader().GetVideoTimeline(ctx, until, category, page, pageSize, now)
	}
	end, err := timelineEnd(until)
	if err != nil {
		return nil, err
	}
	return s.videoArchive(ctx, "", end, category, page, pageSize, now)
}

// videoArchive reads one page of videos in the UTC instants [start, end); an
// empty bound leaves that side open.
func (s *Store) videoArchive(ctx context.Context, start, end, category string, page, pageSize int, now time.Time) (*ArchivePage[Video], error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	ctx = withReadMemo(ctx)
	if category != "" && !validVideoCategory(category) {
		return nil, ErrInvalidVideoCategory
	}
	const instant = videoInstant
	conditions, bounds := []string{instant + ` IS NOT NULL`}, []any{}
	if start != "" {
		conditions, bounds = append(conditions, instant+` >= julianday(?)`), append(bounds, start)
	}
	if end != "" {
		conditions, bounds = append(conditions, instant+` < julianday(?)`), append(bounds, end)
	}
	inRange := ` WHERE ` + strings.Join(append(conditions, publicVideoScope), " AND ")
	filter, filterArgs := "", []any{}
	if category != "" {
		filter, filterArgs = ` AND `+videoCategorySQL+`=?`, []any{category}
	}
	args := append(append([]any{}, bounds...), filterArgs...)

	counts := map[string]int64{}
	countRows, err := s.db.QueryContext(ctx, `SELECT `+videoCategorySQL+`, COUNT(*) FROM videos`+inRange+` GROUP BY 1`, bounds...)
	if err != nil {
		return nil, dbErr(err)
	}
	for countRows.Next() {
		var name string
		var count int64
		if err := countRows.Scan(&name, &count); err != nil {
			_ = countRows.Close()
			return nil, dbErr(err)
		}
		counts[name] = count
	}
	if err := countRows.Err(); err != nil {
		_ = countRows.Close()
		return nil, dbErr(err)
	}
	_ = countRows.Close()

	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM videos`+inRange+filter, args...).Scan(&total); err != nil {
		return nil, dbErr(err)
	}
	page, pageSize, totalPages := archivePaging(total, page, pageSize)
	probe, err := s.LiveProbeSettings(ctx)
	if err != nil {
		return nil, err
	}
	liveCutoff := now.Add(-probe.Freshness())
	rows, err := s.db.QueryContext(ctx, `SELECT id,source,COALESCE(manual_title,title),published_at,scheduled_at,url,thumbnail_url,COALESCE(manual_kind,kind),is_upcoming,live_status,live_checked_at,duration_seconds,COALESCE(manual_is_short,is_short),`+videoTagSQL("videos")+`,COALESCE(manual_availability,availability) FROM videos`+inRange+filter+` ORDER BY `+instant+` DESC,id ASC LIMIT ? OFFSET ?`, append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, dbErr(err)
	}
	items := make([]Video, 0, pageSize)
	for rows.Next() {
		var v Video
		var pub, sch, thumb, kind, liveStatus, liveChecked, tag, availability sql.NullString
		var upcoming int64
		var duration sql.NullInt64
		var short sql.NullBool
		if err := rows.Scan(&v.ID, &v.Source, &v.Title, &pub, &sch, &v.URL, &thumb, &kind, &upcoming, &liveStatus, &liveChecked, &duration, &short, &tag, &availability); err != nil {
			_ = rows.Close()
			return nil, dbErr(err)
		}
		v.DurationSeconds = nullInt(duration)
		v.IsShort = nullBool(short)
		v.Tag = nullString(tag)
		v.Availability = nullString(availability)
		if pub.Valid {
			v.PublishedAt = &pub.String
		}
		if sch.Valid {
			v.ScheduledAt = &sch.String
		}
		if thumb.Valid {
			v.ThumbnailURL = &thumb.String
		}
		if kind.Valid {
			v.Kind = &kind.String
		}
		v.IsUpcoming = upcoming != 0
		v.LiveStatus = liveView(liveStatus, liveChecked, liveCutoff)
		items = append(items, v)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, dbErr(err)
	}
	_ = rows.Close()
	if err = s.resolveVideoThumbnails(ctx, items); err != nil {
		return nil, err
	}
	cancelled, err := s.cancelledVideos(ctx, items)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].IsUpcoming = videoUpcoming(items[i], now)
		items[i].IsCancelled = cancelled[items[i].ID]
		applyVideoView(&items[i], now)
	}

	if err = s.pinVideoThumbnails(ctx, items); err != nil {
		return nil, err
	}
	refs := make([]*Video, len(items))
	for i := range items {
		refs[i] = &items[i]
	}
	if err = s.attachVideoTranslations(ctx, refs...); err != nil {
		return nil, err
	}
	if err = s.attachVideoAlternates(ctx, refs...); err != nil {
		return nil, err
	}
	// An open bound has no neighbor: julianday('') is NULL and matches nothing.
	previous, next, err := s.adjacentDates(ctx, "videos", instant, " AND "+publicVideoScope+filter, filterArgs, start, end)
	if err != nil {
		return nil, err
	}
	revision, err := s.GetDashboardRevision(ctx)
	if err != nil {
		return nil, err
	}
	return &ArchivePage[Video]{
		Items: items, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages,
		HasNext: page < totalPages, Revision: revision, Previous: previous, Next: next, CategoryCounts: counts,
	}, nil
}
