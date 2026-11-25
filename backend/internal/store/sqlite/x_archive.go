package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/yexca/kano-navi/backend/internal/mediaformat"
	xsource "github.com/yexca/kano-navi/backend/internal/source/x"
)

// An account export holds the account's current top-level posts. Importing
// it writes them as the account's own top-level posts (upstream_kind
// top_level) and, with HideUnlisted, marks the posts an earlier third-party
// import (raw_json.import) filed under the account that the export does not
// list: replies, other authors' statuses written as the account's own, and
// deleted posts. Such a post's upstream_kind becomes unlisted, which keeps it
// off the public pages until an answer tells what it is, and
// raw_json.import.reconcile records why, so a review can settle it. The
// import never writes the operator's hidden_at or shown_at.

// XArchiveImport is one account's export.
type XArchiveImport struct {
	Account string
	// Posts are the export's own top-level posts (xsource.ParseArchive).
	Posts []xsource.Post
	// Synchronized is true for an account the X synchronization reads; any
	// other account becomes an archive-only account.
	Synchronized bool
	HideUnlisted bool
}

// XArchiveImportResult counts what an import did, or would do.
type XArchiveImportResult struct {
	// Added posts were not stored; Rewritten ones were imported earlier and
	// now carry the export's record; Reassigned ones were also moved from the
	// account an earlier import filed them under.
	Added, Rewritten, Reassigned int
	// KeptSynchronized posts were stored by synchronization and stay as they
	// are; Skipped ones belong to another account or are known reposts.
	KeptSynchronized, Skipped int
	// HiddenReply and HiddenUnlisted count the earlier import's posts marked
	// unlisted, by reason: a reply (a leading mention of another account) or
	// any other; AlreadyHidden counts, apart from them, those an operator
	// hides anyway.
	HiddenReply, HiddenUnlisted, AlreadyHidden int
	// ArchiveAccountAdded is true when the account joined x_archive_accounts.
	ArchiveAccountAdded bool
}

// Reconcile reasons and decisions in raw_json.import.reconcile.
const (
	XReconcileReasonReply    = "at_reply"
	XReconcileReasonUnlisted = "unlisted"
	XReconcileHidden         = "hidden"
	XReconcileShown          = "shown"
)

// XReconcile is an earlier import's post the export does not list, and the
// decision recorded for it.
type XReconcile struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	At       string `json:"at"`
	// Outcome is what a review of the status found; empty until reviewed.
	Outcome   string `json:"outcome,omitempty"`
	CheckedAt string `json:"checkedAt,omitempty"`
}

var (
	// ErrXArchiveInvalid rejects an import without a valid account.
	ErrXArchiveInvalid = errors.New("x_archive_invalid")
	archiveHandle      = regexp.MustCompile(`^[A-Za-z0-9_]{1,30}$`)
	leadingMention     = regexp.MustCompile(`^[\s.]*[@＠]([A-Za-z0-9_]{1,30})`)
)

