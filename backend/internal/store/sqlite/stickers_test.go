package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
)

const (
	stickerLaugh = "https://i0.hdslb.com/bfs/emote/synthetic-laugh.png"
	stickerCry   = "https://i0.hdslb.com/bfs/emote/synthetic-cry.png"
	stickerLike  = "https://i0.hdslb.com/bfs/emote/synthetic-like.png"
)

func TestEmojiOnlyForwardsSurviveTheSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name, ownCaption, quoteCaption, icon string
		keep                                 bool
	}{
		{name: "quote sticker", quoteCaption: "[smile]", icon: stickerLaugh, keep: true},
		{name: "quote caption without picture", quoteCaption: "[smile]", keep: true},
		{name: "own sticker with unavailable quote", ownCaption: "[smile]", icon: stickerLaugh, keep: true},
		{name: "own caption with unavailable quote", ownCaption: "[smile]", keep: true},
		{name: "empty forward with unavailable quote"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := biliStore(t)
			ctx := context.Background()
			body := fmt.Sprintf(`{"items":[{"id_str":"900000000000000700","type":"DYNAMIC_TYPE_FORWARD","modules":{
			 "module_author":{"mid":316381099,"pub_ts":1794024288},"module_dynamic":{"desc":{"rich_text_nodes":[
			 {"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":%q,"emoji":{"icon_url":%q}}]}}},
			 "orig":{"id_str":"900000000000000701","type":"DYNAMIC_TYPE_WORD","modules":{"module_author":{"mid":42,"pub_ts":1794000000},
			 "module_dynamic":{"desc":{"rich_text_nodes":[{"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":%q,"emoji":{"icon_url":%q}}]}}}}}]}`,
				tc.ownCaption, tc.icon, tc.quoteCaption, tc.icon)
			page, err := bilisource.ParseSpaceFeed([]byte(body), biliAccount)
			if err != nil || len(page.Dynamics) != 1 {
				t.Fatalf("page=%#v err=%v", page, err)
			}
			result := bilisource.DynamicsResult{Dynamics: page.Dynamics, State: biliState()}
			changed, err := s.WriteBilibiliDynamics(ctx, biliAccount, result)
			if err != nil || changed != tc.keep {
				t.Fatalf("changed=%v err=%v", changed, err)
			}
			post, exists := dashboardQuotes(t, s)["bilibili-900000000000000700"]
			if exists != tc.keep {
				t.Fatalf("post stored=%v want=%v", exists, tc.keep)
			}
			if !tc.keep {
				return
			}
			if post.Text != "" || post.Quote == nil || post.Quote.Unavailable != (tc.quoteCaption == "") {
				t.Fatalf("post=%#v", post)
			}
			if tc.ownCaption != "" && richString(post.Rich) != "<[smile]>" || tc.quoteCaption != "" && richString(post.Quote.Rich) != "<[smile]>" {
				t.Fatalf("own rich=%q quote rich=%q", richString(post.Rich), richString(post.Quote.Rich))
			}
			if tc.quoteCaption != "" {
				var text string
				if err := s.DB().QueryRow(`SELECT text FROM bilibili_statuses WHERE id='bilibili-900000000000000701'`).Scan(&text); err != nil || text != "" {
					t.Fatalf("quoted status text=%q err=%v", text, err)
				}
			}
			if changed, err = s.WriteBilibiliDynamics(ctx, biliAccount, result); err != nil || changed {
				t.Fatalf("repeat changed=%v err=%v", changed, err)
			}
			if tc.icon != "" {
				role, rich := rolePostEmoji, post.Rich
				if tc.quoteCaption != "" {
					role, rich = roleQuoteEmoji, post.Quote.Rich
				}
				if got := linkedMedia(t, s.DB(), post.ID, role); got != tc.icon || len(rich) != 1 || rich[0].URL != nil {
					t.Fatalf("linked=%q rich=%#v", got, rich)
				}
				id := readySticker(t, s, tc.icon)
				post = dashboardQuotes(t, s)[post.ID]
				rich = post.Rich
				if tc.quoteCaption != "" {
					rich = post.Quote.Rich
				}
				if len(rich) != 1 || rich[0].URL == nil || !strings.HasPrefix(*rich[0].URL, "/media/"+id+"?v=") {
					t.Fatalf("cached rich=%#v", rich)
				}
			}
		})
	}
}

