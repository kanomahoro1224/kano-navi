package sqlite

import "strconv"

// What needs an operator's look. One rule serves the console's calendar, its
// Review queue and the overview counts, so they always agree.

// UncertainConfidence is the model confidence below which an automatic entry
// is an uncertain addition.
const UncertainConfidence = 0.7

// Attention kinds of an event.
const (
	AttentionSuspected = "suspected"
	AttentionUnlisted  = "unlisted"
	AttentionUncertain = "uncertain"
)

// Reasons an addition is uncertain.
const (
	ReasonLowConfidence = "lowConfidence"
	ReasonTentative     = "tentative"
)

// EventReview is why an entry needs a look: the model suspects its
// cancellation, its posts no longer list it, or it is an uncertain addition
// (Reasons say why).
type EventReview struct {
	Kind    string   `json:"kind"`
	Reasons []string `json:"reasons"`
}

// todaySQL is today's Japan date as SQL: today when it is a date, otherwise
// the database clock's.
func todaySQL(today string) string {
	if adminDateRE.MatchString(today) {
		return "'" + today + "'"
	}
	return "date('now','+9 hours')"
}

// EventAttentionSQL is the attention kind of the events row alias, or NULL:
// a live suspected cancellation; an unlisted entry, an unconfirmed automatic
// one whose live announcing evidence all comes from posts read again as no
// schedule (event_sources.unlisted_at); or an uncertain addition, which is an
// unconfirmed automatic entry from today on with a model confidence below
// UncertainConfidence or a tentative status. Deleted and merged entries need
// no look.
func EventAttentionSQL(alias, today string) string {
	return `CASE WHEN ` + alias + `.deleted_at IS NOT NULL OR ` + alias + `.merged_into IS NOT NULL THEN NULL
	 WHEN ` + alias + `.cancellation_status='llm_suspected' THEN '` + AttentionSuspected + `'
	 WHEN ` + alias + `.provenance='automatic' AND ` + alias + `.manual_locked=0
	   AND EXISTS(SELECT 1 FROM event_sources u WHERE u.event_id=` + alias + `.id AND u.deleted_at IS NULL AND u.unlisted_at IS NOT NULL)
	   AND NOT EXISTS(SELECT 1 FROM event_sources l WHERE l.event_id=` + alias + `.id AND l.deleted_at IS NULL AND l.unlisted_at IS NULL AND l.source<>'bilibili-video') THEN '` + AttentionUnlisted + `'
	 WHEN ` + alias + `.provenance='automatic' AND ` + alias + `.manual_locked=0 AND ` + alias + `.starts_on>=` + todaySQL(today) + `
	   AND (` + alias + `.confidence<` + strconv.FormatFloat(UncertainConfidence, 'f', -1, 64) + ` OR ` + alias + `.status='tentative') THEN '` + AttentionUncertain + `'
	 END`
}

// ScheduleImagePendingSQL holds the schedule assets row a, joined to its
// review row r, that wait for a decision: an image whose operator's label
// was on an image the asset no longer shows (previous_manual_status), or an
// image the operator has not labelled whose verdict the model has not
// settled or whose latest check failed or was skipped (check_status). A
// record without an image URL has nothing to check.
func ScheduleImagePendingSQL(a, r string) string {
	return a + `.kind='schedule' AND TRIM(` + a + `.url)<>'' AND ` + LiveScheduleAssetSQL(a) + ` AND (` + r + `.previous_manual_status IS NOT NULL
	 OR COALESCE(` + r + `.manual_status,'unreviewed')='unreviewed'
	 AND (COALESCE(` + r + `.llm_status,'pending') IN ('pending','uncertain','failed') OR ` + r + `.check_status IN ('failed','skipped')))`
}

// eventReview completes the attention kind read with EventAttentionSQL.
func eventReview(kind string, e AdminEvent) *EventReview {
	switch kind {
	case AttentionSuspected, AttentionUnlisted:
		return &EventReview{Kind: kind, Reasons: []string{}}
	case AttentionUncertain:
		review := &EventReview{Kind: kind, Reasons: []string{}}
		if e.Confidence != nil && *e.Confidence < UncertainConfidence {
			review.Reasons = append(review.Reasons, ReasonLowConfidence)
		}
		if e.Status != nil && *e.Status == "tentative" {
			review.Reasons = append(review.Reasons, ReasonTentative)
		}
		return review
	}
	return nil
}