// ImportXArchives writes the exports in one transaction: every account's
// posts first, so a status an earlier import filed under the wrong account
// reaches its owner before any account's unlisted posts are hidden. dryRun
// counts what the import would do and writes nothing.
func (s *Store) ImportXArchives(ctx context.Context, imports []XArchiveImport, dryRun bool) ([]XArchiveImportResult, error) {
	if s == nil || s.db == nil || !s.workflowWritable {
		return nil, ErrSourceWrite
	}
	for _, in := range imports {
		if !archiveHandle.MatchString(in.Account) {
			return nil, ErrXArchiveInvalid
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, ErrSourceWrite
	}
	defer tx.Rollback()
	now := workflowTime(time.Now())
	out := make([]XArchiveImportResult, len(imports))
	listed := make([]map[string]bool, len(imports))
	for i, in := range imports {
		listed[i] = map[string]bool{}
		for _, post := range in.Posts {
			listed[i][post.ID] = true
			if err = importArchivePost(ctx, tx, in.Account, post, now, &out[i]); err != nil {
				return nil, err
			}
		}
	}
	for i, in := range imports {
		if in.HideUnlisted {
			if err = hideUnlistedImports(ctx, tx, in.Account, listed[i], now, &out[i]); err != nil {
				return nil, err
			}
		}
		if !in.Synchronized {
			if out[i].ArchiveAccountAdded, err = addXArchiveAccount(ctx, tx, in.Account, now); err != nil {
				return nil, err
			}
		}
	}
	if err = bumpRevision(ctx, tx, now); err != nil {
		return nil, ErrSourceWrite
	}
	if dryRun {
		return out, nil
	}
	if tx.Commit() != nil {
		return nil, ErrSourceWrite
	}
	return out, nil
}

func importArchivePost(ctx context.Context, tx *sql.Tx, account string, post xsource.Post, now string, out *XArchiveImportResult) error {
	var owner sql.NullString
	var imported bool
	// An earlier import's post, or one an earlier run of this import wrote,
	// takes the export's record; a synchronized post keeps its own.
	err := tx.QueryRowContext(ctx, `SELECT account_handle,
		COALESCE(json_valid(raw_json) AND (json_extract(raw_json,'$.import') IS NOT NULL OR json_extract(raw_json,'$.archive.origin')=?), 0)
		FROM posts WHERE id=? AND source='x'`, xsource.ArchiveOrigin, post.ID).Scan(&owner, &imported)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ErrSourceWrite
	}
	if exists && !imported {
		if !strings.EqualFold(owner.String, account) {
			out.Skipped++
			return nil
		}
		// The export lists it, so it is a top-level post; a kind an answer
		// told stays.
		if _, err = tx.ExecContext(ctx, `UPDATE posts SET upstream_kind=? WHERE id=? AND promoted_at IS NULL AND (upstream_kind IS NULL OR upstream_kind=?)`, xsource.StatusTopLevel, post.ID, XKindUnlisted); err != nil {
			return ErrSourceWrite
		}
		out.KeptSynchronized++
		return nil
	}
	if exists && !strings.EqualFold(owner.String, account) {
		// An earlier import filed another account's status here (a repost
		// written as the reposter's post); the export names its owner.
		if _, err = tx.ExecContext(ctx, `UPDATE posts SET account_handle=? WHERE id=?`, account, post.ID); err != nil {
			return ErrSourceWrite
		}
		out.Reassigned++
	}
	// The post's Kind (top_level) replaces an earlier run's unlisted mark.
	stored, err := writeXPostAs(ctx, tx, account, post, now, true)
	if err != nil {
		return ErrSourceWrite
	}
	switch {
	case !stored:
		out.Skipped++
	case exists:
		out.Rewritten++
	default:
		out.Added++
	}
	return nil
}

