package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// AdminBilibiliStatus is a pooled Bilibili dynamic as the console's External
// posts view lists it. It is never promoted: the forwarding post's quote card
// already shows it, so only its translations are the console's.
type AdminBilibiliStatus struct {
	ID          string  `json:"id"`
	AuthorMid   string  `json:"authorMid"`
	AuthorName  *string `json:"authorName"`
	Text        string  `json:"text"`
	PublishedAt *string `json:"publishedAt"`
	URL         string  `json:"url"`
	// Kind is video for an upload (a video page URL), else dynamic.
	Kind   string `json:"kind"`
	Images int    `json:"images"`
	// Reasons is why the pool holds it: forwarded.
	Reasons     []string `json:"reasons"`
	ForwardedAt *string  `json:"forwardedAt"`
	FirstSeenAt string   `json:"firstSeenAt"`
	// ForwardedBy lists the own posts that forward it, newest first, at most
	// five.
	ForwardedBy []AdminBilibiliForward `json:"forwardedBy"`
	// Shown is public when a shown forward shows its quote available, hidden
	// when every forward that shows it available is hidden or hides its
	// quote, and gone when no forward shows it available.
	Shown       string            `json:"shown"`
	Translation *AdminTranslation `json:"translation"`
}

// AdminBilibiliForward is an own post that forwards a pooled dynamic.
type AdminBilibiliForward struct {
	PostID         string `json:"postId"`
	Hidden         bool   `json:"hidden"`
	QuoteHidden    bool   `json:"quoteHidden"`
	QuoteAvailable bool   `json:"quoteAvailable"`
}

// AdminBilibiliStatusFilter selects pooled dynamics. Author is an exact mid
// or part of the display name, ignoring letter case; Kind is dynamic or
// video.
type AdminBilibiliStatusFilter struct {
	Page, PageSize int
	Author, Search string
	Kind           string
}

// Kinds of a pooled dynamic.
const (
	BilibiliStatusKindDynamic = "dynamic"
	BilibiliStatusKindVideo   = "video"
)

// bilibiliStatusVideoSQL holds for a pooled upload, whose URL is its video
// page.
const bilibiliStatusVideoSQL = `instr(s.url,'/video/BV')>0`

