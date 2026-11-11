package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
)

const (
	forwardID   = "900000000000000110"
	forwardedID = "900000000000000210"
	ownID       = "900000000000000220"
)

// biliForward is a synthetic forward by the account of the quoted dynamic.
func biliForward(text string, quote *bilisource.Quote) bilisource.Dynamic {
	return bilisource.Dynamic{IDStr: forwardID, OwnerMid: 316381099, Text: text, PublishedAt: "2026-11-07T05:40:00Z", Images: []string{},
		Quote: quote, Raw: map[string]any{"type": bilisource.ForwardType, "orig": map[string]any{"id": forwardedID, "type": "DYNAMIC_TYPE_DRAW"}}}
}

// agencyDynamic is another author's dynamic with two pictures.
func agencyDynamic(text string) *bilisource.Quote {
	return &bilisource.Quote{ID: "bilibili-" + forwardedID, AuthorHandle: "42", AuthorName: "Synthetic Agency", Text: text, PublishedAt: "2026-11-07T02:53:20Z",
		URL: "https://www.bilibili.com/opus/" + forwardedID, MediaURLs: []string{"https://i0.hdslb.com/bfs/new_dyn/synthetic-q0.jpg", "https://i0.hdslb.com/bfs/new_dyn/synthetic-q1.jpg"}}
}

func biliState() bilisource.State {
	return bilisource.State{Dynamics: bilisource.DynamicsState{LastAttemptAt: time.Now(), LastSuccessAt: time.Now()}}
}

