package sqlite

import (
	"strings"

	xsource "github.com/yexca/kano-navi/backend/internal/source/x"
)

// A post's public visibility is computed at read time from what the
// platform says about it and what the operator decided, never stored:
//
//   - posts.deleted_at (a repost's own ID soft-deleted) removes the post
//     from every reader, and nothing overrides it;
//   - posts.shown_at, the operator's show, shows the post whatever the
//     rules below say;
//   - posts.hidden_at, the operator's hide, hides it;
//   - posts.upstream_deleted_at, X no longer has the status, hides it;
//   - an own post (promoted_at IS NULL) is hidden when posts.upstream_kind
//     is reply, foreign, repost or unlisted; an unknown (NULL) kind shows;
//   - a promoted post is hidden while the X status pool demotes it: an
//     operator's demote (x_statuses.manual_decision) or, without an
//     operator's decision, the rules' demotion (x_statuses.demoted_at).
//
// Synchronization, imports and reviews write only the platform columns;
// shown_at and hidden_at are the operator's alone, and at most one of them
// is set.

// XKindUnlisted is the upstream kind of an earlier import's post that an
// account export left out: not one of the account's top-level posts, its
// shape unknown until an answer tells it.
const XKindUnlisted = "unlisted"

// upstreamKinds are the values posts.upstream_kind takes.
var upstreamKinds = map[string]bool{
	xsource.StatusTopLevel: true, xsource.StatusSelfReply: true, xsource.StatusReply: true,
	xsource.StatusForeign: true, xsource.StatusRepost: true, XKindUnlisted: true,
}

// hiddenKindsSQL lists the upstream kinds that keep an own post off the
// public pages, in the order posts_account_instant_idx names them.
var hiddenKindsSQL = "'" + strings.Join([]string{xsource.StatusReply, xsource.StatusForeign, xsource.StatusRepost, XKindUnlisted}, "','") + "'"

// postColumn names a column of the posts row aliased alias, or of the
// unaliased posts table when alias is empty.
func postColumn(alias, column string) string {
	if alias == "" {
		return column
	}
	return alias + "." + column
}

// ownPostShownSQL holds for an own post (promoted_at IS NULL) of the posts
// row aliased alias that the public pages show. It reads posts columns
// only, so posts_account_instant_idx can carry it.
func ownPostShownSQL(alias string) string {
	c := func(column string) string { return postColumn(alias, column) }
	return `(` + c("shown_at") + ` IS NOT NULL OR (` + c("hidden_at") + ` IS NULL AND ` + c("upstream_deleted_at") + ` IS NULL AND COALESCE(` + c("upstream_kind") + `,'') NOT IN (` + hiddenKindsSQL + `)))`
}

// postDemotedSQL holds for a promoted post of the posts row aliased alias
// that the X status pool demotes.
func postDemotedSQL(alias string) string {
	id, x := alias+".id", alias+"xd"
	if alias == "" {
		id, x = "posts.id", "pxd"
	}
	return `EXISTS (SELECT 1 FROM x_statuses ` + x + ` WHERE ` + x + `.id=` + id + ` AND (` + x + `.manual_decision='demote' OR (` + x + `.manual_decision IS NULL AND ` + x + `.demoted_at IS NOT NULL)))`
}

// postShownSQL holds for the posts row aliased alias (or the unaliased
// posts table) when its operator decisions and upstream state show it,
// whether or not it is soft-deleted.
func postShownSQL(alias string) string {
	c := func(column string) string { return postColumn(alias, column) }
	return `(` + c("shown_at") + ` IS NOT NULL OR (` + c("hidden_at") + ` IS NULL AND ` + c("upstream_deleted_at") + ` IS NULL AND CASE WHEN ` + c("promoted_at") + ` IS NULL THEN COALESCE(` + c("upstream_kind") + `,'') NOT IN (` + hiddenKindsSQL + `) ELSE NOT ` + postDemotedSQL(alias) + ` END))`
}

// PostVisibleSQL holds for a post the public pages show: live and shown.
// Every public reader applies it, and so do extraction, translation and the
// console's queues, so a post the board leaves out is not processed either.
func PostVisibleSQL(alias string) string {
	return postColumn(alias, "deleted_at") + ` IS NULL AND ` + postShownSQL(alias)
}

// ownPublicPostSQL holds for an own post of the unaliased posts table that
// the public pages show. Its terms are posts_account_instant_idx's, so a
// query that applies it reads that index.
var ownPublicPostSQL = `deleted_at IS NULL AND promoted_at IS NULL AND ` + ownPostShownSQL("")

// postUpstreamGoneSQL holds for the posts row aliased alias when X no longer
// has its status and the operator does not show it anyway.
func postUpstreamGoneSQL(alias string) string {
	return postColumn(alias, "upstream_deleted_at") + ` IS NOT NULL AND ` + postColumn(alias, "shown_at") + ` IS NULL`
}