func hideUnlistedImports(ctx context.Context, tx *sql.Tx, account string, listed map[string]bool, now string, out *XArchiveImportResult) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, text, hidden_at IS NOT NULL FROM posts
		WHERE source='x' AND lower(account_handle)=lower(?) AND deleted_at IS NULL AND json_valid(raw_json)
		AND json_extract(raw_json,'$.import') IS NOT NULL AND json_extract(raw_json,'$.import.reconcile') IS NULL`, account)
	if err != nil {
		return ErrSourceWrite
	}
	type unlisted struct {
		id, reason string
		hidden     bool
	}
	found := []unlisted{}
	for rows.Next() {
		var id, text string
		var hidden bool
		if err = rows.Scan(&id, &text, &hidden); err != nil {
			rows.Close()
			return ErrSourceWrite
		}
		if listed[id] {
			continue
		}
		found = append(found, unlisted{id, unlistedReason(account, text), hidden})
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return ErrSourceWrite
	}
	for _, post := range found {
		record, _ := json.Marshal(XReconcile{Decision: XReconcileHidden, Reason: post.reason, At: now})
		// A kind an answer told outranks the export's silence and stays.
		if _, err = tx.ExecContext(ctx, `UPDATE posts SET upstream_kind=COALESCE(upstream_kind,?), raw_json=json_set(raw_json,'$.import.reconcile',json(?)) WHERE id=?`, XKindUnlisted, string(record), post.id); err != nil {
			return ErrSourceWrite
		}
		switch {
		case post.hidden:
			out.AlreadyHidden++
		case post.reason == XReconcileReasonReply:
			out.HiddenReply++
		default:
			out.HiddenUnlisted++
		}
	}
	return nil
}

// unlistedReason is at_reply for wording that opens with another account's
// handle, as a reply did until X moved reply mentions out of the text, and
// unlisted for anything else, a thread's reply to the account itself
// included.
func unlistedReason(account, text string) string {
	if m := leadingMention.FindStringSubmatch(text); m != nil && !strings.EqualFold(m[1], account) {
		return XReconcileReasonReply
	}
	return XReconcileReasonUnlisted
}

func addXArchiveAccount(ctx context.Context, tx *sql.Tx, account, now string) (bool, error) {
	var value string
	err := tx.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key=?`, XArchiveAccountsSetting).Scan(&value)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, ErrSourceWrite
	}
	accounts := parseAccountList(value)
	for _, known := range accounts {
		if strings.EqualFold(known, account) {
			return false, nil
		}
	}
	encoded, _ := json.Marshal(append(accounts, account))
	if _, err = tx.ExecContext(ctx, `INSERT INTO app_settings(key,value,updated_at) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, XArchiveAccountsSetting, string(encoded), now); err != nil {
		return false, ErrSourceWrite
	}
	return true, nil
}

// SaveXArchiveImage stores an export's copy of a pending X image, so the
// media step need not download it. It reports false, and stores nothing, for
// an image that is not a pending X asset or bytes the cache does not take.
func (s *Store) SaveXArchiveImage(ctx context.Context, rawURL string, content []byte) (bool, error) {
	if s == nil || s.db == nil || !s.workflowWritable {
		return false, ErrSourceWrite
	}
	id, normalized, err := mediaIdentity(rawURL)
	if err != nil {
		return false, nil
	}
	var status string
	err = s.db.QueryRowContext(ctx, `SELECT status FROM media_assets WHERE id=? AND source='x' AND source_url=?`, id, normalized).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && status == "ready") {
		return false, nil
	}
	if err != nil {
		return false, ErrSourceWrite
	}
	mime := mediaformat.Sniff(content)
	if len(content) == 0 || len(content) > maxCachedMediaBytes || mediaformat.Extension(mime) == "" {
		return false, nil
	}
	if err = s.SaveMediaDownload(ctx, PendingMedia{ID: id, Source: "x", SourceURL: normalized}, content, mime, "", "", "ready", ""); err != nil {
		return false, err
	}
	return true, nil
}

// XReconcileCandidate is an unlisted post waiting for its review.
type XReconcileCandidate struct{ ID, Account string }

// XReconcileCandidates lists the own posts whose upstream kind is still
// unlisted, whatever the import's reason, newest first. A post X no longer
// has (upstream_deleted_at) needs no review.
func (s *Store) XReconcileCandidates(ctx context.Context) ([]XReconcileCandidate, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_handle FROM posts
		WHERE source='x' AND upstream_kind=? AND upstream_deleted_at IS NULL AND deleted_at IS NULL AND promoted_at IS NULL
		AND account_handle IS NOT NULL
		ORDER BY julianday(published_at) DESC, id`, XKindUnlisted)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	out := []XReconcileCandidate{}
	for rows.Next() {
		var c XReconcileCandidate
		if err = rows.Scan(&c.ID, &c.Account); err != nil {
			return nil, dbErr(err)
		}
		out = append(out, c)
	}
	return out, dbErr(rows.Err())
}

// XReconcileOutcome is a review's finding for one post (an xsource Status
// outcome) and when the status was read.
type XReconcileOutcome struct {
	ID        string `json:"id"`
	Account   string `json:"account"`
	Outcome   string `json:"outcome"`
	CheckedAt string `json:"checkedAt"`
}

// XReconcileApplied counts what applying reviews did, or would do.
type XReconcileApplied struct {
	// Shown posts are the account's own top-level posts or thread replies,
	// which the rules show; Kept ones are replies, other authors' statuses,
	// reposts and gone statuses, which they keep off the public pages;
	// Unknown ones carry no unlisted mark or import record to settle.
	Shown, Kept, Unknown int
}

// reviewShows are the outcomes the rules show.
var reviewShows = map[string]bool{xsource.StatusTopLevel: true, xsource.StatusSelfReply: true}

var reviewOutcomes = map[string]bool{
	xsource.StatusTopLevel: true, xsource.StatusSelfReply: true, xsource.StatusReply: true,
	xsource.StatusForeign: true, xsource.StatusRepost: true, xsource.StatusGone: true,
}

