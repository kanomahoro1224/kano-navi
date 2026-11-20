package schedule

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// applyExtraction writes one accepted model result in a single transaction:
// cancellation targets are resolved first, the post's automatic events are
// replaced, and the cancellation overlay is applied last.
func (e *Extractor) applyExtraction(ctx context.Context, source, postID string, extractionID int64, result normalizedExtraction) error {
	cancellations := []normalizedEvent{}
	scheduled := []normalizedEvent{}
	for _, event := range result.events {
		if !event.cancellationSuspected {
			scheduled = append(scheduled, event)
		}
		if result.action == "cancel" || event.cancellationSuspected {
			cancellations = append(cancellations, event)
		}
	}
	scheduled = mergeIdenticalEntries(scheduled)
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	targets, err := findCancellationTargets(ctx, tx, cancellations)
	if err != nil {
		return err
	}
	// A post read again as no schedule keeps its events for an operator's
	// decision.
	if result.classification == "not_schedule" {
		if err = e.markUnlisted(ctx, tx, source, postID); err != nil {
			return err
		}
	}
	// Cancellation evidence must not retire a source's existing schedules.
	if result.classification == "schedule" && result.action != "cancel" && (len(scheduled) > 0 || len(cancellations) == 0) {
		if err = e.replaceAutomaticEvents(ctx, tx, source, postID, extractionID, scheduled, targets); err != nil {
			return err
		}
	}
	if len(cancellations) > 0 || result.action == "cancel" {
		reason := result.reason
		if reason == "" {
			reason = result.evidence
		}
		evidence := result.evidence
		if evidence == "" {
			parts := make([]string, len(cancellations))
			for i, event := range cancellations {
				parts[i] = event.evidence
			}
			evidence = strings.Join(parts, "; ")
		}
		if err = e.applyCancellations(ctx, tx, postID, reason, evidence, result.confidence, targets); err != nil {
			return err
		}
	}
	// The post's events are public, so open boards follow the change.
	if err = sqlite.BumpRevisionTx(ctx, tx, e.nowISO()); err != nil {
		return err
	}
	return tx.Commit()
}

var cancellationTitleNoise = regexp.MustCompile(jsSpaceClass + `|[「」『』【】()（）［］\[\]・:：,.，。!?！？]`)

func normalizedCancellationTitle(value string) string {
	return cancellationTitleNoise.ReplaceAllString(jsToLower(value), "")
}

