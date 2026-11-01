package sqlite

// SchedulePeriod holds the SQL expressions of a schedule asset's covered
// dates, for an assets row under some alias. A manual correction wins; then
// a stored period; then the whole Monday-to-Sunday weeks around the live
// events extracted from the asset's source post (basis `events`).
type SchedulePeriod struct {
	// WeekStart is the Monday of the period's first week.
	WeekStart string
	Start     string
	End       string
	Basis     string
	// InferredStart and InferredEnd leave out the manual correction.
	InferredStart string
	InferredEnd   string
}

// scheduleEventDay is the earliest or latest start date of the live events
// extracted from the post that published the asset.
func scheduleEventDay(alias, aggregate string) string {
	return `(SELECT ` + aggregate + `(e.starts_on) FROM event_sources es JOIN events e ON e.id=es.event_id
	  WHERE es.source='x' AND es.deleted_at IS NULL AND e.deleted_at IS NULL
	    AND es.source_item_id IN (SELECT p.id FROM posts p WHERE p.url=` + alias + `.source_url))`
}

// SchedulePeriodSQL returns the period expressions of the assets row alias.
func SchedulePeriodSQL(alias string) SchedulePeriod {
	first := scheduleEventDay(alias, "MIN")
	monday := `date(` + first + `,'-6 days','weekday 1')`
	sunday := `date(` + scheduleEventDay(alias, "MAX") + `,'weekday 0')`
	inferredStart := `COALESCE(` + alias + `.period_start,` + monday + `)`
	inferredEnd := `COALESCE(` + alias + `.period_end,` + sunday + `)`
	return SchedulePeriod{
		WeekStart:     `COALESCE(date(` + alias + `.manual_period_start,'weekday 0','-6 days'),` + alias + `.week_start,` + monday + `)`,
		Start:         `COALESCE(` + alias + `.manual_period_start,` + inferredStart + `)`,
		End:           `COALESCE(` + alias + `.manual_period_end,` + inferredEnd + `)`,
		Basis:         `CASE WHEN ` + alias + `.manual_period_start IS NOT NULL THEN 'manual' WHEN ` + alias + `.period_start IS NOT NULL THEN ` + alias + `.period_basis WHEN ` + first + ` IS NOT NULL THEN 'events' ELSE ` + alias + `.period_basis END`,
		InferredStart: inferredStart,
		InferredEnd:   inferredEnd,
	}
}