// stickerListing is a synthetic listing page: a word dynamic using [笑]
// twice and [哭] once, another using [笑], and a forward with its own [笑]
// of a dynamic using [赞]. Without pictures, its emoji nodes are what a
// dynamic stored before stickers were kept looks like.
func stickerListing(t *testing.T, pictures bool) []bilisource.Dynamic {
	t.Helper()
	emoji := func(caption, url string) string {
		if !pictures {
			return `{"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"` + caption + `"}`
		}
		return `{"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"` + caption + `","emoji":{"icon_url":"` + url + `"}}`
	}
	text := func(s string) string { return `{"type":"RICH_TEXT_NODE_TYPE_TEXT","text":"` + s + `"}` }
	page := `{"items":[
 {"id_str":"900000000000000310","type":"DYNAMIC_TYPE_WORD","modules":{"module_author":{"mid":316381099,"pub_ts":1794024288},"module_dynamic":{"desc":{"rich_text_nodes":[` +
		text("早") + `,` + emoji("[笑]", stickerLaugh) + `,` + text("安 ") + `,` + emoji("[笑]", stickerLaugh) + `,` + emoji("[哭]", stickerCry) + `]}}}},
 {"id_str":"900000000000000311","type":"DYNAMIC_TYPE_WORD","modules":{"module_author":{"mid":316381099,"pub_ts":1794020000},"module_dynamic":{"desc":{"rich_text_nodes":[` +
		emoji("[笑]", stickerLaugh) + `]}}}},
 {"id_str":"900000000000000312","type":"DYNAMIC_TYPE_FORWARD","modules":{"module_author":{"mid":316381099,"pub_ts":1794010000},"module_dynamic":{"desc":{"rich_text_nodes":[` +
		text("转") + `,` + emoji("[笑]", stickerLaugh) + `]}}},
  "orig":{"id_str":"900000000000000313","type":"DYNAMIC_TYPE_WORD","modules":{"module_author":{"mid":42,"name":"Synthetic Agency","pub_ts":1794000000},"module_dynamic":{"desc":{"rich_text_nodes":[` +
		text("明天直播") + `,` + emoji("[赞]", stickerLike) + `]}}}}}
]}`
	got, err := bilisource.ParseSpaceFeed([]byte(page), biliAccount)
	if err != nil || len(got.Dynamics) != 3 {
		t.Fatalf("page=%#v err=%v", got, err)
	}
	return got.Dynamics
}

func readySticker(t *testing.T, s *Store, url string) string {
	t.Helper()
	id, _, _ := mediaIdentity(url)
	if err := s.SaveMediaDownload(context.Background(), PendingMedia{ID: id, Source: "bilibili", SourceURL: url}, syntheticImage(t, "image/png", 20, 20, true), "image/png", "", "", "ready", ""); err != nil {
		t.Fatal(err)
	}
	return id
}

// richString renders segments as text with stickers as <caption|url>.
func richString(segments []RichSegment) string {
	var b strings.Builder
	for _, s := range segments {
		switch {
		case s.Emoji == "":
			b.WriteString(s.Text)
		case s.URL == nil:
			b.WriteString("<" + s.Emoji + ">")
		default:
			b.WriteString("<" + s.Emoji + "|" + *s.URL + ">")
		}
	}
	return b.String()
}

func richText(segments []RichSegment) string {
	var b strings.Builder
	for _, s := range segments {
		b.WriteString(s.Text)
	}
	return b.String()
}

