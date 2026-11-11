package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
)

// The older-listing fallback changes the stored wording of some dynamics a
// listing without rich-text nodes gave: an album with an opus block but no
// major type takes its opus summary instead of desc.text, plain text is
// HTML-unescaped, and a forwarded forward with plain text only becomes an
// available quote with a pool row. The rows below are what the parse stored
// before the fallback; the next read rewrites them once, so their wording is
// translated (and extracted) again, and reading again changes nothing.
func TestOlderListingFallbackRewritesStoredWordingOnce(t *testing.T) {
	s := biliStore(t)
	ctx := context.Background()
	const (
		albumID   = "900000000000000501"
		plainID   = "900000000000000502"
		forwardID = "900000000000000503"
		forwarded = "900000000000000509"
	)
	before := []bilisource.Dynamic{
		{IDStr: albumID, OwnerMid: 316381099, Text: "今天的直播 &amp; 歌单", PublishedAt: "2026-11-07T04:00:00Z", Images: []string{}, Raw: map[string]any{"type": "DYNAMIC_TYPE_DRAW"}},
		{IDStr: plainID, OwnerMid: 316381099, Text: "早 &amp; 晚", PublishedAt: "2026-11-07T03:00:00Z", Images: []string{}, Raw: map[string]any{"type": "DYNAMIC_TYPE_WORD"}},
		{IDStr: forwardID, OwnerMid: 316381099, Text: "看", PublishedAt: "2026-11-07T02:00:00Z", Images: []string{},
			Quote: &bilisource.Quote{ID: "bilibili-" + forwarded, URL: "https://www.bilibili.com/opus/" + forwarded, Unavailable: true},
			Raw:   map[string]any{"type": bilisource.ForwardType, "orig": map[string]any{"id": forwarded, "type": bilisource.ForwardType}}},
	}
	if _, err := s.WriteBilibiliDynamics(ctx, biliAccount, bilisource.DynamicsResult{Dynamics: before, State: biliState()}); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	for id, text := range map[string]string{albumID: "今天的直播 &amp; 歌单", plainID: "早 &amp; 晚", forwardID: "看"} {
		if _, err := s.SaveMachineTranslation(ctx, MachineTranslation{OwnerType: TranslationOwnerPost, OwnerID: "bilibili-" + id, SourceHash: TranslationHash(text),
			SourceLanguage: "zh", Texts: map[string]string{"en": "synthetic en", "ja": "synthetic ja"}, ProviderID: "p", ModelID: "m"}, when); err != nil {
			t.Fatal(err)
		}
	}
	candidates := func() string {
		t.Helper()
		list, err := s.ListTranslationCandidates(ctx, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), 50)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, c := range list {
			out = append(out, c.OwnerType+":"+c.OwnerID)
		}
		return strings.Join(out, " ")
	}
	if got := candidates(); got != "" {
		t.Fatalf("candidates before=%q", got)
	}
	var pooled int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM bilibili_statuses`).Scan(&pooled)
	if pooled != 0 {
		t.Fatalf("pooled before=%d", pooled)
	}

	listing := `{"items":[
 {"id_str":"` + albumID + `","type":"DYNAMIC_TYPE_DRAW","modules":{"module_author":{"mid":316381099,"pub_ts":1794024000},
   "module_dynamic":{"desc":{"text":"今天的直播 &amp; 歌单"},"major":{"opus":{"summary":{"text":"今週のスケジュールです"},"pics":[{"url":"https://i0.hdslb.com/bfs/new_dyn/synthetic-old.jpg"}]}}}}},
 {"id_str":"` + plainID + `","type":"DYNAMIC_TYPE_WORD","modules":{"module_author":{"mid":316381099,"pub_ts":1794020400},
   "module_dynamic":{"desc":{"text":"早 &amp; 晚"}}}},
 {"id_str":"` + forwardID + `","type":"DYNAMIC_TYPE_FORWARD","modules":{"module_author":{"mid":316381099,"pub_ts":1794016800},
   "module_dynamic":{"desc":{"rich_text_nodes":[{"type":"RICH_TEXT_NODE_TYPE_TEXT","text":"看"}]}}},
  "orig":{"id_str":"` + forwarded + `","type":"DYNAMIC_TYPE_FORWARD","modules":{"module_author":{"mid":42,"name":"Synthetic Agency","pub_ts":1794000000},
   "module_dynamic":{"desc":{"text":"转 &amp; 发"}}}}}
]}`
	page, err := bilisource.ParseSpaceFeed([]byte(listing), biliAccount)
	if err != nil || len(page.Dynamics) != 3 {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	revision, _ := s.GetDashboardRevision(ctx)
	changed, err := s.WriteBilibiliDynamics(ctx, biliAccount, bilisource.DynamicsResult{Dynamics: page.Dynamics, State: biliState()})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if after, _ := s.GetDashboardRevision(ctx); after != revision+1 {
		t.Fatalf("revision %d->%d", revision, after)
	}
	text := func(id string) string {
		var v string
		if err := s.DB().QueryRow(`SELECT text FROM posts WHERE id=?`, "bilibili-"+id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if got := text(albumID); got != "今週のスケジュールです" {
		t.Fatalf("album text=%q", got)
	}
	if got := linkedMedia(t, s.DB(), "bilibili-"+albumID, "post-image"); got != "https://i0.hdslb.com/bfs/new_dyn/synthetic-old.jpg" {
		t.Fatalf("album images=%q", got)
	}
	if got := text(plainID); got != "早 & 晚" {
		t.Fatalf("plain text=%q", got)
	}
	if got := text(forwardID); got != "看" {
		t.Fatalf("forward text=%q", got)
	}
	q := dashboardQuotes(t, s)["bilibili-"+forwardID].Quote
	if q == nil || q.Unavailable || q.Text != "转 & 发" || q.AuthorName != "Synthetic Agency" {
		t.Fatalf("quote=%#v", q)
	}
	var poolText string
	if err = s.DB().QueryRow(`SELECT text FROM bilibili_statuses WHERE id=?`, "bilibili-"+forwarded).Scan(&poolText); err != nil || poolText != "转 & 发" {
		t.Fatalf("pool=%q err=%v", poolText, err)
	}
	// The rewritten wording is translated again: the album, the plain post and
	// the newly pooled dynamic; the forward's own wording is unchanged.
	if got, want := candidates(), "post:bilibili-"+albumID+" post:bilibili-"+plainID+" bilibili_status:bilibili-"+forwarded; got != want {
		t.Fatalf("candidates after=%q want %q", got, want)
	}
	if changed, err = s.WriteBilibiliDynamics(ctx, biliAccount, bilisource.DynamicsResult{Dynamics: page.Dynamics, State: biliState()}); err != nil || changed {
		t.Fatalf("repeat changed=%v err=%v", changed, err)
	}
}
