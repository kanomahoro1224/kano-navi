package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// A Bilibili upload's form is upload or stream (a live replay, shown as a
// stream archive). Bilibili's unsigned endpoints carry no live-replay flag,
// so the list read stores a new row as an unsettled upload and the form is
// settled by the first of:
//
//  1. the operator's manual_kind;
//  2. a rule (SettleBilibiliForms): a title naming a replay is a stream; an
//     upload of at least thirty minutes published within 48 hours after a
//     Bilibili broadcast ended, at least half that broadcast's length, is a
//     stream; an upload shorter than ten minutes is an upload;
//  3. the title inference, which answers the form with the content tag
//     (RecordBilibiliForm) and may answer uncertain.
//
// kind_source records who settled the stored kind (rule or llm); an
// operator's manual_kind reads as manual. kind_review marks an upload that
// waits for the operator because the model was uncertain or failed
// bilibiliFormMaxFailures times. An unsettled upload is not public
// (publicVideoScope), so a live replay never shows briefly as a work.

// Who settled a Bilibili upload's form.
const (
	FormSourceRule   = "rule"
	FormSourceLLM    = "llm"
	FormSourceManual = "manual"
)

// Why a Bilibili upload waits for the operator's form decision.
const (
	FormReviewUncertain = "uncertain"
	FormReviewFailed    = "failed"
)

// Bilibili forms and the model's undecided answer.
const (
	BilibiliFormUpload    = "upload"
	BilibiliFormStream    = "stream"
	BilibiliFormUncertain = "uncertain"
)

const (
	bilibiliFormMaxFailures = 3
	// bilibiliReplayMinSeconds and bilibiliShortUpload bound the length
	// rules; bilibiliReplayWindow is how long after a broadcast's end a
	// replay is expected.
	bilibiliReplayMinSeconds = 30 * 60
	bilibiliShortUpload      = 10 * 60
	bilibiliReplayWindow     = 48 * time.Hour
	// bilibiliFormDescription bounds the description start sent to a model.
	bilibiliFormDescription = 200
	maxBilibiliFormReviews  = 100
)

// bilibiliReplayTitle names a replay in a Bilibili title.
var bilibiliReplayTitle = regexp.MustCompile(`直播回放|录播|直播录像|回放`)

// bilibiliBroadcast is one Bilibili broadcast a replay may come from: a
// stream entry shown on Bilibili or a broadcast the live room recorded. End
// is zero when its length is unknown. YouTubeID is the YouTube stream archive
// linked to the same schedule entry, if any.
type bilibiliBroadcast struct {
	EventID, Title, YouTubeID string
	Start, End                time.Time
}

func (b bilibiliBroadcast) length() time.Duration { return b.End.Sub(b.Start) }

// BilibiliFormBroadcast is the broadcast a form rule considered, as the
// console shows it. Source is entry (a schedule entry) or room (the live
// room's record).
type BilibiliFormBroadcast struct {
	Source   string  `json:"source"`
	EventID  *string `json:"eventId"`
	Title    *string `json:"title"`
	StartsAt string  `json:"startsAt"`
	EndsAt   string  `json:"endsAt"`
}

func (b bilibiliBroadcast) view() *BilibiliFormBroadcast {
	out := &BilibiliFormBroadcast{Source: "room", StartsAt: workflowTime(b.Start), EndsAt: workflowTime(b.End)}
	if b.EventID != "" {
		out.Source, out.EventID = "entry", &b.EventID
	}
	if b.Title != "" {
		out.Title = &b.Title
	}
	return out
}

// replayBroadcast is the broadcast an upload published at published may
// replay: one that ended no more than 48 hours before it, the latest such
// end first. accept narrows the broadcasts considered.
func replayBroadcast(published time.Time, broadcasts []bilibiliBroadcast, accept func(bilibiliBroadcast) bool) (bilibiliBroadcast, bool) {
	var best bilibiliBroadcast
	found := false
	for _, b := range broadcasts {
		if b.End.IsZero() || !b.End.After(b.Start) || published.Before(b.End) || published.Sub(b.End) > bilibiliReplayWindow {
			continue
		}
		if accept != nil && !accept(b) {
			continue
		}
		if !found || b.End.After(best.End) || b.End.Equal(best.End) && b.EventID < best.EventID {
			best, found = b, true
		}
	}
	return best, found
}