func TestBilibiliStickersAreCachedMediaShownInTheOriginalWording(t *testing.T) {
	s := biliStore(t)
	ctx := context.Background()
	dynamics := stickerListing(t, true)
	if _, err := s.WriteBilibiliDynamics(ctx, biliAccount, bilisource.DynamicsResult{Dynamics: dynamics, State: biliState()}); err != nil {
		t.Fatal(err)
	}
	// One asset per picture, however many posts and roles link it.
	var assets int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM media_assets WHERE source='bilibili' AND source_url LIKE '%/bfs/emote/%'`).Scan(&assets)
	if assets != 3 {
		t.Fatalf("sticker assets=%d", assets)
	}
	for _, c := range []struct{ post, role, want string }{
		{"bilibili-900000000000000310", "post-emoji", stickerLaugh + " " + stickerCry},
		{"bilibili-900000000000000311", "post-emoji", stickerLaugh},
		{"bilibili-900000000000000312", "post-emoji", stickerLaugh},
		{"bilibili-900000000000000312", "quote-emoji", stickerLike},
		{"bilibili-900000000000000310", "post-image", ""},
		{"bilibili-900000000000000312", "quote-image", ""},
	} {
		if got := linkedMedia(t, s.DB(), c.post, c.role); got != c.want {
			t.Fatalf("%s %s=%q", c.post, c.role, got)
		}
	}
	// Stickers are never schedule-image candidates.
	var candidates int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM assets WHERE kind = 'schedule'`).Scan(&candidates)
	if candidates != 0 {
		t.Fatalf("schedule candidates=%d", candidates)
	}

	// Before a picture is cached, a sticker is its caption alone.
	posts := dashboardQuotes(t, s)
	word := posts["bilibili-900000000000000310"]
	if word.Text != "早安" || richString(word.Rich) != "早<[笑]>安<[笑]><[哭]>" || richText(word.Rich) != word.Text {
		t.Fatalf("word=%q rich=%q", word.Text, richString(word.Rich))
	}
	if only := posts["bilibili-900000000000000311"]; only.Text != "" || richString(only.Rich) != "<[笑]>" {
		t.Fatalf("emoji-only rich=%q", richString(only.Rich))
	}

	before, _ := s.GetDashboardRevision(ctx)
	laugh := readySticker(t, s, stickerLaugh)
	if after, _ := s.GetDashboardRevision(ctx); after <= before {
		t.Fatal("a cached sticker left the revision")
	}
	var hash string
	_ = s.DB().QueryRow(`SELECT sha256 FROM media_assets WHERE id=?`, laugh).Scan(&hash)
	local := "/media/" + laugh + "?v=" + hash
	posts = dashboardQuotes(t, s)
	word = posts["bilibili-900000000000000310"]
	if richString(word.Rich) != "早<[笑]|"+local+">安<[笑]|"+local+"><[哭]>" || richText(word.Rich) != word.Text {
		t.Fatalf("rich=%q", richString(word.Rich))
	}
	forward := posts["bilibili-900000000000000312"]
	if richString(forward.Rich) != "转<[笑]|"+local+">" || forward.Quote == nil || richString(forward.Quote.Rich) != "明天直播<[赞]>" || richText(forward.Quote.Rich) != forward.Quote.Text {
		t.Fatalf("forward rich=%q quote=%#v", richString(forward.Rich), forward.Quote)
	}
	like := readySticker(t, s, stickerLike)
	if q := dashboardQuotes(t, s)["bilibili-900000000000000312"].Quote; q == nil || len(q.Rich) != 2 || q.Rich[1].URL == nil || !strings.HasPrefix(*q.Rich[1].URL, "/media/"+like+"?v=") {
		t.Fatalf("quote rich=%#v", q)
	}
	// The archive reads the same segments.
	archive, err := s.GetPostArchive(ctx, "2026-11-01", "2026-11-30", "", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range archive.Items {
		if p.ID == "bilibili-900000000000000310" && richString(p.Rich) != richString(word.Rich) {
			t.Fatalf("archive rich=%q", richString(p.Rich))
		}
	}
	// A hidden quote takes its stickers with it.
	if _, err = s.DB().Exec(`UPDATE posts SET quote_hidden_at='2026-11-08T00:00:00Z' WHERE id='bilibili-900000000000000312'`); err != nil {
		t.Fatal(err)
	}
	if p := dashboardQuotes(t, s)["bilibili-900000000000000312"]; p.Quote != nil || richString(p.Rich) != "转<[笑]|"+local+">" {
		t.Fatalf("hidden quote=%#v", p.Quote)
	}
}