func dashboardQuotes(t *testing.T, s *Store) map[string]Post {
	t.Helper()
	dashboard, err := s.GetDashboard(context.Background(), 30, time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Post{}
	for _, p := range dashboard.Posts {
		out[p.ID] = p
	}
	return out
}

func TestBilibiliForwardsStoreTheirQuoteImagesAndPool(t *testing.T) {
	s := biliStore(t)
	ctx := context.Background()
	start, _ := s.GetDashboardRevision(ctx)
	changed, err := s.WriteBilibiliDynamics(ctx, biliAccount, bilisource.DynamicsResult{Dynamics: []bilisource.Dynamic{biliForward("看这个", agencyDynamic("明天20点直播"))}, State: biliState()})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	var quote, raw string
	var media sql.NullString
	if err = s.DB().QueryRow(`SELECT quote_json, raw_json, media_url FROM posts WHERE id = 'bilibili-`+forwardID+`'`).Scan(&quote, &raw, &media); err != nil {
		t.Fatal(err)
	}
	// The projection names no image address, and the post has none of its own.
	if strings.Contains(quote, "hdslb") || !strings.Contains(quote, `"authorName":"Synthetic Agency"`) || !strings.Contains(raw, bilisource.ForwardType) || media.Valid {
		t.Fatalf("quote=%s raw=%s media=%v", quote, raw, media)
	}
	if got := linkedMedia(t, s.DB(), "bilibili-"+forwardID, "quote-image"); got != "https://i0.hdslb.com/bfs/new_dyn/synthetic-q0.jpg https://i0.hdslb.com/bfs/new_dyn/synthetic-q1.jpg" {
		t.Fatalf("quote images=%s", got)
	}
	var own, sources int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM media_links WHERE owner_id = ? AND role = 'post-image'`, "bilibili-"+forwardID).Scan(&own)
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM media_assets WHERE source = 'bilibili' AND source_url LIKE '%synthetic-q%'`).Scan(&sources)
	if own != 0 || sources != 2 {
		t.Fatalf("own=%d bilibili assets=%d", own, sources)
	}
	// Quoted pictures are never schedule-image candidates.
	var candidates int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM assets WHERE kind = 'schedule'`).Scan(&candidates)
	if candidates != 0 {
		t.Fatalf("schedule candidates=%d", candidates)
	}
	var mid, name, text, forwardedAt string
	if err = s.DB().QueryRow(`SELECT author_mid, author_name, text, forwarded_at FROM bilibili_statuses WHERE id = ?`, "bilibili-"+forwardedID).Scan(&mid, &name, &text, &forwardedAt); err != nil ||
		mid != "42" || name != "Synthetic Agency" || text != "明天20点直播" || forwardedAt != "2026-11-07T05:40:00Z" {
		t.Fatalf("pool=%s %s %s %s err=%v", mid, name, text, forwardedAt, err)
	}

	posts := dashboardQuotes(t, s)
	q := posts["bilibili-"+forwardID].Quote
	if q == nil || q.ID != "bilibili-"+forwardedID || q.AuthorHandle != "42" || q.AuthorName != "Synthetic Agency" || q.Text != "明天20点直播" ||
		q.URL != "https://www.bilibili.com/opus/"+forwardedID || q.Unavailable || len(q.Media) != 0 {
		t.Fatalf("dashboard quote=%+v", q)
	}

	// The same read changes nothing; a changed quote is a visible change.
	revision, _ := s.GetDashboardRevision(ctx)
	if changed, _ = s.WriteBilibiliDynamics(ctx, biliAccount, bilisource.DynamicsResult{Dynamics: []bilisource.Dynamic{biliForward("看这个", agencyDynamic("明天20点直播"))}, State: biliState()}); changed {
		t.Fatal("unchanged forward reported a change")
	}
	if changed, _ = s.WriteBilibiliDynamics(ctx, biliAccount, bilisource.DynamicsResult{Dynamics: []bilisource.Dynamic{biliForward("看这个", agencyDynamic("明天21点直播"))}, State: biliState()}); !changed {
		t.Fatal("edited quote reported no change")
	}
	if after, _ := s.GetDashboardRevision(ctx); after != revision+1 || revision != start+1 {
		t.Fatalf("revisions start=%d mid=%d after=%d", start, revision, after)
	}
}

// An operator's single read takes the dynamic as its page shows it, without
// a quote; it never rewrites a forward the listing stored.
func TestASingleReadNeverRewritesAStoredForward(t *testing.T) {
	s := biliStore(t)
	ctx := context.Background()
	if _, err := s.WriteBilibiliDynamics(ctx, biliAccount, bilisource.DynamicsResult{Dynamics: []bilisource.Dynamic{biliForward("看这个", agencyDynamic("明天20点直播"))}, State: biliState()}); err != nil {
		t.Fatal(err)
	}
	revision, _ := s.GetDashboardRevision(ctx)
	read := biliForward("看这个 as the page shows it", nil)
	read.Raw = map[string]any{"type": "opus"}
	if changed, err := s.WriteBilibiliDynamic(ctx, biliAccount, read, biliState()); err != nil || changed {
		t.Fatalf("operator read changed=%v err=%v", changed, err)
	}
	p := dashboardQuotes(t, s)["bilibili-"+forwardID]
	if p.Text != "看这个" || p.Quote == nil || p.Quote.Text != "明天20点直播" {
		t.Fatalf("post after operator read=%+v quote=%+v", p, p.Quote)
	}
	if got := linkedMedia(t, s.DB(), "bilibili-"+forwardID, "quote-image"); got == "" {
		t.Fatal("operator read removed the quote images")
	}
	if after, _ := s.GetDashboardRevision(ctx); after != revision {
		t.Fatalf("revision %d -> %d", revision, after)
	}
}

// A forwarded dynamic that becomes unavailable replaces a stored forward's
// quote, with or without wording of its own, and removes its quoted images;
// its pool row stays. A new forward without wording of its own whose dynamic
// is unavailable is not stored.
func TestForwardsOfADynamicThatIsGoneShowItUnavailable(t *testing.T) {
	s := biliStore(t)
	ctx := context.Background()
	textless := biliForward("", agencyDynamic("明天20点直播"))
	textless.IDStr = "900000000000000111"
	write := func(dynamics ...bilisource.Dynamic) bool {
		t.Helper()
		changed, err := s.WriteBilibiliDynamics(ctx, biliAccount, bilisource.DynamicsResult{Dynamics: dynamics, State: biliState()})
		if err != nil {
			t.Fatal(err)
		}
		return changed
	}
	write(biliForward("看这个", agencyDynamic("明天20点直播")), textless)
	start, _ := s.GetDashboardRevision(ctx)
	gone := func(d bilisource.Dynamic) bilisource.Dynamic {
		d.Quote = &bilisource.Quote{ID: "bilibili-" + forwardedID, URL: "https://www.bilibili.com/opus/" + forwardedID, Unavailable: true}
		return d
	}
	newTextless := gone(biliForward("", nil))
	newTextless.IDStr = "900000000000000112"
	if !write(gone(biliForward("看这个", nil)), gone(textless), newTextless) {
		t.Fatal("an unavailable quote reported no change")
	}
	if after, _ := s.GetDashboardRevision(ctx); after != start+1 {
		t.Fatalf("revision %d -> %d", start, after)
	}
	posts := dashboardQuotes(t, s)
	for _, id := range []string{"bilibili-" + forwardID, "bilibili-900000000000000111"} {
		q := posts[id].Quote
		if q == nil || !q.Unavailable || q.ID != "bilibili-"+forwardedID || q.Text != "" || len(q.Media) != 0 {
			t.Fatalf("%s quote=%+v", id, q)
		}
		if got := linkedMedia(t, s.DB(), id, "quote-image"); got != "" {
			t.Fatalf("%s quote images=%s", id, got)
		}
		if raw := storedQuote(t, s.DB(), id); raw != `{"id":"bilibili-`+forwardedID+`","url":"https://www.bilibili.com/opus/`+forwardedID+`","unavailable":true}` {
			t.Fatalf("%s stored quote=%s", id, raw)
		}
	}
	if posts["bilibili-"+forwardID].Text != "看这个" || posts["bilibili-900000000000000111"].Text != "" {
		t.Fatal("forward wording changed")
	}
	var stored int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM posts WHERE id = 'bilibili-900000000000000112'`).Scan(&stored)
	if stored != 0 {
		t.Fatal("stored a new forward that would be an empty card")
	}
	var text, forwardedAt string
	if err := s.DB().QueryRow(`SELECT text, forwarded_at FROM bilibili_statuses WHERE id = ?`, "bilibili-"+forwardedID).Scan(&text, &forwardedAt); err != nil || text != "明天20点直播" || forwardedAt != "2026-11-07T05:40:00Z" {
		t.Fatalf("pool row=%q %q err=%v", text, forwardedAt, err)
	}
	// The same read again writes nothing.
	if write(gone(biliForward("看这个", nil)), gone(textless), newTextless) {
		t.Fatal("an unchanged read reported a change")
	}
	if after, _ := s.GetDashboardRevision(ctx); after != start+1 {
		t.Fatalf("revision moved on an unchanged read: %d -> %d", start, after)
	}
}

