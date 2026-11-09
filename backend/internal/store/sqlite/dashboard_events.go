package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

func (s *Store) queryDashboardEvents(ctx context.Context, from, to string, now time.Time) ([]DashboardEvent, error) {
	q := `SELECT ` + eventColumnsAs("o") + ` FROM events o WHERE deleted_at IS NULL AND merged_into IS NULL AND id IN (SELECT id FROM (SELECT id FROM events WHERE ` + publicEventSQL + ` AND ` + eventDaySQL + ` ORDER BY julianday(COALESCE(starts_at,starts_on)) ASC,id ASC LIMIT 100) UNION SELECT id FROM (SELECT id FROM events WHERE ` + eligibleEventSQL + ` AND (julianday(starts_at)>=julianday(?) OR (starts_at IS NULL AND starts_on>=?)) ORDER BY COALESCE(julianday(starts_at),julianday(starts_on||'T23:59:59.999+09:00')) ASC,id ASC LIMIT 30) UNION SELECT id FROM (SELECT id FROM events WHERE ` + eligibleEventSQL + ` AND (julianday(starts_at)<=julianday(?) AND (julianday(ends_at)>julianday(?) OR (ends_at IS NULL AND event_type IN ('stream','member') AND julianday(starts_at)>julianday(?,'-3 hours')))) AND (event_type IN ('stream','member') OR source='youtube') ORDER BY julianday(starts_at) ASC,id ASC LIMIT 30) UNION SELECT id FROM (SELECT id FROM events WHERE ` + eligibleEventSQL + ` AND ` + currentSaleSQL + ` ORDER BY COALESCE(ends_on,starts_on) ASC,id ASC LIMIT 10)) ORDER BY COALESCE(julianday(starts_at),julianday(starts_on||'T23:59:59.999+09:00')) ASC,id ASC`
	today := dateInJapan(now)
	soon := now.In(time.FixedZone("JST", 9*3600)).AddDate(0, 0, saleLookaheadDays).Format("2006-01-02")
	args := []any{from, to, from, to, now.UTC().Format(time.RFC3339Nano), today, now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano), soon, today}
	return s.scanEvents(ctx, q, args, now)
}

// eventDaySQL holds for an event shown on a Japan date in the inclusive range
// [?, ?] (bound twice): its start date, or the close date of a sale period.
const eventDaySQL = `(starts_on BETWEEN ? AND ? OR (event_type='sale' AND ends_on BETWEEN ? AND ?))`

// saleLookaheadDays is how far ahead the board's on-sale list looks for a
// sale that has not opened yet.
const saleLookaheadDays = 7

// currentSaleSQL holds for a sale period on sale on the Japan date today or
// opening by soon (bound as soon, today): it opens by soon, or is a deadline
// whose opening is not known, and it has not closed before today. The
// dashboard adds up to 10 of them, closing soonest first, so a period that
// opened weeks ago still reaches the board's on-sale list.
const currentSaleSQL = `event_type='sale' AND (starts_on<=? OR (ends_on=starts_on AND starts_at IS NULL)) AND COALESCE(ends_on,starts_on)>=?`

// publicEventSQL holds for an event of the unaliased events table that a
// public reader shows: not a tombstone, not a merged duplicate and not hidden
// by its posts.
var publicEventSQL = `deleted_at IS NULL AND merged_into IS NULL AND ` + eventShownSQL("events")

var eligibleEventSQL = publicEventSQL + ` AND lower(trim(COALESCE(status,''))) NOT LIKE 'cancel%' AND trim(COALESCE(status,'')) NOT IN ('取消','已取消','中止','キャンセル') AND cancellation_status NOT IN ('llm_suspected','manual_confirmed')`