// findCancellationTargets resolves each cancellation candidate to at most one
// active event: same Japan date, same instant when both carry one, then an
// exact title match or the only event of that day. Ambiguity selects nothing.
func findCancellationTargets(ctx context.Context, q queryer, candidates []normalizedEvent) ([]string, error) {
	type activeEvent struct {
		id       string
		title    string
		startsOn string
		startsAt string
	}
	rows, err := q.QueryContext(ctx, `SELECT id, title, starts_on, starts_at FROM events WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	active := []activeEvent{}
	for rows.Next() {
		var event activeEvent
		var title, startsOn, startsAt sql.NullString
		if err = rows.Scan(&event.id, &title, &startsOn, &startsAt); err != nil {
			rows.Close()
			return nil, err
		}
		event.title, event.startsOn, event.startsAt = title.String, startsOn.String, startsAt.String
		active = append(active, event)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	targets := []string{}
	for _, candidate := range candidates {
		title := normalizedCancellationTitle(candidate.title)
		var instant int64
		hasInstant := false
		if candidate.startsAt != nil {
			if parsed, ok := parseInstant(*candidate.startsAt); ok {
				instant, hasInstant = parsed.UnixMilli(), true
			}
		}
		sameDate := []activeEvent{}
		for _, event := range active {
			if event.startsOn != candidate.startsOn {
				continue
			}
			// Stored times mix `Z` and `+09:00` offsets, so compare instants.
			parsed, ok := parseInstant(event.startsAt)
			if hasInstant && ok && parsed.UnixMilli() != instant {
				continue
			}
			sameDate = append(sameDate, event)
		}
		if len(sameDate) == 0 {
			continue
		}
		exact := []activeEvent{}
		if title != "" {
			for _, event := range sameDate {
				if normalizedCancellationTitle(event.title) == title {
					exact = append(exact, event)
				}
			}
		}
		switch {
		case len(exact) == 1:
			targets = append(targets, exact[0].id)
		case len(sameDate) == 1:
			targets = append(targets, sameDate[0].id)
		}
	}
	return targets, nil
}

const upsertEventSQL = `INSERT INTO events (
		id, source, source_item_id, source_key, title, detail, starts_on, starts_at,
		ends_at, ends_on, timezone, time_precision, status, event_type, tag, tag_source, url, is_upcoming,
		provenance, manual_locked, confidence, extraction_id,
		cancellation_status, cancellation_source, cancellation_reason,
		cancellation_evidence, cancellation_source_item_id,
		cancellation_confidence, cancellation_at, deleted_at,
		created_at, updated_at, raw_json, platform, platforms, platforms_inferred
	) VALUES (
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'Asia/Tokyo', ?, ?, ?, ?, ?, ?, ?,
		'automatic', 0, ?, ?, 'none', NULL, NULL, NULL, NULL, NULL, NULL, NULL,
		?, ?, ?, ?, ?, ?
	)
	ON CONFLICT(id) DO UPDATE SET
		source=excluded.source, source_item_id=excluded.source_item_id,
		source_key=excluded.source_key, title=excluded.title, detail=excluded.detail,
		starts_on=excluded.starts_on, starts_at=excluded.starts_at,
		ends_at=excluded.ends_at, ends_on=excluded.ends_on, timezone=excluded.timezone,
		time_precision=excluded.time_precision, status=excluded.status,
		event_type=excluded.event_type,
		tag=CASE WHEN excluded.tag IS NULL OR events.tag_source='manual'
				OR excluded.tag IN ('live','video') AND events.tag IN ('game','talk','song')
			THEN events.tag ELSE excluded.tag END,
		tag_source=CASE WHEN excluded.tag IS NULL OR events.tag_source='manual'
				OR excluded.tag IN ('live','video') AND events.tag IN ('game','talk','song')
			THEN events.tag_source ELSE excluded.tag_source END,
		url=excluded.url,
		is_upcoming=excluded.is_upcoming, provenance=excluded.provenance,
		manual_locked=excluded.manual_locked, confidence=excluded.confidence,
		extraction_id=excluded.extraction_id, deleted_at=excluded.deleted_at,
		cancellation_status=CASE WHEN events.cancellation_status IN ('llm_suspected', 'manual_confirmed')
			THEN events.cancellation_status ELSE excluded.cancellation_status END,
		cancellation_source=CASE WHEN events.cancellation_status IN ('llm_suspected', 'manual_confirmed')
			THEN events.cancellation_source ELSE excluded.cancellation_source END,
		cancellation_reason=CASE WHEN events.cancellation_status IN ('llm_suspected', 'manual_confirmed')
			THEN events.cancellation_reason ELSE excluded.cancellation_reason END,
		cancellation_evidence=CASE WHEN events.cancellation_status IN ('llm_suspected', 'manual_confirmed')
			THEN events.cancellation_evidence ELSE excluded.cancellation_evidence END,
		cancellation_source_item_id=CASE WHEN events.cancellation_status IN ('llm_suspected', 'manual_confirmed')
			THEN events.cancellation_source_item_id ELSE excluded.cancellation_source_item_id END,
		cancellation_confidence=CASE WHEN events.cancellation_status IN ('llm_suspected', 'manual_confirmed')
			THEN events.cancellation_confidence ELSE excluded.cancellation_confidence END,
		cancellation_at=CASE WHEN events.cancellation_status IN ('llm_suspected', 'manual_confirmed')
			THEN events.cancellation_at ELSE excluded.cancellation_at END,
		updated_at=excluded.updated_at, raw_json=excluded.raw_json, platform=excluded.platform,
		platforms=excluded.platforms, platforms_inferred=excluded.platforms_inferred
	WHERE events.manual_locked = 0`

func nullableText(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

// llmTagSource records that the extraction prompt decided the label. A post
// that names no label leaves the row's label and source as they are (see
// upsertEventSQL), so a label the title inference stored on the same YouTube
// entry survives, while a label the post states replaces it.
func llmTagSource(tag *string) any {
	if tag == nil || *tag == "" {
		return nil
	}
	return sqlite.TagSourceLLM
}

// lendYoutubeEventSQL is what an entry lends the event of a YouTube video
// that the YouTube writer wrote: its stated label, under the label rules of
// upsertEventSQL, and the announced platforms. Every other field is the
// YouTube writer's.
const lendYoutubeEventSQL = `UPDATE events SET
		tag=CASE WHEN ? IS NULL OR tag_source='manual' OR ? IN ('live','video') AND tag IN ('game','talk','song')
			THEN tag ELSE ? END,
		tag_source=CASE WHEN ? IS NULL OR tag_source='manual' OR ? IN ('live','video') AND tag IN ('game','talk','song')
			THEN tag_source ELSE ? END,
		platforms=?, platform=?, updated_at=?
	WHERE id=? AND manual_locked=0`

// youtubeWritten reports whether the YouTube writer vouches for the event:
// it holds that video's active reservation evidence.
func youtubeWritten(ctx context.Context, tx *sql.Tx, eventID string) (bool, error) {
	var found int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM event_sources WHERE event_id=? AND source='youtube'
		AND source_key='reservation' AND deleted_at IS NULL LIMIT 1`, eventID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// lendToYoutubeEvent attaches an entry to a YouTube-written event: the stored
// platforms keep their order and gain the ones the entry states.
func lendToYoutubeEvent(ctx context.Context, tx *sql.Tx, eventID string, event normalizedEvent, now string) error {
	var stored sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT platforms FROM events WHERE id=?`, eventID).Scan(&stored); err != nil {
		return err
	}
	platforms := []string{}
	if stored.Valid {
		_ = json.Unmarshal([]byte(stored.String), &platforms)
	}
	if len(platforms) == 0 {
		platforms = []string{sqlite.PlatformYouTube}
	}
	if !event.platformsInferred {
		platforms = append(platforms, event.platforms...)
	}
	platforms = sqlite.NormalizePlatforms(platforms)
	tag, tagSource := nullableText(event.tag), llmTagSource(event.tag)
	_, err := tx.ExecContext(ctx, lendYoutubeEventSQL, tag, tag, tag, tag, tag, tagSource,
		sqlite.EncodePlatforms(platforms), sqlite.PrimaryPlatform(platforms), now, eventID)
	return err
}

// postEvidenceEvents are the events the post's active evidence supports.
func postEvidenceEvents(ctx context.Context, tx *sql.Tx, source, postID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT event_id FROM event_sources
		WHERE source=? AND source_item_id=? AND deleted_at IS NULL ORDER BY event_id`, source, postID)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, errors.Join(rows.Err(), rows.Close())
}

// retireUnsupported retires an automatic, unlocked event that has no active
// announcing evidence left, and releases the rule merges into it.
func retireUnsupported(ctx context.Context, tx *sql.Tx, eventID, timestamp string) error {
	var remaining int
	// An upload the release was paired with is not an announcement: it
	// keeps no retired entry alive.
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_sources WHERE event_id=? AND deleted_at IS NULL AND source<>'bilibili-video'`, eventID).Scan(&remaining); err != nil {
		return err
	}
	if remaining > 0 {
		return nil
	}
	retired, err := tx.ExecContext(ctx, `UPDATE events SET deleted_at=?, updated_at=?
		WHERE id=? AND deleted_at IS NULL AND provenance='automatic' AND manual_locked=0`, timestamp, timestamp, eventID)
	if err != nil {
		return err
	}
	// As with a manual delete, the rule merges into a retired event are
	// released, so a duplicate outside the reconciler's window is not left
	// hidden behind it.
	if count, _ := retired.RowsAffected(); count > 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE events SET merged_into=NULL, merge_source=NULL, merge_reason=NULL, merged_at=NULL
			WHERE merged_into=? AND merge_source='rule'`, eventID); err != nil {
			return err
		}
	}
	return nil
}