// ApplyXReconcile records each review's outcome on an own post that is
// unlisted or carries the import's record: the outcome becomes the post's
// upstream_kind, except that gone sets upstream_deleted_at (at the review's
// checkedAt) and keeps the kind; any other outcome carried the status, so it
// clears an upstream_deleted_at set before the review read the post (a mark
// set after it is newer than the review and stays). The import's record in
// raw_json.import.reconcile takes the outcome. It never writes the
// operator's hidden_at or shown_at.
func (s *Store) ApplyXReconcile(ctx context.Context, outcomes []XReconcileOutcome, dryRun bool) (XReconcileApplied, error) {
	var out XReconcileApplied
	if s == nil || s.db == nil || !s.workflowWritable {
		return out, ErrSourceWrite
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return out, ErrSourceWrite
	}
	defer tx.Rollback()
	now := workflowTime(time.Now())
	for _, review := range outcomes {
		if !reviewOutcomes[review.Outcome] {
			out.Unknown++
			continue
		}
		var account sql.NullString
		var reconcile string
		var unlisted bool
		err = tx.QueryRowContext(ctx, `SELECT account_handle, upstream_kind IS ?,
			CASE WHEN json_valid(raw_json) THEN COALESCE(json_extract(raw_json,'$.import.reconcile'),'') ELSE '' END
			FROM posts WHERE id=? AND source='x' AND deleted_at IS NULL AND promoted_at IS NULL`, XKindUnlisted, review.ID).Scan(&account, &unlisted, &reconcile)
		if errors.Is(err, sql.ErrNoRows) {
			out.Unknown++
			continue
		}
		if err != nil {
			return XReconcileApplied{}, ErrSourceWrite
		}
		var record XReconcile
		recorded := reconcile != "" && json.Unmarshal([]byte(reconcile), &record) == nil && record.Decision == XReconcileHidden
		if (!recorded && !unlisted) || (review.Account != "" && !strings.EqualFold(review.Account, account.String)) {
			out.Unknown++
			continue
		}
		checkedAt := review.CheckedAt
		if _, parseErr := time.Parse(time.RFC3339Nano, checkedAt); parseErr != nil {
			checkedAt = now
		}
		gone := review.Outcome == xsource.StatusGone
		kind := any(review.Outcome)
		if gone {
			kind = nil
		}
		if reviewShows[review.Outcome] {
			out.Shown++
		} else {
			out.Kept++
		}
		if _, err = tx.ExecContext(ctx, `UPDATE posts SET upstream_kind=COALESCE(?,upstream_kind),
			upstream_deleted_at=CASE WHEN ? THEN COALESCE(upstream_deleted_at,?)
				WHEN julianday(upstream_deleted_at) > julianday(?) THEN upstream_deleted_at END WHERE id=?`, kind, gone, checkedAt, checkedAt, review.ID); err != nil {
			return XReconcileApplied{}, ErrSourceWrite
		}
		if !recorded {
			continue
		}
		record.Outcome, record.CheckedAt = review.Outcome, review.CheckedAt
		if reviewShows[review.Outcome] {
			record.Decision = XReconcileShown
		}
		encoded, _ := json.Marshal(record)
		if _, err = tx.ExecContext(ctx, `UPDATE posts SET raw_json=json_set(raw_json,'$.import.reconcile',json(?)) WHERE id=?`, string(encoded), review.ID); err != nil {
			return XReconcileApplied{}, ErrSourceWrite
		}
	}
	if err = bumpRevision(ctx, tx, now); err != nil {
		return XReconcileApplied{}, ErrSourceWrite
	}
	if dryRun {
		return out, nil
	}
	if tx.Commit() != nil {
		return XReconcileApplied{}, ErrSourceWrite
	}
	return out, nil
}

// XArchiveControl names a shown post an export listed, the newest: a status
// known to exist, which tells a failing detail source from gone statuses.
// It is empty when there is none.
func (s *Store) XArchiveControl(ctx context.Context) (XReconcileCandidate, error) {
	var c XReconcileCandidate
	if s == nil || s.db == nil {
		return c, ErrUnavailable
	}
	err := s.db.QueryRowContext(ctx, `SELECT id, account_handle FROM posts
		WHERE source='x' AND `+publicPostSQL+` AND account_handle IS NOT NULL
		AND json_valid(raw_json) AND json_extract(raw_json,'$.archive.origin')=?
		ORDER BY julianday(published_at) DESC, id LIMIT 1`, xsource.ArchiveOrigin).Scan(&c.ID, &c.Account)
	if errors.Is(err, sql.ErrNoRows) {
		return XReconcileCandidate{}, nil
	}
	return c, dbErr(err)
}
