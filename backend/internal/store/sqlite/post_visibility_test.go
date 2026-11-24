package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	xsource "github.com/yexca/kano-navi/backend/internal/source/x"
	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// queryIDs runs a query of IDs and returns them sorted.
func queryIDs(t *testing.T, db *sql.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.Query(query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	slices.Sort(ids)
	return ids
}

// The public pages show a post by what X says about it and what the
// operator decided: the operator's show outranks every automatic rule, the
// operator's hide, a status X no longer has and a reply, another author's
// status, a repost or an unlisted post hide it, an unknown kind shows, and a
// promoted post follows the pool's demotion instead of its kind. Nothing
// shows a soft-deleted post.
func TestPostVisibilityFollowsUpstreamStateAndOperatorDecisions(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const at = "'2026-10-20T00:00:00.000Z'"
	rows := []struct {
		id, columns, values string
		shown               bool
	}{
		{"own-unknown", "", "", true},
		{"own-top", "upstream_kind", "'top_level'", true},
		{"own-thread", "upstream_kind", "'self_reply'", true},
		{"own-reply", "upstream_kind", "'reply'", false},
		{"own-foreign", "upstream_kind", "'foreign'", false},
		{"own-repost", "upstream_kind", "'repost'", false},
		{"own-unlisted", "upstream_kind", "'unlisted'", false},
		{"own-gone", "upstream_kind,upstream_deleted_at", "'top_level'," + at, false},
		{"own-hidden", "upstream_kind,hidden_at", "'top_level'," + at, false},
		{"shown-reply", "upstream_kind,shown_at", "'reply'," + at, true},
		{"shown-gone", "upstream_kind,upstream_deleted_at,shown_at", "'unlisted'," + at + "," + at, true},
		{"shown-deleted", "deleted_at,shown_at", at + "," + at, false},
		{"promoted-reply", "upstream_kind,promoted_at", "'reply'," + at, true},
		{"promoted-demoted", "promoted_at", at, false},
		{"promoted-locked", "promoted_at", at, false},
		{"promoted-demoted-shown", "promoted_at,shown_at", at + "," + at, true},
	}
	for _, row := range rows {
		columns, values := "id,source,account_handle,text,published_at,url", "?,'x','kano','wording',"+at+",'https://x.com/kano/status/1'"
		if row.columns != "" {
			columns, values = columns+","+row.columns, values+","+row.values
		}
		testutil.Exec(t, p, `INSERT INTO posts(`+columns+`) VALUES(`+values+`)`, row.id)
	}
	testutil.Exec(t, p, `INSERT INTO posts(id,source,account_handle,text,published_at,url) VALUES('bilibili-1','bilibili','1','wording',`+at+`,'https://www.bilibili.com/opus/1')`)
	for id, columns := range map[string]string{"promoted-reply": "NULL,NULL", "promoted-demoted": at + ",NULL", "promoted-locked": "NULL,'demote'", "promoted-demoted-shown": at + ",NULL"} {
		testutil.Exec(t, p, `INSERT INTO x_statuses(id,author_handle,text,url,demoted_at,manual_decision,first_seen_at,updated_at) VALUES(?,'agency_synthetic','wording','https://x.com/agency_synthetic/status/1',`+columns+`,`+at+`,`+at+`)`, id)
	}
	want := []string{"bilibili-1"}
	for _, row := range rows {
		if row.shown {
			want = append(want, row.id)
		}
	}
	slices.Sort(want)
	if got := queryIDs(t, s.db, `SELECT id FROM posts p WHERE `+PostVisibleSQL("p")); !slices.Equal(got, want) {
		t.Fatalf("visible %v, want %v", got, want)
	}
	if got := queryIDs(t, s.db, `SELECT id FROM posts WHERE `+publicPostSQL); !slices.Equal(got, want) {
		t.Fatalf("unaliased %v, want %v", got, want)
	}
	// The index-backed own-post rule agrees with the general one.
	own := queryIDs(t, s.db, `SELECT id FROM posts p WHERE p.promoted_at IS NULL AND `+PostVisibleSQL("p"))
	if got := queryIDs(t, s.db, `SELECT id FROM posts WHERE `+ownPublicPostSQL); !slices.Equal(got, own) {
		t.Fatalf("own rule %v, general %v", got, own)
	}
}

// A post the public pages leave out is not translated, unless a post quotes
// it: the quote card shares the quoted post's translations, so the quoted
// post stays a translation candidate and its request counts as queued.
func TestQuotedPostsOffThePublicPagesStayTranslated(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	const at = "2026-10-20T00:00:00.000Z"
	insert := func(id, columns, values string) {
		t.Helper()
		testutil.Exec(t, p, `INSERT INTO posts(id,source,account_handle,text,published_at,url`+columns+`) VALUES(?,'x','kano','ことば '||?,?,'https://x.com/kano/status/'||?`+values+`)`, id, id, at, id)
	}
	insert("quoting", ",quote_json", `,'{"id":"quoted-reply","text":"ことば quoted-reply"}'`)
	insert("quoted-reply", ",upstream_kind", ",'reply'")
	insert("lone-reply", ",upstream_kind", ",'reply'")
	insert("quoted-deleted", ",upstream_kind,deleted_at", ",'reply','"+at+"'")
	for _, id := range []string{"quoted-reply", "quoted-deleted"} {
		testutil.Exec(t, p, `INSERT INTO x_statuses(id,author_handle,text,url,quoted_at,first_seen_at,updated_at) VALUES(?,'kano','ことば','https://x.com/kano/status/1',?,?,?)`, id, at, at, at)
	}
	candidates, err := s.ListTranslationCandidates(ctx, time.Time{}, 50)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, candidate := range candidates {
		if candidate.OwnerType == TranslationOwnerPost {
			got = append(got, candidate.OwnerID)
		}
	}
	slices.Sort(got)
	if strings.Join(got, ",") != "quoted-reply,quoting" {
		t.Fatalf("translated posts %v", got)
	}
	for _, id := range []string{"quoted-reply", "lone-reply"} {
		testutil.Exec(t, p, `INSERT INTO translation_states(owner_type,owner_id,field,source_hash,status,requested,updated_at) VALUES('post',?,'text','','pending',1,?)`, id, at)
	}
	queued, err := s.ListAdminPosts(ctx, AdminPostFilter{Queued: "translation"})
	if err != nil || queued.Total != 1 || queued.Items[0].ID != "quoted-reply" {
		t.Fatalf("queued translations=%+v err=%v", queued, err)
	}
}

// Every answer records what it says a post is, and an answer that cannot
// tell keeps the stored kind. A reply then stays off the board, the
// archive, translation and its events, while the console lists it with its
// kind; an operator's show brings it back.
func TestSyncRecordsWhatAnAnswerSaysAPostIs(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	post := func(id, kind string, age time.Duration) xsource.Post {
		out := syntheticXPost("kano", id, "wording "+id, now.Add(-age))
		out.MediaURLs, out.Kind = nil, kind
		return out
	}
	top, reply, unknown := post("1000000000020", xsource.StatusTopLevel, time.Hour), post("1000000000021", xsource.StatusReply, 2*time.Hour), post("1000000000022", "", 3*time.Hour)
	if err = s.WriteXRecent(ctx, "kano", xsource.Result{Account: "kano", Posts: []xsource.Post{top, reply, unknown}}, true); err != nil {
		t.Fatal(err)
	}
	kinds := func() string {
		t.Helper()
		var got string
		if err := s.db.QueryRow(`SELECT group_concat(id||'='||COALESCE(upstream_kind,'-'),',') FROM (SELECT * FROM posts ORDER BY id)`).Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := kinds(); got != "1000000000020=top_level,1000000000021=reply,1000000000022=-" {
		t.Fatalf("written kinds: %s", got)
	}
	public := func() (board, archive, translated []string) {
		t.Helper()
		dashboard, dashErr := s.GetDashboard(ctx, 7, now)
		if dashErr != nil {
			t.Fatal(dashErr)
		}
		for _, item := range dashboard.Posts {
			board = append(board, item.ID)
		}
		page, archiveErr := s.GetPostTimeline(ctx, "", "", 1, 50)
		if archiveErr != nil {
			t.Fatal(archiveErr)
		}
		for _, item := range page.Items {
			archive = append(archive, item.ID)
		}
		candidates, candidateErr := s.ListTranslationCandidates(ctx, time.Time{}, 50)
		if candidateErr != nil {
			t.Fatal(candidateErr)
		}
		for _, candidate := range candidates {
			if candidate.OwnerType == TranslationOwnerPost {
				translated = append(translated, candidate.OwnerID)
			}
		}
		slices.Sort(translated)
		return board, archive, translated
	}
	board, archive, translated := public()
	if strings.Join(board, ",") != top.ID+","+unknown.ID || strings.Join(archive, ",") != top.ID+","+unknown.ID || strings.Join(translated, ",") != top.ID+","+unknown.ID {
		t.Fatalf("board=%v archive=%v translated=%v", board, archive, translated)
	}
	testutil.Exec(t, p, `INSERT INTO events(id,source,source_item_id,title,starts_on,created_at,updated_at) VALUES('reply-event','x',?,'synthetic','2026-10-21','','')`, reply.ID)
	testutil.Exec(t, p, `INSERT INTO event_sources(event_id,source,source_item_id,source_key,created_at,updated_at) VALUES('reply-event','x',?,'reply-event','','')`, reply.ID)
	hiddenEvent := func() bool {
		t.Helper()
		var hidden bool
		if err := s.db.QueryRow(`SELECT ` + eventHiddenByPostsSQL("e") + ` FROM events e WHERE id='reply-event'`).Scan(&hidden); err != nil {
			t.Fatal(err)
		}
		return hidden
	}
	if !hiddenEvent() {
		t.Fatal("the reply's event is public")
	}
	admin, err := s.AdminPostByID(ctx, reply.ID)
	if err != nil || admin.UpstreamKind == nil || *admin.UpstreamKind != xsource.StatusReply || admin.Visible || admin.HiddenAt != nil {
		t.Fatalf("console post=%+v err=%v", admin, err)
	}
	if hidden, err := s.ListAdminPosts(ctx, AdminPostFilter{Visibility: "hidden"}); err != nil || hidden.Total != 1 || hidden.Items[0].ID != reply.ID {
		t.Fatalf("hidden filter=%+v err=%v", hidden, err)
	}

	// A listing that cannot tell keeps the kind; one that tells replaces it.
	listed := func(p xsource.Post) xsource.Result {
		return xsource.Result{Account: "kano", Posts: []xsource.Post{p}, FromListing: map[string]bool{p.ID: true}}
	}
	if err = s.WriteXRecent(ctx, "kano", listed(post(reply.ID, "", 2*time.Hour)), true); err != nil {
		t.Fatal(err)
	}
	if got := kinds(); !strings.Contains(got, reply.ID+"=reply") {
		t.Fatalf("an answer that cannot tell replaced the kind: %s", got)
	}
	if err = s.WriteXRecent(ctx, "kano", listed(post(reply.ID, xsource.StatusSelfReply, 2*time.Hour)), true); err != nil {
		t.Fatal(err)
	}
	if hiddenEvent() {
		t.Fatal("the thread reply's event stays hidden")
	}
	// A detail answer naming another author marks the stored post foreign;
	// the operator's show outranks the kind.
	if err = s.WriteXRecent(ctx, "kano", xsource.Result{Account: "kano", Dropped: []xsource.DroppedStatus{{ID: top.ID, Code: xsource.DroppedForeign}}}, true); err != nil {
		t.Fatal(err)
	}
	if got := kinds(); got != "1000000000020=foreign,1000000000021=self_reply,1000000000022=-" {
		t.Fatalf("after the answers: %s", got)
	}
	if board, _, _ = public(); strings.Join(board, ",") != reply.ID+","+unknown.ID {
		t.Fatalf("board after the answers: %v", board)
	}
	testutil.Exec(t, p, `UPDATE posts SET shown_at='2026-10-22T00:00:00.000Z' WHERE id=?`, top.ID)
	if board, _, _ = public(); strings.Join(board, ",") != top.ID+","+reply.ID+","+unknown.ID {
		t.Fatalf("board after the operator's show: %v", board)
	}
}

// Startup converts the automatic hides an older snapshot keeps in hidden_at
// into the platform columns once: the import's reviewed outcomes become
// kinds or deleted-upstream marks, its unreviewed posts become unlisted, an
// operator's show of a post the import hid becomes shown_at, and the pool's
// demotions are read from the pool. The operator's own hides stay, each
// stored answer tells its post's kind, the account index is rebuilt for the
// new rule, and no post changes visibility.
func TestStandaloneConvertsAutomaticHidesIntoUpstreamState(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, "database", "kano.sqlite")
	s, err := OpenInitialized(p, true, true)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reconcile := func(decision, reason, at, outcome, checked string) string {
		record := `"decision":"` + decision + `","reason":"` + reason + `","at":"` + at + `"`
		if outcome != "" {
			record += `,"outcome":"` + outcome + `","checkedAt":"` + checked + `"`
		}
		return `{"import":{"origin":"https://third-party.example.invalid","reconcile":{` + record + `}}}`
	}
	const (
		importAt  = "2026-11-18T00:00:00.000Z"
		checkedAt = "2026-11-22T07:16:54Z"
		operator  = "2026-11-24T02:33:09.681Z"
	)
	for _, statement := range []string{
		`DROP INDEX posts_account_instant_idx`, `ALTER TABLE posts DROP COLUMN upstream_kind`, `ALTER TABLE posts DROP COLUMN shown_at`,
		`CREATE INDEX posts_account_instant_idx ON posts (lower(COALESCE(account_handle, '')), julianday(published_at) DESC, id) WHERE deleted_at IS NULL AND hidden_at IS NULL AND upstream_deleted_at IS NULL AND promoted_at IS NULL`,
	} {
		if _, err = db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	insert := func(id, account, raw string, hidden any, extra ...string) {
		t.Helper()
		columns, values := "", ""
		for i := 0; i+1 < len(extra); i += 2 {
			columns, values = columns+","+extra[i], values+","+extra[i+1]
		}
		if _, err := db.Exec(`INSERT INTO posts(id,source,account_handle,text,published_at,url,raw_json,hidden_at`+columns+`) VALUES(?,'x',?,'wording','2026-10-01T00:00:00.000Z',?,?,?`+values+`)`,
			id, account, "https://x.com/"+account+"/status/"+id, raw, hidden); err != nil {
			t.Fatal(err)
		}
	}
	insert("import-reply", "kano_2525", reconcile("hidden", "unlisted", importAt, "reply", checkedAt), importAt)
	insert("import-foreign", "kano_2525", reconcile("hidden", "unlisted", importAt, "foreign", checkedAt), importAt)
	insert("import-gone", "kano_2525", reconcile("hidden", "unlisted", importAt, "gone", checkedAt), importAt)
	insert("import-top", "kano_2525", reconcile("shown", "at_reply", importAt, "top_level", checkedAt), nil)
	insert("import-thread", "kano_2525", reconcile("shown", "unlisted", importAt, "self_reply", checkedAt), nil)
	insert("import-unreviewed", "kano_2525", reconcile("hidden", "at_reply", importAt, "", ""), importAt)
	insert("import-operator-shown", "kano_2525", reconcile("hidden", "unlisted", importAt, "reply", checkedAt), nil)
	insert("import-operator-hidden", "kano_2525", reconcile("hidden", "unlisted", importAt, "foreign", checkedAt), operator)
	insert("synced-hidden", "kano_2525", `{"author":{"screen_name":"kano_2525"},"replying_to":null}`, operator)
	insert("synced-thread", "kano_2525", `{"author":{"screen_name":"kano_2525"},"replying_to":{"screen_name":"kano_2525"}}`, nil)
	insert("synced-detail", "kano_2525", `{"user_screen_name":"kano_2525","replyingTo":null,"retweet":null}`, nil)
	insert("synced-silent", "kano_2525", `{"text":"no shape"}`, nil)
	insert("export", "kano_2525", `{"archive":{"origin":"x-export"}}`, nil)
	insert("promoted-auto", "agency_synthetic", `{"author":{"screen_name":"agency_synthetic"},"replying_to":null}`, "2026-10-05T00:00:00.000Z", "promoted_at", "'2026-10-02T00:00:00.000Z'")
	insert("promoted-locked", "agency_synthetic", `{}`, "2026-10-06T00:00:00.000Z", "promoted_at", "'2026-10-02T00:00:00.000Z'")
	insert("promoted-lifted", "agency_synthetic", `{}`, "2026-10-06T00:00:00.000Z", "promoted_at", "'2026-10-02T00:00:00.000Z'")
	insert("promoted-operator", "agency_synthetic", `{}`, operator, "promoted_at", "'2026-10-02T00:00:00.000Z'")
	insert("promoted-shown", "agency_synthetic", `{}`, nil, "promoted_at", "'2026-10-02T00:00:00.000Z'")
	if _, err = db.Exec(`INSERT INTO posts(id,source,account_handle,text,published_at,url,raw_json) VALUES('bilibili-1','bilibili','1','wording','2026-10-01T00:00:00.000Z','https://www.bilibili.com/opus/1','{"author":{"screen_name":"1"},"replying_to":null}')`); err != nil {
		t.Fatal(err)
	}
	for id, columns := range map[string]string{
		"promoted-auto":     "'2026-10-05T00:00:00.000Z',NULL,NULL",
		"promoted-locked":   "NULL,'demote','2026-10-06T00:00:00.000Z'",
		"promoted-lifted":   "'2026-10-09T00:00:00.000Z',NULL,NULL",
		"promoted-operator": "NULL,NULL,NULL",
		"promoted-shown":    "NULL,'promote','2026-10-07T00:00:00.000Z'",
	} {
		if _, err = db.Exec(`INSERT INTO x_statuses(id,author_handle,text,url,demoted_at,manual_decision,manual_at,first_seen_at,updated_at) VALUES(?,'agency_synthetic','wording','https://x.com/agency_synthetic/status/1',`+columns+`,'2026-10-01T00:00:00.000Z','2026-10-01T00:00:00.000Z')`, id); err != nil {
			t.Fatal(err)
		}
	}
	before := queryIDs(t, db, `SELECT id FROM posts WHERE deleted_at IS NULL AND hidden_at IS NULL AND upstream_deleted_at IS NULL`)

	state := func() map[string]string {
		t.Helper()
		rows, err := db.Query(`SELECT id, COALESCE(upstream_kind,'-')||'|'||COALESCE(hidden_at,'-')||'|'||COALESCE(upstream_deleted_at,'-')||'|'||CASE WHEN shown_at IS NULL THEN '-' ELSE 'shown' END FROM posts`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]string{}
		for rows.Next() {
			var id, value string
			if err = rows.Scan(&id, &value); err != nil {
				t.Fatal(err)
			}
			out[id] = value
		}
		return out
	}
	upgraded, err := OpenStandalone(p)
	if err != nil {
		t.Fatal(err)
	}
	upgraded.Close()
	got := state()
	for id, want := range map[string]string{
		"import-reply":           "reply|-|-|-",
		"import-foreign":         "foreign|-|-|-",
		"import-gone":            "unlisted|-|" + checkedAt + "|-",
		"import-top":             "top_level|-|-|-",
		"import-thread":          "self_reply|-|-|-",
		"import-unreviewed":      "unlisted|-|-|-",
		"import-operator-shown":  "reply|-|-|shown",
		"import-operator-hidden": "foreign|" + operator + "|-|-",
		"synced-hidden":          "top_level|" + operator + "|-|-",
		"synced-thread":          "self_reply|-|-|-",
		"synced-detail":          "top_level|-|-|-",
		"synced-silent":          "-|-|-|-",
		"export":                 "top_level|-|-|-",
		"promoted-auto":          "top_level|-|-|-",
		"promoted-locked":        "-|-|-|-",
		"promoted-lifted":        "-|-|-|-",
		"promoted-operator":      "-|" + operator + "|-|-",
		"promoted-shown":         "-|-|-|-",
		"bilibili-1":             "-|-|-|-",
	} {
		if got[id] != want {
			t.Errorf("%s: %s, want %s", id, got[id], want)
		}
	}
	if after := queryIDs(t, db, `SELECT id FROM posts p WHERE `+PostVisibleSQL("p")); !slices.Equal(after, before) {
		t.Fatalf("visibility changed: before %v, after %v", before, after)
	}
	var index string
	if err = db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='index' AND name='posts_account_instant_idx'`).Scan(&index); err != nil || !strings.Contains(index, "upstream_kind") {
		t.Fatalf("index=%s err=%v", index, err)
	}
	// A second start changes nothing.
	again, err := OpenStandalone(p)
	if err != nil {
		t.Fatal(err)
	}
	again.Close()
	if second := state(); len(second) != len(got) {
		t.Fatalf("second start: %v", second)
	} else {
		for id, value := range got {
			if second[id] != value {
				t.Fatalf("second start changed %s: %s to %s", id, value, second[id])
			}
		}
	}
}

// A snapshot older than the X status pool gets the pool's table before the
// conversion reads it: the import's hides still become kinds, a promoted
// post's hide, which no pool state explains, stays the operator's, and no
// post changes visibility.
func TestStandaloneConvertsASnapshotWithoutThePool(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, "database", "kano.sqlite")
	s, err := OpenInitialized(p, true, true)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const importAt = "2026-11-18T00:00:00.000Z"
	for _, statement := range []string{
		`DROP TABLE x_statuses`,
		`DROP INDEX posts_account_instant_idx`, `ALTER TABLE posts DROP COLUMN upstream_kind`, `ALTER TABLE posts DROP COLUMN shown_at`,
		`INSERT INTO posts(id,source,account_handle,text,published_at,url,raw_json,hidden_at) VALUES('import-reply','x','kano_2525','wording','2026-10-01T00:00:00.000Z','https://x.com/kano_2525/status/1',
			'{"import":{"reconcile":{"decision":"hidden","reason":"unlisted","at":"` + importAt + `","outcome":"reply","checkedAt":"2026-11-22T07:16:54Z"}}}','` + importAt + `')`,
		`INSERT INTO posts(id,source,account_handle,text,published_at,url,raw_json,hidden_at,promoted_at) VALUES('promoted-hidden','x','agency_synthetic','wording','2026-10-01T00:00:00.000Z','https://x.com/agency_synthetic/status/2','{}','2026-10-05T00:00:00.000Z','2026-10-02T00:00:00.000Z')`,
		`INSERT INTO posts(id,source,account_handle,text,published_at,url,raw_json) VALUES('own-top','x','kano_2525','wording','2026-10-01T00:00:00.000Z','https://x.com/kano_2525/status/3','{"author":{"screen_name":"kano_2525"},"replying_to":null}')`,
	} {
		if _, err = db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	before := queryIDs(t, db, `SELECT id FROM posts WHERE deleted_at IS NULL AND hidden_at IS NULL AND upstream_deleted_at IS NULL`)
	upgraded, err := OpenStandalone(p)
	if err != nil {
		t.Fatal(err)
	}
	upgraded.Close()
	var state string
	if err = db.QueryRow(`SELECT group_concat(id||'='||COALESCE(upstream_kind,'-')||'|'||COALESCE(hidden_at,'-'),',') FROM (SELECT * FROM posts ORDER BY id)`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "import-reply=reply|-,own-top=top_level|-,promoted-hidden=-|2026-10-05T00:00:00.000Z" {
		t.Fatalf("converted %s", state)
	}
	if after := queryIDs(t, db, `SELECT id FROM posts p WHERE `+PostVisibleSQL("p")); !slices.Equal(after, before) {
		t.Fatalf("visibility changed: before %v, after %v", before, after)
	}
}
