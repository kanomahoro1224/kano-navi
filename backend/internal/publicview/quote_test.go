package publicview

import (
	"encoding/json"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// An X quote keeps its fields; a forward of a dynamic whose ID the listing
// did not give has a null id and links to the forward itself.
func TestPublicQuotesKeepTheirShapeForXAndBilibili(t *testing.T) {
	keys := func(v any) string {
		body, _ := json.Marshal(v)
		return string(body)
	}
	x := publicQuote(&sqlite.PostQuote{ID: "1900000000000000001", AuthorHandle: "quoted_synthetic", Text: "quoted", URL: "https://x.com/quoted_synthetic/status/1900000000000000001", Media: []sqlite.PostMedia{}})
	if got := keys(x); got != `{"authorHandle":"quoted_synthetic","authorName":null,"id":"1900000000000000001","media":[],"publishedAt":null,"sourceLanguage":null,"text":"quoted","translations":{},"unavailable":false,"url":"https://x.com/quoted_synthetic/status/1900000000000000001"}` {
		t.Fatalf("x quote=%s", got)
	}
	gone := publicQuote(&sqlite.PostQuote{URL: "https://www.bilibili.com/opus/900000000000000110", Unavailable: true, Media: []sqlite.PostMedia{}})
	if got := keys(gone); got != `{"authorHandle":null,"authorName":null,"id":null,"media":[],"publishedAt":null,"sourceLanguage":null,"text":null,"translations":{},"unavailable":true,"url":"https://www.bilibili.com/opus/900000000000000110"}` {
		t.Fatalf("unknown forwarded dynamic=%s", got)
	}
}