// replaceAutomaticEvents mirrors replaceAutomaticEventsForSource for the
// post's source scope (x or bilibili). The upsert is guarded by manual_locked = 0, so an operator's
// edit or tombstone keeps its fields while the source evidence row is still
// attached to it, and evidence the post attached to such an event stays
// there whatever identity the entry takes now. An entry that names a YouTube
// video only attaches its evidence to an event the YouTube writer wrote. An
// event that loses its last live source in this write, because the post no
// longer lists it or its evidence moved to another identity, is retired when
// it is automatic and unlocked.
func (e *Extractor) replaceAutomaticEvents(ctx context.Context, tx *sql.Tx, source, postID string, extractionID int64, events []normalizedEvent, preserve []string) error {
	timestamp := e.nowISO()
	supported, err := postEvidenceEvents(ctx, tx, source, postID)
	if err != nil {
		return err
	}
	activeKeys := map[string]bool{}
	for _, event := range events {
		activeKeys[event.sourceKey] = true
		eventID := event.id
		if event.canonicalSource == source {
			// Source evidence survives manual edits and tombstones. Reuse its ID
			// across posts as well as across the old and new ID schemes.
			var previous string
			err := tx.QueryRowContext(ctx, `SELECT s.event_id FROM event_sources s
				JOIN events e ON e.id=s.event_id
				WHERE s.source=? AND s.source_key=? AND e.source=? AND e.source_key=?
				ORDER BY (s.source_item_id=?) DESC, s.id ASC LIMIT 1`,
				source, event.sourceKey, source, event.canonicalSourceKey, postID).Scan(&previous)
			if err == nil {
				eventID = previous
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		raw := event.rawObject()
		raw.set("id", eventID)
		raw.set("source_key", event.canonicalSourceKey)
		raw.set("source", event.canonicalSource)
		raw.set("source_item_id", event.canonicalSourceItemID)
		raw.set("provenance", "automatic")
		raw.set("manual_locked", 0)
		raw.set("extraction_id", extractionID)
		raw.set("deleted_at", nil)
		// The persisted source identity wins over a newly generated ID, including
		// legacy date-only IDs and locked/deleted rows. Never insert a second row
		// for a tuple already protected by the unique source index.
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT id FROM events WHERE source=? AND source_item_id=? AND source_key=?`,
			event.canonicalSource, event.canonicalSourceItemID, event.canonicalSourceKey).Scan(&existing)
		if err == nil {
			eventID = existing
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		// Evidence the post attached to an operator's edit or tombstone stays
		// with it, so a link the entry gains or drops neither shows a deleted
		// activity again under another identity nor duplicates an edited one.
		var held string
		err = tx.QueryRowContext(ctx, `SELECT s.event_id FROM event_sources s JOIN events e ON e.id=s.event_id
			WHERE s.source=? AND s.source_item_id=? AND s.source_key=? AND e.manual_locked=1`,
			source, postID, event.sourceKey).Scan(&held)
		if err == nil {
			eventID = held
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		lends := false
		if held == "" && event.canonicalSource == "youtube" {
			if lends, err = youtubeWritten(ctx, tx, eventID); err != nil {
				return err
			}
		}
		upcoming := 0
		if event.isUpcoming {
			upcoming = 1
		}
		now := e.nowISO()
		switch {
		case held != "":
			// The operator's row keeps every field; only the evidence is written.
		case lends:
			if err = lendToYoutubeEvent(ctx, tx, eventID, event, now); err != nil {
				return err
			}
		default:
			if _, err = tx.ExecContext(ctx, upsertEventSQL,
				eventID, event.canonicalSource, event.canonicalSourceItemID, event.canonicalSourceKey,
				event.title, nullableText(event.detail), event.startsOn, nullableText(event.startsAt),
				nullableText(event.endsAt), nullableText(event.endsOn), event.timePrecision, nullable(event.status), event.eventType,
				nullableText(event.tag), llmTagSource(event.tag), nullable(event.url), upcoming, event.confidence, extractionID, now, now, stringify(raw), event.platform, sqlite.EncodePlatforms(event.platforms), event.platformsInferred); err != nil {
				return err
			}
		}
		evidence := event.rawObject()
		evidence.set("source", source)
		evidence.set("source_item_id", postID)
		// Listing the entry again settles a not_schedule reading of the post.
		if _, err = tx.ExecContext(ctx, `INSERT INTO event_sources (
				event_id, source, source_item_id, source_key, url, raw_json, deleted_at,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?)
			ON CONFLICT(source, source_item_id, source_key) DO UPDATE SET
				event_id=excluded.event_id,
				url=COALESCE(excluded.url, event_sources.url),
				raw_json=COALESCE(excluded.raw_json, event_sources.raw_json),
				deleted_at=NULL, unlisted_at=NULL,
				updated_at=excluded.updated_at`,
			eventID, source, postID, event.sourceKey, nullable(event.evidenceURL), stringify(evidence), timestamp, timestamp); err != nil {
			return err
		}
	}

	type sourceRow struct {
		id        int64
		eventID   string
		sourceKey string
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, event_id, source_key FROM event_sources
		WHERE source = ? AND source_item_id = ? AND deleted_at IS NULL`, source, postID)
	if err != nil {
		return err
	}
	previous := []sourceRow{}
	for rows.Next() {
		var row sourceRow
		if err = rows.Scan(&row.id, &row.eventID, &row.sourceKey); err != nil {
			rows.Close()
			return err
		}
		previous = append(previous, row)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	preserved := map[string]bool{}
	for _, id := range preserve {
		preserved[id] = true
	}
	for _, row := range previous {
		if activeKeys[row.sourceKey] || preserved[row.eventID] {
			continue
		}
		if _, err = tx.ExecContext(ctx, `UPDATE event_sources SET deleted_at=?, updated_at=? WHERE id=?`, timestamp, timestamp, row.id); err != nil {
			return err
		}
		supported = append(supported, row.eventID)
	}
	seen := map[string]bool{}
	for _, eventID := range supported {
		if seen[eventID] {
			continue
		}
		seen[eventID] = true
		if err = retireUnsupported(ctx, tx, eventID, timestamp); err != nil {
			return err
		}
	}
	return nil
}

// markUnlisted records that the post, read again, lists no schedule: its
// live evidence stays, so its events stay, and they wait in the Review queue
// for an operator to keep or delete them (see sqlite.EventAttentionSQL).
func (e *Extractor) markUnlisted(ctx context.Context, tx *sql.Tx, source, postID string) error {
	timestamp := e.nowISO()
	_, err := tx.ExecContext(ctx, `UPDATE event_sources SET unlisted_at=?, updated_at=?
		WHERE source=? AND source_item_id=? AND deleted_at IS NULL AND unlisted_at IS NULL`, timestamp, timestamp, source, postID)
	return err
}

// applyCancellations records the model's cancellation judgement as an
// overlay. It never changes schedule fields and never replaces an operator's
// confirmed cancellation; locked events can still be annotated.
func (e *Extractor) applyCancellations(ctx context.Context, tx *sql.Tx, postID, reason, evidence string, confidence float64, targets []string) error {
	timestamp := e.nowISO()
	if reason == "" {
		reason = "The model judged that this schedule may be cancelled"
	}
	var storedConfidence any
	if finite(confidence) {
		storedConfidence = confidence
	}
	seen := map[string]bool{}
	for _, eventID := range targets {
		eventID = jsTrim(eventID)
		if eventID == "" || seen[eventID] {
			continue
		}
		seen[eventID] = true
		if _, err := tx.ExecContext(ctx, `UPDATE events SET
				cancellation_status='llm_suspected', cancellation_source='llm',
				cancellation_reason=?, cancellation_evidence=?,
				cancellation_source_item_id=?, cancellation_confidence=?,
				cancellation_at=?, updated_at=?
			WHERE id=? AND deleted_at IS NULL
				AND COALESCE(cancellation_status, 'none') != 'manual_confirmed'`,
			sliceUTF16(reason, 500), nullable(sliceUTF16(evidence, 1000)), nullable(postID),
			storedConfidence, timestamp, timestamp, eventID); err != nil {
			return err
		}
	}
	return nil
}
