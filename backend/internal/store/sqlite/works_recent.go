package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/yexca/kano-navi/backend/internal/works"
)

// Recent Works uploads (see docs/development/works-archive.md#uploads-after-the-collection-date)
// are the stored videos dated after the packaged list's updatedAt that the
// Works page lists on their own. They are derived on every read and never
// stored, so a release that moves updatedAt drops the earlier ones.

// Origins of a recent Works upload.
const (
	// WorksOriginAuto is a video listed by its tag, form and format.
	WorksOriginAuto = "auto"
	// WorksOriginOperator is a video an operator's decision includes.
	WorksOriginOperator = "operator"
)

// WorksRecentUpload is one recent upload as the Works page lists it.
type WorksRecentUpload struct {
	ID, Source, Title, URL   string
	PublishedAt, ScheduledAt *string
	Cover                    *WorksCover
	Origin                   string
	Translations             TranslationSet
	Alternates               []VideoAlternate
}

// WorksRecentReview is a video the console reviews for the Works page: one
// listed now, or one an operator's decision keeps out.
type WorksRecentReview struct {
	ID           string  `json:"id"`
	Source       string  `json:"source"`
	Title        string  `json:"title"`
	URL          string  `json:"url"`
	PublishedAt  *string `json:"publishedAt"`
	ScheduledAt  *string `json:"scheduledAt"`
	Kind         *string `json:"kind"`
	Tag          *string `json:"tag"`
	IsShort      *bool   `json:"isShort"`
	Availability *string `json:"availability"`
	// Work is the operator's decision: true includes, false excludes, nil
	// leaves the video to its tag, form and format.
	Work *bool `json:"work"`
	// Origin is auto or operator for a listed video and nil for an excluded
	// one.
	Origin *string `json:"origin"`
	// ThumbnailURL is the local URL of the ready cached thumbnail, or nil.
	ThumbnailURL *string `json:"thumbnailUrl"`
}

// WorksRecentReport is the console's view of the recent Works uploads.
type WorksRecentReport struct {
	UpdatedAt string              `json:"updatedAt"`
	Items     []WorksRecentReview `json:"items"`
}

// worksRecentRow is a video that passes the date, availability, merge and
// list rules, with what the last rule reads.
type worksRecentRow struct {
	review      WorksRecentReview
	listed      bool
	cover       *WorksCover
	thumbnail   *string
	pinnedMedia string
	pinnedHash  string
}

// worksRecentAfter is the first instant after the Japan date updatedAt, or
// false when the list names no valid date.
func worksRecentAfter(updatedAt string) (string, bool) {
	if updatedAt == "" {
		return "", false
	}
	_, end, err := japanDateBounds(updatedAt, updatedAt)
	return end, err == nil
}

// worksListedIDs is every video ID of the packaged list, on any platform.
func worksListedIDs(index works.Index) map[string]bool {
	ids := map[string]bool{}
	for _, upload := range index.Uploads {
		for _, video := range upload.Videos {
			ids[video.ID] = true
		}
	}
	return ids
}