// replayLength reports whether an upload of duration seconds is long enough
// to replay b: at least half its length.
func replayLength(duration int64, b bilibiliBroadcast) bool {
	return time.Duration(duration)*time.Second*2 >= b.length()
}

// bilibiliFormRule settles a form from the title, the length and the
// broadcasts, or reports false when no rule applies.
func bilibiliFormRule(title string, duration int64, published time.Time, hasPublished bool, broadcasts []bilibiliBroadcast) (string, bool) {
	if bilibiliReplayTitle.MatchString(title) {
		return BilibiliFormStream, true
	}
	if duration >= bilibiliReplayMinSeconds && hasPublished {
		if _, ok := replayBroadcast(published, broadcasts, func(b bilibiliBroadcast) bool { return replayLength(duration, b) }); ok {
			return BilibiliFormStream, true
		}
	}
	if duration > 0 && duration < bilibiliShortUpload {
		return BilibiliFormUpload, true
	}
	return "", false
}

// loadBilibiliBroadcasts reads the broadcasts a Bilibili replay may come
// from: stream entries shown on Bilibili (their own platforms, or an entry
// merged into them), not deleted, merged or cancelled by an operator, with
// the YouTube stream archive linked to the entry; and the live room's last
// two broadcasts. An entry's end is its own, else its YouTube archive's
// start plus length.
func loadBilibiliBroadcasts(ctx context.Context, q videoQuerier) ([]bilibiliBroadcast, error) {
	linked := `FROM videos v WHERE v.source='youtube' AND COALESCE(v.manual_kind,v.kind)='stream' AND ` + videoEventMatch("v") + ` ORDER BY v.id LIMIT 1`
	rows, err := q.QueryContext(ctx, `SELECT e.id,e.title,e.starts_at,COALESCE(e.ends_at,''),
		COALESCE((SELECT v.id `+linked+`),''),
		COALESCE((SELECT COALESCE(v.duration_seconds,0) `+linked+`),0)
		FROM events e WHERE e.deleted_at IS NULL AND e.merged_into IS NULL AND e.event_type='stream' AND e.starts_at IS NOT NULL
			AND COALESCE(e.cancellation_status,'')<>'manual_confirmed'
			AND (e.platform='bilibili' OR instr(COALESCE(e.platforms,''),'"bilibili"')>0
				OR EXISTS (SELECT 1 FROM events c WHERE c.merged_into=e.id AND c.deleted_at IS NULL
					AND (c.platform='bilibili' OR instr(COALESCE(c.platforms,''),'"bilibili"')>0)))
		ORDER BY e.id`)
	if err != nil {
		return nil, err
	}
	out := []bilibiliBroadcast{}
	for rows.Next() {
		var b bilibiliBroadcast
		var start, end string
		var duration int64
		if err = rows.Scan(&b.EventID, &b.Title, &start, &end, &b.YouTubeID, &duration); err != nil {
			rows.Close()
			return nil, err
		}
		var ok bool
		if b.Start, ok = parseMergeInstant(start); !ok {
			continue
		}
		if b.End, ok = parseMergeInstant(end); !ok && duration > 0 {
			b.End = b.Start.Add(time.Duration(duration) * time.Second)
		}
		out = append(out, b)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	rooms, err := q.QueryContext(ctx, `SELECT COALESCE(metadata_json,'{}') FROM sync_state WHERE source=? ORDER BY account_id`, bilibiliLiveSource)
	if err != nil {
		return nil, err
	}
	defer rooms.Close()
	for rooms.Next() {
		var meta string
		if err = rooms.Scan(&meta); err != nil {
			return nil, err
		}
		var data bilibiliLiveMetadata
		_ = json.Unmarshal([]byte(meta), &data)
		// The last check that saw the room live is the broadcast's end as far
		// as is known; a broadcast still on air has none yet.
		if start, end := parseInstant(data.LastBroadcast), parseInstant(data.LastLiveAt); !start.IsZero() && !end.IsZero() && data.EndedAt != "" {
			out = append(out, bilibiliBroadcast{Title: data.Title, Start: start, End: end})
		}
		if start, end := parseInstant(data.PreviousBroadcast), parseInstant(data.PreviousEndedAt); !start.IsZero() && !end.IsZero() {
			out = append(out, bilibiliBroadcast{Start: start, End: end})
		}
	}
	return out, rooms.Err()
}

// BilibiliFormCounts is the rule part of the translate step's result.
type BilibiliFormCounts struct {
	Considered int `json:"considered"`
	Settled    int `json:"settled"`
}

// SettleBilibiliForms applies the form rules to every Bilibili upload whose
// form nobody settled yet, merged or not, so a rule decides before the model
// is asked. An upload the rules leave open keeps waiting for the model or the
// operator; one in the Review queue leaves it when a rule settles it. The
// revision moves when an unmerged upload becomes public.
func (s *Store) SettleBilibiliForms(ctx context.Context, now time.Time) (BilibiliFormCounts, error) {
	counts := BilibiliFormCounts{}
	tx, err := s.eventTx(ctx)
	if err != nil {
		return counts, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,title,COALESCE(published_at,''),COALESCE(duration_seconds,0),merged_into IS NULL FROM videos
		WHERE source='bilibili' AND manual_kind IS NULL AND kind_source IS NULL ORDER BY id`)
	if err != nil {
		return counts, fmt.Errorf("%w: bilibili forms", ErrUnavailable)
	}
	type upload struct {
		id, title, published string
		duration             int64
		unmerged             bool
	}
	uploads := []upload{}
	for rows.Next() {
		var u upload
		if err = rows.Scan(&u.id, &u.title, &u.published, &u.duration, &u.unmerged); err != nil {
			rows.Close()
			return counts, fmt.Errorf("%w: bilibili forms", ErrUnavailable)
		}
		uploads = append(uploads, u)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return counts, fmt.Errorf("%w: bilibili forms", ErrUnavailable)
	}
	if len(uploads) == 0 {
		return counts, nil
	}
	broadcasts, err := loadBilibiliBroadcasts(ctx, tx)
	if err != nil {
		return counts, fmt.Errorf("%w: bilibili broadcasts", ErrUnavailable)
	}
	stamp, public := workflowTime(now), false
	for _, u := range uploads {
		counts.Considered++
		published, hasPublished := parseMergeInstant(u.published)
		form, ok := bilibiliFormRule(u.title, u.duration, published, hasPublished, broadcasts)
		if !ok {
			continue
		}
		result, err := tx.ExecContext(ctx, `UPDATE videos SET kind=?,kind_source=?,kind_review=NULL WHERE id=? AND manual_kind IS NULL AND kind_source IS NULL`, form, FormSourceRule, u.id)
		if err != nil {
			return counts, fmt.Errorf("%w: bilibili forms", ErrUnavailable)
		}
		if changed, _ := result.RowsAffected(); changed > 0 {
			counts.Settled++
			public = public || u.unmerged
		}
	}
	if public {
		if err = bumpRevision(ctx, tx, stamp); err != nil {
			return counts, fmt.Errorf("%w: revision", ErrUnavailable)
		}
	}
	if err = tx.Commit(); err != nil {
		return counts, fmt.Errorf("%w: commit", ErrUnavailable)
	}
	return counts, nil
}

// BilibiliFormCandidate is one unmerged Bilibili upload whose form the model
// is asked for, with the start of its description.
type BilibiliFormCandidate struct {
	ID, Title, Description string
	DurationSeconds        int64
}

// ListBilibiliFormCandidates returns up to limit unmerged Bilibili uploads
// nobody marked private or removed whose form neither the operator nor a rule
// settled and that are not waiting in the Review queue. Uploads never
// attempted come first, then the least recently attempted, each group newest
// first; a failed upload waits until its last attempt is no later than
// retryBefore.
func (s *Store) ListBilibiliFormCandidates(ctx context.Context, retryBefore time.Time, limit int) ([]BilibiliFormCandidate, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if limit < 1 {
		limit = 1
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,title,COALESCE(json_extract(CASE WHEN json_valid(raw_json) THEN raw_json END,'$.description'),''),COALESCE(duration_seconds,0) FROM videos
		WHERE source='bilibili' AND merged_into IS NULL AND manual_kind IS NULL AND kind_source IS NULL AND kind_review IS NULL
			AND COALESCE(manual_availability,availability,'') NOT IN('private','removed')
			AND (kind_attempted_at IS NULL OR julianday(kind_attempted_at)<=julianday(?))
		ORDER BY kind_attempted_at IS NOT NULL, julianday(kind_attempted_at) ASC, julianday(published_at) DESC, id ASC LIMIT ?`, ISOTime(retryBefore), limit)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	out := []BilibiliFormCandidate{}
	for rows.Next() {
		var item BilibiliFormCandidate
		if err = rows.Scan(&item.ID, &item.Title, &item.Description, &item.DurationSeconds); err != nil {
			return nil, dbErr(err)
		}
		item.Description = strings.TrimSpace(item.Description)
		if utf8.RuneCountInString(item.Description) > bilibiliFormDescription {
			item.Description = string([]rune(item.Description)[:bilibiliFormDescription])
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// RecordBilibiliForm stores the model's answer for one upload: upload or
// stream settles the form (kind_source llm), uncertain sends the upload to
// the Review queue. The tag is recorded as RecordVideoTag would, when the
// video has none yet. An operator's or a rule's form settled in the meantime
// wins and the call reports false. The revision moves when the upload becomes
// public.
func (s *Store) RecordBilibiliForm(ctx context.Context, id, form string, tag *string, now time.Time) (bool, error) {
	if form != BilibiliFormUpload && form != BilibiliFormStream && form != BilibiliFormUncertain {
		return false, ErrVideoInvalid
	}
	tx, err := s.eventTx(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	stamp := ISOTime(now)
	unsettled := ` WHERE id=? AND source='bilibili' AND manual_kind IS NULL AND kind_source IS NULL`
	var result sql.Result
	if form == BilibiliFormUncertain {
		result, err = tx.ExecContext(ctx, `UPDATE videos SET kind_review=?,kind_attempted_at=?`+unsettled, FormReviewUncertain, stamp, id)
	} else {
		result, err = tx.ExecContext(ctx, `UPDATE videos SET kind=?,kind_source=?,kind_review=NULL,kind_attempted_at=?`+unsettled, form, FormSourceLLM, stamp, id)
	}
	if err != nil {
		return false, fmt.Errorf("%w: bilibili form", ErrUnavailable)
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return false, nil
	}
	var value any
	if tag != nil && ValidVideoTag(*tag) {
		value = *tag
	}
	if _, err = tx.ExecContext(ctx, `UPDATE videos SET tag=?,tag_source=?,tag_attempted_at=?
		WHERE id=? AND manual_tag IS NULL AND tag IS NULL AND COALESCE(tag_source,'')=''`, value, TagSourceTitle, stamp, id); err != nil {
		return false, fmt.Errorf("%w: video tag", ErrUnavailable)
	}
	if form != BilibiliFormUncertain {
		if err = bumpRevision(ctx, tx, stamp); err != nil {
			return false, fmt.Errorf("%w: revision", ErrUnavailable)
		}
	}
	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("%w: commit", ErrUnavailable)
	}
	return true, nil
}

// RecordBilibiliFormFailure counts a form inference that produced no answer.
// The upload waits a day before it is asked again, and the third failure
// sends it to the Review queue.
func (s *Store) RecordBilibiliFormFailure(ctx context.Context, id string, now time.Time) error {
	if s == nil || !s.writable {
		return ErrReadOnly
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE videos SET kind_attempts=kind_attempts+1,kind_attempted_at=?,
		kind_review=CASE WHEN kind_attempts+1>=? THEN ? ELSE kind_review END
		WHERE id=? AND source='bilibili' AND manual_kind IS NULL AND kind_source IS NULL`,
		ISOTime(now), bilibiliFormMaxFailures, FormReviewFailed, id); err != nil {
		return fmt.Errorf("%w: bilibili form", ErrUnavailable)
	}
	return nil
}

// bilibiliFormReviewSQL selects the uploads (aliased v) waiting for the
// operator's form decision: unmerged, not marked private or removed, with no
// settled form and a review reason.
const bilibiliFormReviewSQL = `v.source='bilibili' AND v.merged_into IS NULL AND v.manual_kind IS NULL AND v.kind_source IS NULL AND v.kind_review IS NOT NULL
	AND COALESCE(v.manual_availability,v.availability,'') NOT IN('private','removed')`

// bilibiliFormDecisions counts the uploads in the Review queue's Bilibili
// form list.
func bilibiliFormDecisions(ctx context.Context, q rowQuerier) (int, error) {
	var count int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM videos v WHERE `+bilibiliFormReviewSQL).Scan(&count)
	return count, err
}

// BilibiliFormReview is one upload waiting for the operator's form decision:
// why (uncertain or failed) and the broadcast the rules considered, if any.
type BilibiliFormReview struct {
	Video     AdminVideo             `json:"video"`
	Review    string                 `json:"review"`
	Broadcast *BilibiliFormBroadcast `json:"broadcast"`
}

// BilibiliFormReviews lists the uploads waiting for the operator's form
// decision, newest first. The broadcast shown is the one the replay rule
// would read, whatever its length.
func (s *Store) BilibiliFormReviews(ctx context.Context) ([]BilibiliFormReview, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT v.id,v.kind_review,COALESCE(v.published_at,'') FROM videos v WHERE `+bilibiliFormReviewSQL+`
		ORDER BY julianday(v.published_at) DESC,v.id ASC LIMIT ?`, maxBilibiliFormReviews)
	if err != nil {
		return nil, dbErr(err)
	}
	type pending struct{ id, review, published string }
	list := []pending{}
	for rows.Next() {
		var p pending
		if err = rows.Scan(&p.id, &p.review, &p.published); err != nil {
			rows.Close()
			return nil, dbErr(err)
		}
		list = append(list, p)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, dbErr(err)
	}
	out := []BilibiliFormReview{}
	if len(list) == 0 {
		return out, nil
	}
	broadcasts, err := loadBilibiliBroadcasts(ctx, s.db)
	if err != nil {
		return nil, dbErr(err)
	}
	ids := make([]string, 0, len(list))
	for _, p := range list {
		ids = append(ids, p.id)
	}
	videos, err := s.adminVideosByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, p := range list {
		video, ok := videos[p.id]
		if !ok {
			continue
		}
		item := BilibiliFormReview{Video: video, Review: p.review}
		if published, ok := parseMergeInstant(p.published); ok {
			if b, found := replayBroadcast(published, broadcasts, nil); found {
				item.Broadcast = b.view()
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// adminFormSource is who settled a video's form as the console shows it:
// manual for an operator's correction, the stored kind_source of a Bilibili
// upload otherwise, nil for an unsettled upload or a YouTube video's platform
// facts.
func adminFormSource(source string, manualKind, kindSource *string) *string {
	switch {
	case manualKind != nil:
		return ptr(FormSourceManual)
	case source == "bilibili" && kindSource != nil && slices.Contains([]string{FormSourceRule, FormSourceLLM}, *kindSource):
		return kindSource
	}
	return nil
}
