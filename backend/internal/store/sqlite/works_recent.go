package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/yexca/kano-navi/backend/internal/works"
)

// Recent Works uploads (see docs/development/works-archive.md#uploads-after-the-collection-date)
// are the stored videos dated after the packaged list's updatedAt that the
// Works page lists on their own. They are derived on every read and never
// stored, so a release that moves updatedAt drops the earlier ones.

// Bounds of the recent Works uploads, newest first: the Works page reads at
// most WorksRecentLimit and the console reviews at most
// WorksRecentReportLimit; each read reports how many there are in all.
const (
	WorksRecentLimit       = 48
	WorksRecentReportLimit = 200
)

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
	// Total counts every listed and excluded video, beyond the items' bound.
	Total int `json:"total"`
}

// worksRecentRow is a video the recent Works uploads review, with what its
// cover reads.
type worksRecentRow struct {
	review      WorksRecentReview
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
func worksListedIDs(index works.Index) []string {
	ids := []string{}
	for _, upload := range index.Uploads {
		for _, video := range upload.Videos {
			ids = append(ids, video.ID)
		}
	}
	return ids
}

// worksRecentRows reads, newest first, at most limit of the videos that pass
// every rule of the recent Works uploads, and how many pass in all:
//
//  1. the Japan date of COALESCE(scheduled_at, published_at) is after
//     updatedAt;
//  2. public readers list it (publicVideoScope), nobody marked it private or
//     removed, and it is no longer upcoming;
//  3. it is not merged into another video;
//  4. its ID is not a video of the packaged list;
//  5. the operator's decision includes it, or, without one, its shown tag is
//     song, its form is upload or premiere and it is not a Short.
//
// With excluded, a video an operator's decision keeps out passes rule 5 too,
// for the console. SQLite applies every rule, so a read walks the date index
// only until limit videos pass, however many follow the list's date.
func (s *Store) worksRecentRows(ctx context.Context, index works.Index, now time.Time, excluded bool, limit int) ([]worksRecentRow, int, error) {
	after, ok := worksRecentAfter(index.UpdatedAt)
	if !ok {
		return []worksRecentRow{}, 0, nil
	}
	listed, err := json.Marshal(worksListedIDs(index))
	if err != nil {
		return nil, 0, err
	}
	decided := `v.manual_work=1`
	if excluded {
		decided = `v.manual_work IS NOT NULL`
	}
	from := ` FROM videos v WHERE ` + publicVideoScope + ` AND v.merged_into IS NULL
			AND julianday(COALESCE(v.scheduled_at,v.published_at))>=julianday(?1)
			AND COALESCE(v.manual_availability,v.availability,'') NOT IN ('private','removed')
			AND NOT COALESCE(CASE WHEN v.scheduled_at IS NOT NULL THEN julianday(v.scheduled_at)>=julianday(?2) ELSE v.is_upcoming<>0 END,0)
			AND v.id NOT IN (SELECT value FROM json_each(?3))
			AND (` + decided + ` OR v.manual_work IS NULL AND COALESCE(v.manual_kind,v.kind) IN ('upload','premiere')
				AND COALESCE(v.manual_is_short,v.is_short,0)=0 AND ` + videoTagSQL("v") + `='song')`
	args := []any{after, ISOTime(now), string(listed)}
	var total int
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*)`+from, args...).Scan(&total); err != nil {
		return nil, 0, dbErr(err)
	}
	out := []worksRecentRow{}
	if total == 0 || limit < 1 {
		return out, total, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT v.id,v.source,COALESCE(v.manual_title,v.title),v.published_at,v.scheduled_at,v.url,v.thumbnail_url,
		COALESCE(v.manual_thumbnail_media_id,''),COALESCE(v.manual_thumbnail_sha256,''),COALESCE(v.manual_kind,v.kind),COALESCE(v.manual_is_short,v.is_short),
		`+videoTagSQL("v")+`,COALESCE(v.manual_availability,v.availability),v.manual_work`+from+`
		ORDER BY julianday(COALESCE(v.scheduled_at,v.published_at)) DESC,v.id ASC LIMIT ?4`, append(args, limit)...)
	if err != nil {
		return nil, 0, dbErr(err)
	}
	defer rows.Close()
	for rows.Next() {
		var row worksRecentRow
		var published, scheduled, thumbnail, kind, tag, availability sql.NullString
		var short, work sql.NullBool
		if err = rows.Scan(&row.review.ID, &row.review.Source, &row.review.Title, &published, &scheduled, &row.review.URL, &thumbnail,
			&row.pinnedMedia, &row.pinnedHash, &kind, &short, &tag, &availability, &work); err != nil {
			return nil, 0, dbErr(err)
		}
		v := &row.review
		v.PublishedAt, v.ScheduledAt, v.Kind, v.Tag = nullString(published), nullString(scheduled), nullString(kind), nullString(tag)
		v.IsShort, v.Availability, v.Work = nullBool(short), nullString(availability), nullBool(work)
		if thumbnail.Valid && strings.TrimSpace(thumbnail.String) != "" {
			row.thumbnail = &thumbnail.String
		}
		switch {
		case v.Work == nil:
			v.Origin = ptr(WorksOriginAuto)
		case *v.Work:
			v.Origin = ptr(WorksOriginOperator)
		}
		out = append(out, row)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, dbErr(err)
	}
	return out, total, nil
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

