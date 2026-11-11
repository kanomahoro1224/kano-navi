package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	// The homepage keeps each video kind visible even when one kind has a
	// burst of uploads. These are source-pool limits; the shelf applies its
	// smaller visual preview limits afterwards.
	dashboardWorksLimit    = 12
	dashboardArchivesLimit = 12
	dashboardShortsLimit   = 6
	dashboardActiveLimit   = 30
)

func (s *Store) queryVideos(ctx context.Context, now, liveCutoff time.Time) ([]Video, error) {
	n := now.UTC().Format(time.RFC3339Nano)
	// A confirmed live broadcast stays listed past the start-time grace.
	streamKinds := "'stream'"
	videoTime := "julianday(COALESCE(scheduled_at,published_at))"
	base := "COALESCE(manual_kind,kind,'')"
	short := "COALESCE(manual_is_short,is_short,0)"
	q := `SELECT id,source,COALESCE(manual_title,title),published_at,scheduled_at,url,thumbnail_url,COALESCE(manual_kind,kind),is_upcoming,live_status,live_checked_at,duration_seconds,COALESCE(manual_is_short,is_short),` + videoTagSQL("videos") + `,COALESCE(manual_availability,availability) FROM videos WHERE id IN (
		SELECT id FROM (SELECT id FROM videos WHERE ` + publicVideoScope + ` AND ` + short + `<>1 AND ` + base + ` NOT IN (` + streamKinds + `)
			ORDER BY ` + videoTime + ` DESC,id ASC LIMIT ` + fmt.Sprint(dashboardWorksLimit) + `)
		UNION SELECT id FROM (SELECT id FROM videos WHERE ` + publicVideoScope + ` AND ` + short + `<>1 AND ` + base + ` IN (` + streamKinds + `)
			ORDER BY ` + videoTime + ` DESC,id ASC LIMIT ` + fmt.Sprint(dashboardArchivesLimit) + `)
		UNION SELECT id FROM (SELECT id FROM videos WHERE ` + publicVideoScope + ` AND ` + short + `=1
			ORDER BY ` + videoTime + ` DESC,id ASC LIMIT ` + fmt.Sprint(dashboardShortsLimit) + `)
		UNION SELECT id FROM (SELECT v.id FROM videos v WHERE v.source='youtube' AND (
			julianday(v.scheduled_at)>julianday(?,'-3 hours') OR v.scheduled_at IS NULL AND v.is_upcoming=1 OR
			v.live_status='live' AND julianday(v.live_checked_at)>julianday(?) OR EXISTS (SELECT 1 FROM events e WHERE e.deleted_at IS NULL AND julianday(e.ends_at)>julianday(?) AND
				(e.id='youtube-'||v.id OR (e.source='youtube' AND e.source_item_id=v.id) OR EXISTS (SELECT 1 FROM event_sources es WHERE es.event_id=e.id AND es.source='youtube' AND es.source_item_id=v.id) OR
				(e.url IS NOT NULL AND stream_identity(e.url)=stream_identity(v.url)))))
			AND NOT EXISTS (SELECT 1 FROM events e WHERE e.cancellation_status='manual_confirmed' AND
				(e.id='youtube-'||v.id OR (e.source='youtube' AND e.source_item_id=v.id) OR EXISTS (SELECT 1 FROM event_sources es WHERE es.event_id=e.id AND es.source='youtube' AND es.source_item_id=v.id)))
			AND NOT EXISTS (SELECT 1 FROM events e WHERE e.deleted_at IS NULL AND julianday(e.ends_at)<=julianday(?) AND
				(e.id='youtube-'||v.id OR (e.source='youtube' AND e.source_item_id=v.id) OR EXISTS (SELECT 1 FROM event_sources es WHERE es.event_id=e.id AND es.source='youtube' AND es.source_item_id=v.id) OR
				(e.url IS NOT NULL AND stream_identity(e.url)=stream_identity(v.url))))
			ORDER BY julianday(v.scheduled_at) ASC,v.id ASC LIMIT ` + fmt.Sprint(dashboardActiveLimit) + `)
	) ORDER BY ` + videoTime + ` DESC,id ASC`
	rows, err := s.db.QueryContext(ctx, q, n, liveCutoff.UTC().Format(time.RFC3339Nano), n, n, n)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	out := []Video{}
	for rows.Next() {
		var v Video
		var pub, sch, thumb, kind, liveStatus, liveChecked, tag, availability sql.NullString
		var upcoming int64
		var duration sql.NullInt64
		var short sql.NullBool
		if err := rows.Scan(&v.ID, &v.Source, &v.Title, &pub, &sch, &v.URL, &thumb, &kind, &upcoming, &liveStatus, &liveChecked, &duration, &short, &tag, &availability); err != nil {
			return nil, dbErr(err)
		}
		v.LiveStatus = liveView(liveStatus, liveChecked, liveCutoff)
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
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, dbErr(err)
	}
	return out, nil
}

func (s *Store) getVideo(ctx context.Context, id string, liveCutoff time.Time) (*Video, error) {
	var v Video
	var pub, sch, thumb, kind, liveStatus, liveChecked, tag, availability sql.NullString
	var up int64
	var duration sql.NullInt64
	var short sql.NullBool
	err := s.db.QueryRowContext(ctx, `SELECT id,source,COALESCE(manual_title,title),published_at,scheduled_at,url,thumbnail_url,COALESCE(manual_kind,kind),is_upcoming,live_status,live_checked_at,duration_seconds,COALESCE(manual_is_short,is_short),`+videoTagSQL("videos")+`,COALESCE(manual_availability,availability) FROM videos WHERE id=? AND `+publicVideoScope, id).Scan(&v.ID, &v.Source, &v.Title, &pub, &sch, &v.URL, &thumb, &kind, &up, &liveStatus, &liveChecked, &duration, &short, &tag, &availability)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, dbErr(err)
	}
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
	v.IsUpcoming = up != 0
	v.LiveStatus = liveView(liveStatus, liveChecked, liveCutoff)
	v.DurationSeconds = nullInt(duration)
	v.IsShort = nullBool(short)
	v.Tag = nullString(tag)
	v.Availability = nullString(availability)
	return &v, nil
}

func videoUpcoming(v Video, now time.Time) bool {
	if v.ScheduledAt != nil {
		t, err := time.Parse(time.RFC3339, *v.ScheduledAt)
		return err == nil && !t.Before(now)
	}
	return v.IsUpcoming
}

func containsVideo(vs []Video, id string) bool {
	for _, v := range vs {
		if v.ID == id {
			return true
		}
	}
	return false
}

// gone reports whether visitors can no longer watch a video: it is private
// or removed. Unknown availability is not gone.
func gone(availability *string) bool {
	return availability != nil && (*availability == "private" || *availability == "removed")
}
