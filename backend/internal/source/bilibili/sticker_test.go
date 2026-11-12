package bilibili

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestEmojiOnlyQuotesKeepVisibleCaptions(t *testing.T) {
	for _, tc := range []struct {
		name, kind, caption, icon, restriction string
		unavailable                            bool
	}{
		{name: "word with picture", kind: "DYNAMIC_TYPE_WORD", caption: "[smile]", icon: "https://i0.hdslb.com/bfs/emote/synthetic.png"},
		{name: "caption without picture", kind: "DYNAMIC_TYPE_WORD", caption: "[smile]"},
		{name: "unsafe picture keeps caption", kind: "DYNAMIC_TYPE_WORD", caption: "[smile]", icon: "https://images.invalid/sticker.png"},
		{name: "forward wording", kind: ForwardType, caption: "[smile]"},
		{name: "empty emoji is unavailable", kind: "DYNAMIC_TYPE_WORD", unavailable: true},
		{name: "hidden quote", kind: "DYNAMIC_TYPE_WORD", caption: "[smile]", restriction: `"visible":false,`, unavailable: true},
		{name: "supporters only", kind: "DYNAMIC_TYPE_WORD", caption: "[smile]", restriction: `"basic":{"is_only_fans":true},`, unavailable: true},
		{name: "deleted quote", kind: "DYNAMIC_TYPE_NONE", caption: "[smile]", unavailable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"items":[{"id_str":"900000000000000700","type":"DYNAMIC_TYPE_FORWARD","modules":{
			 "module_author":{"mid":316381099,"pub_ts":1794024288},"module_dynamic":{"desc":{"text":"forward"}}},
			 "orig":{"id_str":"900000000000000701","type":%q,%s"modules":{"module_author":{"mid":42,"pub_ts":1794000000},
			 "module_dynamic":{"desc":{"rich_text_nodes":[{"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":%q,"emoji":{"icon_url":%q}}]}}}}}]}`,
				tc.kind, tc.restriction, tc.caption, tc.icon)
			got, err := ParseSpaceFeed([]byte(body), DefaultAccount)
			if err != nil || len(got.Dynamics) != 1 {
				t.Fatalf("page=%#v err=%v", got, err)
			}
			q := got.Dynamics[0].Quote
			if q == nil || q.Unavailable != tc.unavailable || q.Text != "" {
				t.Fatalf("quote=%#v", q)
			}
			if tc.unavailable {
				if len(q.Nodes) != 0 || len(q.Stickers) != 0 {
					t.Fatalf("unavailable quote retained content: %#v", q)
				}
				return
			}
			if len(q.Nodes) != 1 || q.Nodes[0].Text != tc.caption || q.AuthorHandle != "42" || q.PublishedAt == "" {
				t.Fatalf("quote lost visible wording: %#v", q)
			}
			wantStickers := 0
			if tc.name == "word with picture" {
				wantStickers = 1
			}
			if len(q.Stickers) != wantStickers {
				t.Fatalf("stickers=%v", q.Stickers)
			}
		})
	}
}

// A sticker keeps its caption and picture in the raw nodes, never in the
// text, and its picture is listed once however often the dynamic uses it.
func TestStickersStayOutOfTheTextAndKeepTheirPictures(t *testing.T) {
	const page = `{"items":[
 {"id_str":"900000000000000030","type":"DYNAMIC_TYPE_WORD","modules":{"module_author":{"mid":316381099,"pub_ts":1794024288},"module_dynamic":{"desc":{"text":"早[笑]","rich_text_nodes":[
  {"type":"RICH_TEXT_NODE_TYPE_TEXT","text":"早"},
  {"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"[笑]","emoji":{"icon_url":"https://i0.hdslb.com/bfs/emote/synthetic-laugh.png","gif_url":"https://i0.hdslb.com/bfs/emote/synthetic-laugh.gif","webp_url":"https://i0.hdslb.com/bfs/emote/synthetic-laugh.webp","size":1,"type":1}},
  {"type":"RICH_TEXT_NODE_TYPE_TEXT","text":"安"},
  {"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"[笑]","emoji":{"icon_url":"https://i0.hdslb.com/bfs/emote/synthetic-laugh.png","webp_url":"https://i0.hdslb.com/bfs/emote/synthetic-laugh.webp"}},
  {"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"[哭]","emoji":{"icon_url":"http://i1.hdslb.com/bfs/emote/synthetic-cry.png"}},
  {"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"[外]","emoji":{"icon_url":"https://images.invalid/bfs/emote/synthetic-off.png"}},
  {"type":"RICH_TEXT_NODE_TYPE_TOPIC","text":"#话题#","emoji":{"icon_url":"https://i0.hdslb.com/bfs/emote/synthetic-topic.png"}}]}}}},
 {"id_str":"900000000000000031","type":"DYNAMIC_TYPE_FORWARD","modules":{"module_author":{"mid":316381099,"pub_ts":1794024000},"module_dynamic":{"desc":{"rich_text_nodes":[
  {"type":"RICH_TEXT_NODE_TYPE_TEXT","text":"转"},
  {"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"[哭]","emoji":{"icon_url":"https://i1.hdslb.com/bfs/emote/synthetic-cry.png"}}]}}},
  "orig":{"id_str":"900000000000000032","type":"DYNAMIC_TYPE_DRAW","modules":{"module_author":{"mid":42,"name":"synthetic","pub_ts":1794000000},"module_dynamic":{"major":{"type":"MAJOR_TYPE_OPUS","opus":{"title":"标题","summary":{"text":"x","rich_text_nodes":[
   {"type":"RICH_TEXT_NODE_TYPE_TEXT","text":"原文"},
   {"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"[赞]","emoji":{"gif_url":"https://i2.hdslb.com/bfs/emote/synthetic-like.gif","icon_url":"https://i2.hdslb.com/bfs/emote/synthetic-like.png"}}]},"pics":[]}}}}}}
]}`
	got, err := ParseSpaceFeed([]byte(page), DefaultAccount)
	if err != nil || len(got.Dynamics) != 2 {
		t.Fatalf("page=%#v err=%v", got, err)
	}
	word, forward := got.Dynamics[0], got.Dynamics[1]
	if word.Text != "早安#话题#" {
		t.Fatalf("text=%q", word.Text)
	}
	want := []string{"https://i0.hdslb.com/bfs/emote/synthetic-laugh.webp", "https://i1.hdslb.com/bfs/emote/synthetic-cry.png"}
	if strings.Join(word.Stickers, " ") != strings.Join(want, " ") {
		t.Fatalf("stickers=%#v", word.Stickers)
	}
	raw, _ := json.Marshal(word.Raw)
	for _, part := range []string{
		`{"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"[笑]","orig_text":"","emoji":{"url":"https://i0.hdslb.com/bfs/emote/synthetic-laugh.webp"}}`,
		`{"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"[外]","orig_text":""}`,
		`{"type":"RICH_TEXT_NODE_TYPE_TOPIC","text":"#话题#","orig_text":""}`,
	} {
		if !strings.Contains(string(raw), part) {
			t.Fatalf("raw=%s lacks %s", raw, part)
		}
	}
	if strings.Contains(string(raw), "images.invalid") || strings.Contains(string(raw), "synthetic-topic") || strings.Contains(string(raw), "http://") {
		t.Fatalf("raw kept an unsafe picture: %s", raw)
	}
	if forward.Text != "转" || len(forward.Stickers) != 1 || forward.Stickers[0] != "https://i1.hdslb.com/bfs/emote/synthetic-cry.png" {
		t.Fatalf("forward=%#v", forward)
	}
	q := forward.Quote
	if q == nil || q.Unavailable || q.Text != "标题\n原文" || q.Title != "标题" || len(q.Stickers) != 1 || q.Stickers[0] != "https://i2.hdslb.com/bfs/emote/synthetic-like.gif" {
		t.Fatalf("quote=%#v", q)
	}
	// The quote's nodes are kept beside its type and ID; the stored quote
	// projection carries none of them.
	raw, _ = json.Marshal(forward.Raw)
	if !strings.Contains(string(raw), `"orig":{"id":"900000000000000032","nodes":[{"type":"RICH_TEXT_NODE_TYPE_TEXT","text":"原文","orig_text":""},{"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"[赞]","orig_text":"","emoji":{"url":"https://i2.hdslb.com/bfs/emote/synthetic-like.gif"}}],"title":"标题","type":"DYNAMIC_TYPE_DRAW"}`) {
		t.Fatalf("raw=%s", raw)
	}
	projection, _ := json.Marshal(q)
	if strings.Contains(string(projection), "emote") || strings.Contains(string(projection), "nodes") {
		t.Fatalf("projection=%s", projection)
	}
}

func TestStickersAreBoundedPerDynamic(t *testing.T) {
	nodes := []string{}
	for i := range maxDynamicStickers + 8 {
		nodes = append(nodes, fmt.Sprintf(`{"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"[s%d]","emoji":{"icon_url":"https://i0.hdslb.com/bfs/emote/synthetic-%d.png"}}`, i, i))
	}
	page := `{"items":[{"id_str":"900000000000000040","type":"DYNAMIC_TYPE_WORD","modules":{"module_author":{"mid":316381099,"pub_ts":1794024288},"module_dynamic":{"desc":{"rich_text_nodes":[` + strings.Join(nodes, ",") + `]}}}}]}`
	got, err := ParseSpaceFeed([]byte(page), DefaultAccount)
	if err != nil || len(got.Dynamics) != 1 {
		t.Fatalf("page=%#v err=%v", got, err)
	}
	d := got.Dynamics[0]
	if d.Text != "" || len(d.Stickers) != maxDynamicStickers || d.Stickers[0] != "https://i0.hdslb.com/bfs/emote/synthetic-0.png" {
		t.Fatalf("dynamic=%#v", d)
	}
}

func TestOpusPageKeepsStickerPictures(t *testing.T) {
	page := strings.Replace(opusPage, `"rich":{"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"[synthetic_emoji]"}`,
		`"rich":{"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"[synthetic_emoji]","emoji":{"icon_url":"//i0.hdslb.com/bfs/emote/synthetic-e.png"}}`, 1)
	d, err := ParseOpusPage([]byte(page), "900000000000000010")
	if err != nil || d.Text != "大家好！今天北京时间17点上传。\n#话题#" || len(d.Stickers) != 1 || d.Stickers[0] != "https://i0.hdslb.com/bfs/emote/synthetic-e.png" {
		t.Fatalf("dynamic=%#v err=%v", d, err)
	}
	nodes, _ := d.Raw["nodes"].([]richNode)
	// The paragraphs' line break is a node of its own.
	if len(nodes) != 4 || nodes[1].Emoji == nil || nodes[1].Emoji.URL != d.Stickers[0] || nodes[0].Emoji != nil || nodes[2].Text != "\n" || nodes[2].Type == EmojiNode {
		t.Fatalf("nodes=%#v", nodes)
	}
}