func (s *Store) scanEvents(ctx context.Context, q string, args []any, now time.Time) ([]DashboardEvent, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	out := []DashboardEvent{}
	for rows.Next() {
		e, err := scanEvent(rows, now)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanEvent(row scanner, now time.Time) (DashboardEvent, error) {
	var e DashboardEvent
	var detail, sa, ea, eo, status, et, tag, urlv, cs, cr, ce, cat, platform, platforms sql.NullString
	var locked int64
	if err := row.Scan(&e.ID, &e.Source, &e.Title, &detail, &e.StartsOn, &sa, &ea, &eo, &e.Timezone, &e.TimePrecision, &status, &et, &tag, &urlv, &e.Provenance, &locked, &e.CancellationStatus, &cs, &cr, &ce, &cat, &e.UpdatedAt, &platform, &platforms); err != nil {
		return e, dbErr(err)
	}
	if platform.Valid {
		e.Platform = &platform.String
	}
	var room bool
	e.announced, room = readPlatforms(platforms, platform, urlv.String)
	e.Platforms = e.announced
	if room {
		// A live-room link is the same for every broadcast: the entry reads
		// as a Bilibili one without an identity link.
		e.roomURL, urlv = urlv.String, sql.NullString{}
		e.Platform = strPtr(PrimaryPlatform(e.announced))
	}
	if detail.Valid {
		e.Detail = &detail.String
	}
	if sa.Valid {
		e.StartsAt = &sa.String
	}
	if ea.Valid {
		e.EndsAt = &ea.String
	}
	if eo.Valid {
		e.EndsOn = &eo.String
	}
	if status.Valid {
		e.Status = &status.String
	}
	if et.Valid {
		e.EventType = &et.String
	}
	if tag.Valid {
		e.Tag = &tag.String
	}
	if urlv.Valid {
		e.URL = &urlv.String
	}
	if cs.Valid {
		e.CancellationSource = &cs.String
	}
	if cr.Valid {
		e.CancellationReason = &cr.String
	}
	if ce.Valid {
		e.CancellationEvidence = &ce.String
	}
	if cat.Valid {
		e.CancellationAt = &cat.String
	}
	e.ManualLocked = locked != 0
	e.IsUpcoming = eventUpcoming(e, now)
	if e.CancellationStatus == "" {
		e.CancellationStatus = "none"
	}
	return e, nil
}

func (s *Store) queryEventCounts(ctx context.Context, today string, now time.Time) (int64, int64, error) {
	var total, up int64
	err := s.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM events WHERE `+publicEventSQL+`),(SELECT COUNT(*) FROM events WHERE `+eligibleEventSQL+` AND (julianday(starts_at)>=julianday(?) OR (starts_at IS NULL AND starts_on>=?)))`, now.UTC().Format(time.RFC3339Nano), today).Scan(&total, &up)
	if err != nil {
		return 0, 0, dbErr(err)
	}
	return total, up, nil
}

// SaleEvent reports whether an event type names a sale period, the one type
// that stores a close date and is shown at its opening and its close.
func SaleEvent(eventType *string) bool {
	return eventType != nil && *eventType == "sale"
}

func eventUpcoming(e DashboardEvent, now time.Time) bool {
	if e.StartsAt != nil {
		t, err := time.Parse(time.RFC3339, *e.StartsAt)
		return err == nil && !t.Before(now)
	}
	return e.StartsOn >= dateInJapan(now)
}

func eventSortKey(e DashboardEvent) int64 {
	if e.StartsAt != nil {
		if t, err := time.Parse(time.RFC3339, *e.StartsAt); err == nil {
			return t.UnixNano()
		}
	}
	t, err := time.Parse("2006-01-02T15:04:05.999-07:00", e.StartsOn+"T23:59:59.999+09:00")
	if err != nil {
		return 0
	}
	return t.UnixNano()
}

func timeKey(v *string) int64 {
	if v == nil {
		return 1 << 62
	}
	t, err := time.Parse(time.RFC3339, *v)
	if err != nil {
		return 1 << 62
	}
	return t.UnixNano()
}

func containsEvent(es []DashboardEvent, id string) bool {
	for _, e := range es {
		if e.ID == id {
			return true
		}
	}
	return false
}

func cancelledStatus(v *string) bool {
	if v == nil {
		return false
	}
	x := strings.ToLower(strings.TrimSpace(*v))
	return x == "cancelled" || x == "canceled" || x == "cancel" || x == "取消" || x == "已取消" || x == "中止" || x == "キャンセル" || strings.HasPrefix(x, "cancel")
}

// Cancelled reports a definitive cancellation: a cancelled status or an
// operator's confirmation. A model's suspicion alone is not one.
func (e DashboardEvent) Cancelled() bool {
	return cancelledStatus(e.Status) || e.CancellationStatus == "manual_confirmed"
}
