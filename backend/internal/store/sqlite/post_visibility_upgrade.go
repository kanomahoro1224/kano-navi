package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	xsource "github.com/yexca/kano-navi/backend/internal/source/x"
)

// backfillPostUpstreamState converts a snapshot whose posts.hidden_at held
// automatic hides into the platform columns, once, in the transaction that
// adds posts.upstream_kind and posts.shown_at. Afterwards hidden_at holds
// only the operator's hides, and every post keeps its public visibility:
//
//   - an earlier import's reviewed post takes its outcome as upstream_kind;
//     a gone outcome instead sets upstream_deleted_at at the review's
//     checkedAt (else the record's at);
//   - an import record without a usable outcome (unreviewed, or gone) makes
//     the kind unlisted;
//   - a reviewed import record that still says hidden while hidden_at is
//     empty was shown by an operator, which shown_at now records;
//   - hidden_at is cleared where it is the import's own hide (it equals the
//     record's at) or the X status pool's: a promoted post the pool demotes
//     (an operator's demote, or the rules' demoted_at without an operator's
//     decision), since an operator's hide in Posts withdrew both;
//   - an account export's post (raw_json.archive) is top_level, and any
//     other X post takes the kind its stored answer tells (StatusKind).
//
// Each statement touches only rows still in the old shape.
func backfillPostUpstreamState(ctx context.Context, tx *sql.Tx, now string) error {
	fail := func() error { return fmt.Errorf("%w: post upstream state backfill failed", ErrSchema) }
	const record = `json_valid(raw_json) AND json_extract(raw_json,'$.import.reconcile') IS NOT NULL`
	const outcome = `json_extract(raw_json,'$.import.reconcile.outcome')`
	shownKinds := "'" + xsource.StatusTopLevel + "','" + xsource.StatusSelfReply + "'"
	kinds := shownKinds + ",'" + xsource.StatusReply + "','" + xsource.StatusForeign + "','" + xsource.StatusRepost + "'"
	for _, step := range []struct {
		query string
		args  []any
	}{
		{`UPDATE posts SET upstream_kind=` + outcome + ` WHERE source='x' AND upstream_kind IS NULL AND ` + record + ` AND ` + outcome + ` IN (` + kinds + `)`, nil},
		{`UPDATE posts SET upstream_deleted_at=COALESCE(json_extract(raw_json,'$.import.reconcile.checkedAt'),json_extract(raw_json,'$.import.reconcile.at'),?)
			WHERE source='x' AND upstream_deleted_at IS NULL AND ` + record + ` AND ` + outcome + `=?`, []any{now, xsource.StatusGone}},
		{`UPDATE posts SET upstream_kind=? WHERE source='x' AND upstream_kind IS NULL AND ` + record + ` AND json_extract(raw_json,'$.import.reconcile.decision')=?`, []any{XKindUnlisted, XReconcileHidden}},
		{`UPDATE posts SET shown_at=? WHERE source='x' AND hidden_at IS NULL AND shown_at IS NULL AND ` + record + `
			AND json_extract(raw_json,'$.import.reconcile.decision')=? AND COALESCE(` + outcome + `,'') NOT IN (` + shownKinds + `)`, []any{now, XReconcileHidden}},
		{`UPDATE posts SET hidden_at=NULL WHERE source='x' AND hidden_at IS NOT NULL AND ` + record + ` AND hidden_at=json_extract(raw_json,'$.import.reconcile.at')`, nil},
		{`UPDATE posts SET hidden_at=NULL WHERE promoted_at IS NOT NULL AND hidden_at IS NOT NULL AND ` + postDemotedSQL("") + ``, nil},
		{`UPDATE posts SET upstream_kind=? WHERE source='x' AND upstream_kind IS NULL AND json_valid(raw_json) AND json_extract(raw_json,'$.archive.origin')=?`, []any{xsource.StatusTopLevel, xsource.ArchiveOrigin}},
	} {
		if _, err := tx.ExecContext(ctx, step.query, step.args...); err != nil {
			return fail()
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, COALESCE(account_handle,''), raw_json FROM posts
		WHERE source='x' AND upstream_kind IS NULL AND json_valid(raw_json) AND json_type(raw_json)='object'`)
	if err != nil {
		return fail()
	}
	told := map[string]string{}
	for rows.Next() {
		var id, account, raw string
		if err = rows.Scan(&id, &account, &raw); err != nil {
			rows.Close()
			return fail()
		}
		var payload map[string]any
		if account == "" || json.Unmarshal([]byte(raw), &payload) != nil {
			continue
		}
		if kind := xsource.StatusKind(payload, account); kind != "" {
			told[id] = kind
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fail()
	}
	for id, kind := range told {
		if _, err = tx.ExecContext(ctx, `UPDATE posts SET upstream_kind=? WHERE id=?`, kind, id); err != nil {
			return fail()
		}
	}
	return nil
}

// redefinedIndexes are indexes whose definition changed after snapshots
// stored them; startup rebuilds one whose stored definition differs from
// bootstrap.sql's.
var redefinedIndexes = []string{"posts_account_instant_idx"}

// sameIndexDefinition compares two index definitions ignoring whitespace.
func sameIndexDefinition(a, b string) bool {
	return strings.Join(strings.Fields(a), " ") == strings.Join(strings.Fields(b), " ")
}