func TestBilibiliQuotesAreValidatedOnBilibiliPostsOnly(t *testing.T) {
	s := biliStore(t)
	insert := func(id, source, url, quote string) {
		t.Helper()
		if _, err := s.DB().Exec(`INSERT INTO posts (id,source,account_handle,type,text,published_at,url,quote_json) VALUES (?,?,'316381099','daily','text','2026-11-07T00:00:00Z',?,?)`, id, source, url, quote); err != nil {
			t.Fatal(err)
		}
	}
	opus := func(id string) string { return "https://www.bilibili.com/opus/" + id }
	cases := map[string]struct {
		quote string
		ok    bool
	}{
		"bilibili-900000000000000301": {`{"id":"bilibili-900000000000000401","authorHandle":"42","authorName":"A","text":"t","url":"` + opus("900000000000000401") + `"}`, true},
		"bilibili-900000000000000302": {`{"id":"bilibili-900000000000000402","authorHandle":"42","text":"video","url":"https://www.bilibili.com/video/BV1AAAAAAAAA"}`, true},
		"bilibili-900000000000000303": {`{"id":"","url":"` + opus("900000000000000303") + `","unavailable":true}`, true},
		"bilibili-900000000000000304": {`{"id":"bilibili-900000000000000404","url":"` + opus("900000000000000404") + `","unavailable":true}`, true},
		// Another dynamic's page, a foreign host, an unknown ID linking
		// elsewhere, an unavailable quote linking to a video (the parser
		// gives an unavailable upload its opus page), an X status.
		"bilibili-900000000000000305": {`{"id":"bilibili-900000000000000405","text":"t","url":"` + opus("900000000000000999") + `"}`, false},
		"bilibili-900000000000000306": {`{"id":"bilibili-900000000000000406","text":"t","url":"https://bilibili.com.invalid/opus/900000000000000406"}`, false},
		"bilibili-900000000000000307": {`{"id":"","url":"` + opus("900000000000000999") + `","unavailable":true}`, false},
		"bilibili-900000000000000308": {`{"id":"bilibili-900000000000000408","url":"https://www.bilibili.com/video/BV1AAAAAAAAA","unavailable":true}`, false},
		"bilibili-900000000000000309": {`{"id":"1900000000000000409","text":"t","url":"https://x.com/someone/status/1900000000000000409"}`, false},
		"bilibili-900000000000000310": {`{"id":"bilibili-900000000000000410","text":"t","url":"` + opus("900000000000000410") + `?share=1"}`, false},
	}
	for id, c := range cases {
		insert(id, "bilibili", opus(strings.TrimPrefix(id, "bilibili-")), c.quote)
	}
	// An X post never shows a Bilibili projection.
	insert("1900000000000000311", "x", "https://x.com/kano/status/1900000000000000311", `{"id":"bilibili-900000000000000411","text":"t","url":"`+opus("900000000000000411")+`"}`)
	posts := dashboardQuotes(t, s)
	for id, c := range cases {
		if q := posts[id].Quote; (q != nil) != c.ok {
			t.Fatalf("%s quote=%+v want shown=%v", id, q, c.ok)
		}
	}
	if q := posts["1900000000000000311"].Quote; q != nil {
		t.Fatalf("x post showed a bilibili quote: %+v", q)
	}
	if q := posts["bilibili-900000000000000303"].Quote; q.ID != "" || !q.Unavailable || q.URL != opus("900000000000000303") {
		t.Fatalf("unknown forwarded dynamic=%+v", q)
	}
}