// Keeping sticker pictures changes no wording: a dynamic stored before they
// were kept keeps its text and translations when it is read again, so
// nothing is translated again.
func TestStickerPicturesLeaveTextAndTranslationsAlone(t *testing.T) {
	s := biliStore(t)
	ctx := context.Background()
	if _, err := s.WriteBilibiliDynamics(ctx, biliAccount, bilisource.DynamicsResult{Dynamics: stickerListing(t, false), State: biliState()}); err != nil {
		t.Fatal(err)
	}
	if got := linkedMedia(t, s.DB(), "bilibili-900000000000000310", "post-emoji"); got != "" {
		t.Fatalf("stickers without pictures linked %q", got)
	}
	when := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	for id, text := range map[string]string{"bilibili-900000000000000310": "早安", "bilibili-900000000000000312": "转"} {
		if _, err := s.SaveMachineTranslation(ctx, MachineTranslation{OwnerType: TranslationOwnerPost, OwnerID: id, SourceHash: TranslationHash(text),
			SourceLanguage: "zh", Texts: map[string]string{"en": "synthetic en", "ja": "synthetic ja"}, ProviderID: "p", ModelID: "m"}, when); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SaveMachineTranslation(ctx, MachineTranslation{OwnerType: TranslationOwnerBilibiliStatus, OwnerID: "bilibili-900000000000000313", SourceHash: TranslationHash("明天直播"),
		SourceLanguage: "zh", Texts: map[string]string{"en": "synthetic en", "ja": "synthetic ja"}, ProviderID: "p", ModelID: "m"}, when); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var b strings.Builder
		rows, err := s.DB().Query(`SELECT id||':'||text FROM posts UNION ALL SELECT id||':'||text FROM bilibili_statuses
			UNION ALL SELECT owner_type||':'||owner_id||':'||locale||':'||source_hash||':'||text FROM translations ORDER BY 1`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var line string
			_ = rows.Scan(&line)
			b.WriteString(line + "\n")
		}
		candidates, err := s.ListTranslationCandidates(ctx, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), 50)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range candidates {
			b.WriteString("candidate " + c.OwnerType + ":" + c.OwnerID + ":" + c.SourceHash + "\n")
		}
		return b.String()
	}
	stored := snapshot()
	revision, _ := s.GetDashboardRevision(ctx)
	changed, err := s.WriteBilibiliDynamics(ctx, biliAccount, bilisource.DynamicsResult{Dynamics: stickerListing(t, true), State: biliState()})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if after, _ := s.GetDashboardRevision(ctx); after <= revision {
		t.Fatal("newly linked stickers left the revision")
	}
	if again := snapshot(); again != stored {
		t.Fatalf("wording or translations changed:\n%s\n---\n%s", stored, again)
	}
	if strings.Contains(stored, "candidate post:bilibili-900000000000000310") {
		t.Fatalf("translated post is a candidate:\n%s", stored)
	}
	if got := linkedMedia(t, s.DB(), "bilibili-900000000000000310", "post-emoji"); got != stickerLaugh+" "+stickerCry {
		t.Fatalf("stickers=%q", got)
	}
	// Reading the same listing again changes nothing.
	if changed, err = s.WriteBilibiliDynamics(ctx, biliAccount, bilisource.DynamicsResult{Dynamics: stickerListing(t, true), State: biliState()}); err != nil || changed {
		t.Fatalf("repeat changed=%v err=%v", changed, err)
	}
}