// ListBilibiliStatuses pages the pool, newest dynamic first. A dynamic that
// is a stored post (a forward of an own dynamic) is left out: its
// translations are the post's.
func (s *Store) ListBilibiliStatuses(ctx context.Context, f AdminBilibiliStatusFilter) (AdminPage[AdminBilibiliStatus], error) {
	page, pageSize := boundPage(f.Page, f.PageSize, 100, 25)
	clauses := []string{"NOT EXISTS (SELECT 1 FROM posts p WHERE p.id=s.id)"}
	args := []any{}
	if author := strings.TrimSpace(f.Author); author != "" {
		clauses = append(clauses, "(s.author_mid=? OR lower(COALESCE(s.author_name,'')) LIKE lower(?) ESCAPE '\\')")
		args = append(args, author, likePattern(author))
	}
	if search := strings.TrimSpace(f.Search); search != "" {
		clauses = append(clauses, "(s.text LIKE ? ESCAPE '\\' OR s.id=? OR s.id=?)")
		args = append(args, likePattern(search), search, "bilibili-"+search)
	}
	switch f.Kind {
	case BilibiliStatusKindVideo:
		clauses = append(clauses, bilibiliStatusVideoSQL)
	case BilibiliStatusKindDynamic:
		clauses = append(clauses, "NOT "+bilibiliStatusVideoSQL)
	}
	where := strings.Join(clauses, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM bilibili_statuses s WHERE `+where, args...).Scan(&total); err != nil {
		return AdminPage[AdminBilibiliStatus]{}, dbErr(err)
	}
	totalPages := pageCount(total, pageSize)
	page = safePage(page, totalPages)
	rows, err := s.db.QueryContext(ctx, `SELECT s.id,s.author_mid,s.author_name,s.text,s.published_at,s.url,`+bilibiliStatusVideoSQL+`,
		COALESCE(json_array_length(CASE WHEN json_valid(s.raw_json) THEN json_extract(s.raw_json,'$.mediaURLs') END),0),
		s.forwarded_at,s.first_seen_at
		FROM bilibili_statuses s WHERE `+where+`
		ORDER BY julianday(s.published_at) IS NULL,julianday(s.published_at) DESC,length(s.id) DESC,s.id DESC LIMIT ? OFFSET ?`,
		append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return AdminPage[AdminBilibiliStatus]{}, dbErr(err)
	}
	items := []AdminBilibiliStatus{}
	for rows.Next() {
		var item AdminBilibiliStatus
		var name, published, forwarded sql.NullString
		var video bool
		if err = rows.Scan(&item.ID, &item.AuthorMid, &name, &item.Text, &published, &item.URL, &video, &item.Images, &forwarded, &item.FirstSeenAt); err != nil {
			rows.Close()
			return AdminPage[AdminBilibiliStatus]{}, dbErr(err)
		}
		item.AuthorName, item.PublishedAt, item.ForwardedAt = nullString(name), nullString(published), nullString(forwarded)
		item.Kind = BilibiliStatusKindDynamic
		if video {
			item.Kind = BilibiliStatusKindVideo
		}
		item.Reasons = []string{"forwarded"}
		item.ForwardedBy = []AdminBilibiliForward{}
		item.Shown = "gone"
		items = append(items, item)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return AdminPage[AdminBilibiliStatus]{}, dbErr(err)
	}
	if err = s.bilibiliStatusForwards(ctx, items); err != nil {
		return AdminPage[AdminBilibiliStatus]{}, err
	}
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	translations, err := s.AdminTranslations(ctx, TranslationOwnerBilibiliStatus, ids)
	if err != nil {
		return AdminPage[AdminBilibiliStatus]{}, err
	}
	for i := range items {
		if translation, ok := translations[items[i].ID]; ok {
			items[i].Translation = &translation
		}
	}
	return AdminPage[AdminBilibiliStatus]{Items: items, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}, nil
}

// bilibiliStatusForwards fills, for a page of pooled dynamics, the own posts
// that forward each (newest first, at most five) and its shown state, which
// every forward decides. One scan of the posts' quotes serves the whole page.
func (s *Store) bilibiliStatusForwards(ctx context.Context, items []AdminBilibiliStatus) error {
	if len(items) == 0 {
		return nil
	}
	index := make(map[string]*AdminBilibiliStatus, len(items))
	ids := make([]string, 0, len(items))
	for i := range items {
		index[items[i].ID] = &items[i]
		ids = append(ids, items[i].ID)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT quoted, id, hidden, quote_hidden, available, public, shown FROM (
			SELECT quoted, id, hidden, quote_hidden, available, n,
				MAX(available AND NOT hidden AND NOT quote_hidden) OVER w AS public, MAX(available) OVER w AS shown
			FROM (
				SELECT json_extract(quote_json,'$.id') AS quoted, id,
					hidden_at IS NOT NULL AS hidden, quote_hidden_at IS NOT NULL AS quote_hidden,
					COALESCE(json_extract(quote_json,'$.unavailable'),0)=0 AS available,
					ROW_NUMBER() OVER (PARTITION BY json_extract(quote_json,'$.id') ORDER BY julianday(published_at) DESC,length(id) DESC,id DESC) AS n
				FROM posts WHERE source='bilibili' AND json_valid(quote_json)
					AND json_extract(quote_json,'$.id') IN (`+placeholders(len(ids))+`))
			WINDOW w AS (PARTITION BY quoted))
		WHERE n<=5 ORDER BY quoted,n`, idArgs(ids)...)
	if err != nil {
		return dbErr(err)
	}
	for rows.Next() {
		var quoted string
		var forward AdminBilibiliForward
		var public, available bool
		if err = rows.Scan(&quoted, &forward.PostID, &forward.Hidden, &forward.QuoteHidden, &forward.QuoteAvailable, &public, &available); err != nil {
			rows.Close()
			return dbErr(err)
		}
		item := index[quoted]
		if item == nil {
			continue
		}
		switch {
		case public:
			item.Shown = "public"
		case available:
			item.Shown = "hidden"
		}
		item.ForwardedBy = append(item.ForwardedBy, forward)
	}
	return dbErr(errors.Join(rows.Err(), rows.Close()))
}