// worksRecentRows reads the videos that pass rules 1 to 4 of the recent Works
// uploads, newest first, and decides rule 5 for each:
//
//  1. the Japan date of COALESCE(scheduled_at, published_at) is after
//     updatedAt;
//  2. it is a YouTube video or a Bilibili upload, nobody marked it private or
//     removed, and it is no longer upcoming;
//  3. it is not merged into another video;
//  4. its ID is not a video of the packaged list;
//  5. the operator's decision includes it, or, without one, its shown tag is
//     song, its form is upload or premiere and it is not a Short.
//
// Rule 2 admits Bilibili uploads for this list alone; publicVideoScope keeps
// the board, the video archive and MCP on YouTube.
func (s *Store) worksRecentRows(ctx context.Context, index works.Index, now time.Time) ([]worksRecentRow, error) {
	after, ok := worksRecentAfter(index.UpdatedAt)
	if !ok {
		return []worksRecentRow{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT v.id,v.source,COALESCE(v.manual_title,v.title),v.published_at,v.scheduled_at,v.url,v.thumbnail_url,
		COALESCE(v.manual_thumbnail_media_id,''),COALESCE(v.manual_thumbnail_sha256,''),COALESCE(v.manual_kind,v.kind),COALESCE(v.manual_is_short,v.is_short),
		`+videoTagSQL("v")+`,COALESCE(v.manual_availability,v.availability),COALESCE(v.is_upcoming,0),v.manual_work
		FROM videos v WHERE v.source IN('youtube','bilibili') AND v.merged_into IS NULL
			AND julianday(COALESCE(v.scheduled_at,v.published_at))>=julianday(?)
		ORDER BY julianday(COALESCE(v.scheduled_at,v.published_at)) DESC,v.id ASC`, after)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	listed := worksListedIDs(index)
	out := []worksRecentRow{}
	for rows.Next() {
		var row worksRecentRow
		var published, scheduled, thumbnail, kind, tag, availability sql.NullString
		var short, work sql.NullBool
		var upcoming int
		if err = rows.Scan(&row.review.ID, &row.review.Source, &row.review.Title, &published, &scheduled, &row.review.URL, &thumbnail,
			&row.pinnedMedia, &row.pinnedHash, &kind, &short, &tag, &availability, &upcoming, &work); err != nil {
			return nil, dbErr(err)
		}
		v := &row.review
		v.PublishedAt, v.ScheduledAt, v.Kind, v.Tag = nullString(published), nullString(scheduled), nullString(kind), nullString(tag)
		v.IsShort, v.Availability, v.Work = nullBool(short), nullString(availability), nullBool(work)
		if thumbnail.Valid && strings.TrimSpace(thumbnail.String) != "" {
			row.thumbnail = &thumbnail.String
		}
		if gone(v.Availability) || listed[v.ID] || videoUpcoming(Video{ScheduledAt: v.ScheduledAt, IsUpcoming: upcoming != 0}, now) {
			continue
		}
		switch {
		case v.Work != nil && *v.Work:
			row.listed, v.Origin = true, ptr(WorksOriginOperator)
		case v.Work == nil && worksSong(v.Tag, v.Kind, v.IsShort):
			row.listed, v.Origin = true, ptr(WorksOriginAuto)
		case v.Work == nil:
			// Neither listed nor excluded: the console does not list it.
			continue
		}
		out = append(out, row)
	}
	if err = rows.Err(); err != nil {
		return nil, dbErr(err)
	}
	return out, nil
}

// worksSong is rule 5 without a decision: a song upload or premiere that is
// not a Short.
func worksSong(tag, kind *string, short *bool) bool {
	return tag != nil && *tag == "song" && kind != nil && (*kind == "upload" || *kind == "premiere") && (short == nil || !*short)
}

func ptr[T any](value T) *T { return &value }

// resolveWorksRecentCovers sets each row's ready cached thumbnail: an
// operator's pinned file while it is cached, else the platform thumbnail's.
func (s *Store) resolveWorksRecentCovers(ctx context.Context, rows []worksRecentRow) error {
	thumbnails := make([]*string, 0, len(rows))
	for _, row := range rows {
		thumbnails = append(thumbnails, row.thumbnail)
	}
	if err := s.prefetchMedia(ctx, thumbnails...); err != nil {
		return err
	}
	for i := range rows {
		row := &rows[i]
		if row.pinnedMedia != "" && safeHash(row.pinnedMedia) && safeHash(row.pinnedHash) {
			if url, servable := s.mediaVersionURL(ctx, row.pinnedMedia, row.pinnedHash); servable {
				row.cover = &WorksCover{URL: url}
				continue
			}
		}
		if row.thumbnail == nil {
			continue
		}
		ref, err := s.resolveMedia(ctx, row.thumbnail)
		if err != nil {
			return err
		}
		if ref != nil && ref.Status == "ready" && ref.PublicURL != nil {
			row.cover = &WorksCover{URL: *ref.PublicURL, Width: ref.Width, Height: ref.Height}
		}
	}
	return nil
}

// worksRecentUploads lists the recent Works uploads with their covers,
// translations and merged alternates.
func (s *Store) worksRecentUploads(ctx context.Context, index works.Index, now time.Time) ([]WorksRecentUpload, error) {
	all, err := s.worksRecentRows(ctx, index, now)
	if err != nil {
		return nil, err
	}
	rows := make([]worksRecentRow, 0, len(all))
	for _, row := range all {
		if row.listed {
			rows = append(rows, row)
		}
	}
	if err = s.resolveWorksRecentCovers(ctx, rows); err != nil {
		return nil, err
	}
	refs := make([]*Video, 0, len(rows))
	for _, row := range rows {
		refs = append(refs, &Video{ID: row.review.ID, Source: row.review.Source, Title: row.review.Title})
	}
	if err = s.attachVideoTranslations(ctx, refs...); err != nil {
		return nil, err
	}
	if err = s.attachVideoAlternates(ctx, refs...); err != nil {
		return nil, err
	}
	out := make([]WorksRecentUpload, 0, len(rows))
	for i, row := range rows {
		out = append(out, WorksRecentUpload{
			ID: row.review.ID, Source: row.review.Source, Title: row.review.Title, URL: row.review.URL,
			PublishedAt: row.review.PublishedAt, ScheduledAt: row.review.ScheduledAt, Cover: row.cover,
			Origin: *row.review.Origin, Translations: refs[i].Translations, Alternates: refs[i].Alternates,
		})
	}
	return out, nil
}

// WorksRecentReport lists, newest first, the videos after the list's date
// that the Works page shows and those an operator's decision keeps out. It
// never writes.
func (s *Store) WorksRecentReport(ctx context.Context, index works.Index, now time.Time) (*WorksRecentReport, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	ctx = withReadMemo(ctx)
	rows, err := s.worksRecentRows(ctx, index, now)
	if err != nil {
		return nil, err
	}
	if err = s.resolveWorksRecentCovers(ctx, rows); err != nil {
		return nil, err
	}
	report := &WorksRecentReport{UpdatedAt: index.UpdatedAt, Items: make([]WorksRecentReview, 0, len(rows))}
	for _, row := range rows {
		item := row.review
		if row.cover != nil {
			item.ThumbnailURL = &row.cover.URL
		}
		report.Items = append(report.Items, item)
	}
	return report, nil
}