func TestPlaceStickersCutsTheStoredText(t *testing.T) {
	emoji := func(caption, url string) storedNode {
		n := storedNode{Type: bilisource.EmojiNode, Text: caption}
		if url != "" {
			n.Emoji = &struct {
				URL string `json:"url"`
			}{URL: url}
		}
		return n
	}
	word := func(s string) storedNode { return storedNode{Type: "RICH_TEXT_NODE_TYPE_TEXT", Text: s} }
	readyID, _, _ := mediaIdentity(stickerLaugh)
	ready := map[string]string{readyID: "/media/" + readyID + "?v=0"}
	local := "/media/" + readyID + "?v=0"
	for _, c := range []struct {
		name, text, title string
		nodes             []storedNode
		want              string
	}{
		{"none", "早安", "", []storedNode{word("早安")}, ""},
		{"emoji only", "", "", []storedNode{emoji("[笑]", stickerLaugh), emoji("[哭]", "")}, "<[笑]|" + local + "><[哭]>"},
		{"trimmed edges", "早 安", "", []storedNode{word(" 早 "), emoji("[笑]", stickerLaugh), word("安\n")}, "早 <[笑]|" + local + ">安"},
		{"trailing sticker", "早", "", []storedNode{word("早 "), emoji("[笑]", "")}, "早<[笑]>"},
		{"title", "早\n早安", "早", []storedNode{word("早"), emoji("[笑]", ""), word("安")}, "早\n早<[笑]>安"},
		{"newlines", "a\nb", "", []storedNode{word("a\r\n"), emoji("[哭]", stickerCry), word("b")}, "a\n<[哭]>b"},
		{"cut by the bound", "早安", "", []storedNode{word("早"), emoji("[笑]", ""), word("安晚安"), emoji("[哭]", "")}, "早<[笑]>安"},
		{"empty caption", "早", "", []storedNode{word("早"), emoji("", stickerLaugh)}, ""},
		// An opus page's paragraphs are joined by a line break node.
		{"paragraph opener", "abc\ndef", "", []storedNode{word("abc"), word("\n"), emoji("[e]", ""), word("def")}, "abc\n<[e]>def"},
		{"sticker paragraph", "abc\n\ndef", "", []storedNode{word("abc"), word("\n"), emoji("[e]", ""), word("\n"), word("def")}, "abc\n<[e]>\ndef"},
		{"title and stickers alone", "T", "T", []storedNode{emoji("[e]", "")}, "T<[e]>"},
		// A node matches only where the last one ended.
		{"anchored first node", "早安 早", "", []storedNode{word(" 早"), emoji("[笑]", ""), word("安 早")}, "早<[笑]>安 早"},
		{"no skipping ahead", "早安晚", "", []storedNode{word("早"), word("晚"), emoji("[笑]", "")}, ""},
	} {
		got := placeStickers(c.text, c.title, c.nodes, ready)
		if richString(got) != c.want || got != nil && richText(got) != c.text {
			t.Fatalf("%s: got %q (%q)", c.name, richString(got), richText(got))
		}
	}
}