func TestBilibiliForwardQuotesTranslateOnceThroughThePool(t *testing.T) {
	s := biliStore(t)
	ctx := context.Background()
	own := biliDynamic("今天20点开播")
	own.IDStr = ownID
	selfForward := biliForward("", &bilisource.Quote{ID: own.PostID(), AuthorHandle: biliAccount, AuthorName: "Kano", Text: "今天20点开播",
		PublishedAt: own.PublishedAt, URL: own.URL()})
	selfForward.IDStr = "900000000000000111"
	other := biliForward("看这个", agencyDynamic("明天20点直播"))
	if _, err := s.WriteBilibiliDynamics(ctx, biliAccount, bilisource.DynamicsResult{Dynamics: []bilisource.Dynamic{own, selfForward, other}, State: biliState()}); err != nil {
		t.Fatal(err)
	}
	// An X status pooled for a quote, to show the order.
	if _, err := s.DB().Exec(`INSERT INTO x_statuses(id,author_handle,text,url,quoted_at,first_seen_at,updated_at) VALUES('1900000000000000205','agency_synthetic','告知','https://x.com/agency_synthetic/status/1900000000000000205','2026-11-07T06:00:00.000Z','2026-11-07T06:00:00.000Z','2026-11-07T06:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	candidates, err := s.ListTranslationCandidates(ctx, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), 10)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, c := range candidates {
		if c.OwnerType != TranslationOwnerPost {
			got = append(got, c.OwnerType+":"+c.OwnerID+":"+c.Source)
		}
	}
	// The self-forwarded dynamic is a stored post and is not pooled for it.
	if want := []string{"x_status:1900000000000000205:x", "bilibili_status:bilibili-" + forwardedID + ":bilibili"}; strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("candidates=%v", got)
	}
	var pooled int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM bilibili_statuses`).Scan(&pooled)
	if pooled != 2 {
		t.Fatalf("pooled=%d", pooled)
	}
	// A Chinese dynamic needs ja as well, until detected.
	summary, err := s.AdminTranslations(ctx, TranslationOwnerBilibiliStatus, []string{"bilibili-" + forwardedID})
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range summary["bilibili-"+forwardedID].Locales {
		if locale.Status != "missing" {
			t.Fatalf("locale=%+v", locale)
		}
	}
	saved, err := s.SaveMachineTranslation(ctx, MachineTranslation{OwnerType: TranslationOwnerBilibiliStatus, OwnerID: "bilibili-" + forwardedID, SourceHash: TranslationHash("明天20点直播"),
		SourceLanguage: "zh", Texts: map[string]string{"en": "Live tomorrow at 20:00", "ja": "明日20時に配信"}, ProviderID: "p", ModelID: "m"}, time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC))
	if err != nil || !saved {
		t.Fatalf("saved=%v err=%v", saved, err)
	}
	if _, err = s.SaveMachineTranslation(ctx, MachineTranslation{OwnerType: TranslationOwnerPost, OwnerID: own.PostID(), SourceHash: TranslationHash("今天20点开播"),
		SourceLanguage: "zh", Texts: map[string]string{"en": "Live today at 20:00", "ja": "今日20時に配信"}, ProviderID: "p", ModelID: "m"}, time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	posts := dashboardQuotes(t, s)
	if q := posts["bilibili-"+forwardID].Quote; q == nil || q.Translations.Locales["en"].Text != "Live tomorrow at 20:00" || q.Translations.SourceLanguage == nil || *q.Translations.SourceLanguage != "zh" {
		t.Fatalf("pooled quote=%+v", q)
	}
	// A self-forward shows the stored post's translations.
	if q := posts["bilibili-900000000000000111"].Quote; q == nil || q.Translations.Locales["ja"].Text != "今日20時に配信" {
		t.Fatalf("self-forward quote=%+v", q)
	}
	// An X post quoting a status with the forwarded dynamic's ID digits
	// reads no Bilibili rows: the owner types are apart.
	if _, err = s.DB().Exec(`INSERT INTO translations (owner_type,owner_id,field,locale,origin,text,source_hash,created_at,updated_at) VALUES ('x_status','bilibili-` + forwardedID + `','text','en','manual','wrong owner','','','')`); err != nil {
		t.Fatal(err)
	}
	if q := dashboardQuotes(t, s)["bilibili-"+forwardID].Quote; q.Translations.Locales["en"].Text != "Live tomorrow at 20:00" {
		t.Fatalf("quote read another owner type: %+v", q)
	}
}