// worksRecentUploads lists the newest WorksRecentLimit recent Works uploads
// with their covers, translations and merged alternates, and how many there
// are in all.
func (s *Store) worksRecentUploads(ctx context.Context, index works.Index, now time.Time) ([]WorksRecentUpload, int, error) {
	rows, total, err := s.worksRecentRows(ctx, index, now, false, WorksRecentLimit)
	if err != nil {
		return nil, 0, err
	}
	if err = s.resolveWorksRecentCovers(ctx, rows); err != nil {
		return nil, 0, err
	}
	refs := make([]*Video, 0, len(rows))
	for _, row := range rows {
		refs = append(refs, &Video{ID: row.review.ID, Source: row.review.Source, Title: row.review.Title})
	}
	if err = s.attachVideoTranslations(ctx, refs...); err != nil {
		return nil, 0, err
	}
	if err = s.attachVideoAlternates(ctx, refs...); err != nil {
		return nil, 0, err
	}
	out := make([]WorksRecentUpload, 0, len(rows))
	for i, row := range rows {
		out = append(out, WorksRecentUpload{
			ID: row.review.ID, Source: row.review.Source, Title: row.review.Title, URL: row.review.URL,
			PublishedAt: row.review.PublishedAt, ScheduledAt: row.review.ScheduledAt, Cover: row.cover,
			Origin: *row.review.Origin, Translations: refs[i].Translations, Alternates: refs[i].Alternates,
		})
	}
	return out, total, nil
}

// WorksRecentReport lists, newest first, at most WorksRecentReportLimit of
// the videos after the list's date that the Works page shows and those an
// operator's decision keeps out, with how many there are in all. It never
// writes.
func (s *Store) WorksRecentReport(ctx context.Context, index works.Index, now time.Time) (*WorksRecentReport, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	ctx = withReadMemo(ctx)
	rows, total, err := s.worksRecentRows(ctx, index, now, true, WorksRecentReportLimit)
	if err != nil {
		return nil, err
	}
	if err = s.resolveWorksRecentCovers(ctx, rows); err != nil {
		return nil, err
	}
	report := &WorksRecentReport{UpdatedAt: index.UpdatedAt, Items: make([]WorksRecentReview, 0, len(rows)), Total: total}
	for _, row := range rows {
		item := row.review
		if row.cover != nil {
			item.ThumbnailURL = &row.cover.URL
		}
		report.Items = append(report.Items, item)
	}
	return report, nil
}
