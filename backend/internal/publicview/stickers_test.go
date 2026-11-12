package publicview

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// Stickers reach a public read only as their caption and the local address
// of a ready cached picture; wording without stickers has no rich key.
func TestPublicStickersGiveOnlyLocalAddresses(t *testing.T) {
	local := "/media/" + strings.Repeat("a", 64) + "?v=" + strings.Repeat("b", 64)
	remote := "https://i0.hdslb.com/bfs/emote/synthetic-laugh.png"
	post := sqlite.Post{ID: "bilibili-900000000000000310", Source: "bilibili", Text: "早安", Media: []sqlite.PostMedia{},
		Rich: []sqlite.RichSegment{{Text: "早"}, {Emoji: "[笑]", URL: &local}, {Text: "安"}, {Emoji: "[哭]"}, {Emoji: "[外]", URL: &remote}},
		Quote: &sqlite.PostQuote{ID: "bilibili-900000000000000313", AuthorHandle: "42", Text: "明天", URL: "https://www.bilibili.com/opus/900000000000000313", Media: []sqlite.PostMedia{},
			Rich: []sqlite.RichSegment{{Text: "明天"}, {Emoji: "[赞]"}}}}
	body, _ := json.Marshal(publicPost(post))
	var got struct {
		Text  string           `json:"text"`
		Rich  []map[string]any `json:"rich"`
		Quote struct {
			Rich []map[string]any `json:"rich"`
		} `json:"quote"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	rich, _ := json.Marshal(got.Rich)
	if string(rich) != `[{"text":"早"},{"emoji":"[笑]","url":"`+local+`"},{"text":"安"},{"emoji":"[哭]","url":null},{"emoji":"[外]","url":null}]` {
		t.Fatalf("rich=%s", rich)
	}
	quote, _ := json.Marshal(got.Quote.Rich)
	if string(quote) != `[{"text":"明天"},{"emoji":"[赞]","url":null}]` {
		t.Fatalf("quote rich=%s", quote)
	}
	if strings.Contains(string(body), "hdslb") {
		t.Fatalf("a remote sticker address reached the read: %s", body)
	}
	plain, _ := json.Marshal(publicPost(sqlite.Post{ID: "bilibili-900000000000000311", Source: "bilibili", Text: "早安", Media: []sqlite.PostMedia{},
		Quote: &sqlite.PostQuote{ID: "1900000000000000001", URL: "https://x.com/q/status/1900000000000000001", Media: []sqlite.PostMedia{}}}))
	if strings.Contains(string(plain), `"rich"`) {
		t.Fatalf("wording without stickers has rich: %s", plain)
	}
}