// An opus page's stickers keep their paragraphs, and a sticker body under a
// title follows the title.
func TestOpusPageStickersKeepParagraphsAndTitle(t *testing.T) {
	s := biliStore(t)
	ctx := context.Background()
	page := func(id, modules string) []byte {
		return []byte(`<html><script>window.__INITIAL_STATE__={"detail":{"id_str":"` + id + `","basic":{"comment_type":17},"modules":[
 {"module_type":"MODULE_TYPE_AUTHOR","module_author":{"mid":316381099,"pub_ts":1794024288}},` + modules + `]}};</script></html>`)
	}
	sticker := `{"type":"TEXT_NODE_TYPE_RICH","rich":{"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"[e]","emoji":{"icon_url":"` + stickerLaugh + `"}}}`
	paragraphs := page("900000000000000601", `{"module_type":"MODULE_TYPE_CONTENT","module_content":{"paragraphs":[
  {"para_type":1,"text":{"nodes":[{"type":"TEXT_NODE_TYPE_WORD","word":{"words":"abc"}}]}},
  {"para_type":1,"text":{"nodes":[`+sticker+`,{"type":"TEXT_NODE_TYPE_WORD","word":{"words":"def"}}]}},
  {"para_type":1,"text":{"nodes":[`+sticker+`]}},
  {"para_type":1,"text":{"nodes":[{"type":"TEXT_NODE_TYPE_WORD","word":{"words":"ghi"}}]}}]}}`)
	titled := page("900000000000000602", `{"module_type":"MODULE_TYPE_TITLE","module_title":{"text":"T"}},
 {"module_type":"MODULE_TYPE_CONTENT","module_content":{"paragraphs":[{"para_type":1,"text":{"nodes":[`+sticker+`]}}]}}`)
	for _, c := range []struct {
		id   string
		body []byte
		want string
	}{
		{"900000000000000601", paragraphs, "abc\n<[e]>def\n<[e]>\nghi"},
		{"900000000000000602", titled, "T<[e]>"},
	} {
		d, err := bilisource.ParseOpusPage(c.body, c.id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.WriteBilibiliDynamic(ctx, biliAccount, d, biliState()); err != nil {
			t.Fatal(err)
		}
		p := dashboardQuotes(t, s)["bilibili-"+c.id]
		if got := richString(p.Rich); got != c.want || richText(p.Rich) != p.Text {
			t.Fatalf("%s rich=%q text=%q", c.id, got, p.Text)
		}
	}
}

// A sticker that moves within unchanged text changes what the board shows,
// so the revision moves once; reading the same dynamic again does not.
func TestAMovedStickerAdvancesTheRevision(t *testing.T) {
	s := biliStore(t)
	ctx := context.Background()
	read := func(nodes string) (bool, int64) {
		t.Helper()
		got, err := bilisource.ParseSpaceFeed([]byte(`{"items":[{"id_str":"900000000000000610","type":"DYNAMIC_TYPE_WORD","modules":{"module_author":{"mid":316381099,"pub_ts":1794024288},"module_dynamic":{"desc":{"rich_text_nodes":[`+nodes+`]}}}}]}`), biliAccount)
		if err != nil {
			t.Fatal(err)
		}
		changed, err := s.WriteBilibiliDynamics(ctx, biliAccount, bilisource.DynamicsResult{Dynamics: got.Dynamics, State: biliState()})
		if err != nil {
			t.Fatal(err)
		}
		revision, _ := s.GetDashboardRevision(ctx)
		return changed, revision
	}
	laugh := `{"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"[笑]","emoji":{"icon_url":"` + stickerLaugh + `"}}`
	between := `{"type":"RICH_TEXT_NODE_TYPE_TEXT","text":"早"},` + laugh + `,{"type":"RICH_TEXT_NODE_TYPE_TEXT","text":"安"}`
	after := `{"type":"RICH_TEXT_NODE_TYPE_TEXT","text":"早安"},` + laugh
	_, first := read(between)
	if changed, revision := read(between); changed || revision != first {
		t.Fatalf("unchanged read changed=%v revision=%d->%d", changed, first, revision)
	}
	changed, moved := read(after)
	if !changed || moved != first+1 {
		t.Fatalf("moved sticker changed=%v revision=%d->%d", changed, first, moved)
	}
	if richString(dashboardQuotes(t, s)["bilibili-900000000000000610"].Rich) != "早安<[笑]>" {
		t.Fatal("the board kept the old placement")
	}
	if changed, revision := read(after); changed || revision != moved {
		t.Fatalf("repeat changed=%v revision=%d->%d", changed, moved, revision)
	}
}
