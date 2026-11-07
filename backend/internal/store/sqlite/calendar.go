package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"
)

var calendarDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

var ErrInvalidCalendarRange = errors.New("invalid calendar range")

// CalendarPage is the bounded public calendar read. Events are intentionally
// represented by the same store type used by the dashboard so both routes
// share the exact event fields, cancellation semantics and ordering rules.
type CalendarPage struct {
	Items          []DashboardEvent
	Page           int
	PageSize       int
	Total          int64
	TotalPages     int
	HasPrevious    bool
	HasNext        bool
	DayCounts      map[string]int64
	Revision       int64
	ScheduleImages []Asset
	Previous       *string
	Next           *string
}

func validCalendarDate(value string) bool {
	if !calendarDatePattern.MatchString(value) {
		return false
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

// maxCalendarDays covers a Monday-first month grid: six weeks of inclusive
// Japan dates. Events stay paginated at 100 per page.
const maxCalendarDays = 42

func validateCalendarRange(from, to string) error {
	if !validCalendarDate(from) || !validCalendarDate(to) {
		return ErrInvalidCalendarRange
	}
	start, _ := time.Parse("2006-01-02", from)
	end, _ := time.Parse("2006-01-02", to)
	if end.Before(start) || end.Sub(start) > (maxCalendarDays-1)*24*time.Hour {
		return ErrInvalidCalendarRange
	}
	return nil
}

func (s *Store) GetCalendarPage(ctx context.Context, from, to string, page, pageSize int) (*CalendarPage, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if err := validateCalendarRange(from, to); err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 1
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE deleted_at IS NULL AND merged_into IS NULL AND `+eventDaySQL, from, to, from, to).Scan(&total); err != nil {
		return nil, dbErr(err)
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * pageSize
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+eventColumnsAs("o")+`
		FROM events o
		WHERE deleted_at IS NULL AND merged_into IS NULL AND `+eventDaySQL+`
		ORDER BY COALESCE(julianday(starts_at), julianday(starts_on || 'T23:59:59.999+09:00')) ASC, id ASC
		LIMIT ? OFFSET ?`, from, to, from, to, pageSize, offset)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	now := time.Now()
	items := make([]DashboardEvent, 0, pageSize)
	for rows.Next() {
		event, scanErr := scanEvent(rows, now)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, dbErr(err)
	}
	if err := rows.Close(); err != nil {
		return nil, dbErr(err)
	}
	if err := s.attachLiveStatus(ctx, items, now); err != nil {
		return nil, err
	}
	if err := s.attachEventTranslations(ctx, items); err != nil {
		return nil, err
	}

	dayCounts := make(map[string]int64)
	// A sale period counts on its opening date and, when it closes on a later
	// date, on its close date too: the board shows it on both.
	countRows, err := s.db.QueryContext(ctx, `SELECT day, COUNT(*) FROM (
		SELECT starts_on AS day FROM events WHERE deleted_at IS NULL AND merged_into IS NULL AND starts_on BETWEEN ? AND ?
		UNION ALL SELECT ends_on FROM events WHERE deleted_at IS NULL AND merged_into IS NULL AND event_type='sale' AND ends_on>starts_on AND ends_on BETWEEN ? AND ?
	) GROUP BY day`, from, to, from, to)
	if err != nil {
		return nil, dbErr(err)
	}
	for countRows.Next() {
		var day string
		var count int64
		if err := countRows.Scan(&day, &count); err != nil {
			_ = countRows.Close()
			return nil, dbErr(err)
		}
		dayCounts[day] = count
	}
	if err := countRows.Err(); err != nil {
		_ = countRows.Close()
		return nil, dbErr(err)
	}
	_ = countRows.Close()

	var previous, next sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(day) FROM (SELECT starts_on AS day FROM events WHERE deleted_at IS NULL AND merged_into IS NULL AND starts_on < ?
		UNION ALL SELECT ends_on FROM events WHERE deleted_at IS NULL AND merged_into IS NULL AND event_type='sale' AND ends_on < ?)`, from, from).Scan(&previous); err != nil {
		return nil, dbErr(err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT MIN(day) FROM (SELECT starts_on AS day FROM events WHERE deleted_at IS NULL AND merged_into IS NULL AND starts_on > ?
		UNION ALL SELECT ends_on FROM events WHERE deleted_at IS NULL AND merged_into IS NULL AND event_type='sale' AND ends_on > ?)`, to, to).Scan(&next); err != nil {
		return nil, dbErr(err)
	}

	scheduleImages, err := s.queryScheduleAssets(ctx, from, to)
	if err != nil {
		return nil, err
	}
	revision, err := s.GetDashboardRevision(ctx)
	if err != nil {
		return nil, err
	}
	result := &CalendarPage{
		Items: items, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages,
		HasPrevious: page > 1, HasNext: page < totalPages, DayCounts: dayCounts,
		Revision: revision, ScheduleImages: scheduleImages,
	}
	if previous.Valid {
		result.Previous = &previous.String
	}
	if next.Valid {
		result.Next = &next.String
	}
	return result, nil
}

// attachLiveStatus gives each linked event the trusted broadcast state of its
// video, the same state the board reads, so the schedule and the board agree
// on what is on air.
func (s *Store) attachLiveStatus(ctx context.Context, events []DashboardEvent, now time.Time) error {
	identities := []any{}
	for _, event := range events {
		if event.URL != nil {
			identities = append(identities, streamIdentity(*event.URL))
		}
	}
	if len(identities) == 0 {
		return nil
	}
	probe, err := s.LiveProbeSettings(ctx)
	if err != nil {
		return err
	}
	cutoff := now.Add(-probe.Freshness())
	rows, err := s.db.QueryContext(ctx, `SELECT url,live_status,live_checked_at FROM videos WHERE live_status IS NOT NULL AND stream_identity(url) IN (?`+strings.Repeat(",?", len(identities)-1)+`)`, identities...)
	if err != nil {
		return dbErr(err)
	}
	defer rows.Close()
	live := map[string]*string{}
	for rows.Next() {
		var url string
		var status, checked sql.NullString
		if err := rows.Scan(&url, &status, &checked); err != nil {
			return dbErr(err)
		}
		if view := liveView(status, checked, cutoff); view != nil {
			live[streamIdentity(url)] = view
		}
	}
	if err := rows.Err(); err != nil {
		return dbErr(err)
	}
	for i := range events {
		if events[i].URL != nil {
			events[i].LiveStatus = live[streamIdentity(*events[i].URL)]
		}
	}
	_, err = s.attachBilibiliLive(ctx, events, now)
	return err
}
