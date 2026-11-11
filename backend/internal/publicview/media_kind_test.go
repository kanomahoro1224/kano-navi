package publicview

import (
	"testing"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// Each public media item names what it shows as a code: a video's or GIF's
// thumbnail says so, and every other image, a stored or unknown kind
// included, is a photo.
func TestPublicMediaNamesItsKind(t *testing.T) {
	url := "/media/x-synthetic"
	items := publicMedia([]sqlite.PostMedia{
		{URL: &url, Status: "ready", Kind: "video"},
		{URL: &url, Status: "ready", Kind: "gif"},
		{URL: &url, Status: "ready", Kind: "photo"},
		{URL: &url, Status: "ready"},
		{URL: &url, Status: "ready", Kind: "<b>"},
	})
	got := []string{}
	for _, item := range items {
		got = append(got, item.(map[string]any)["kind"].(string))
	}
	if want := "video,gif,photo,photo,photo"; joined(got) != want {
		t.Fatalf("kinds=%v", got)
	}
	quote := publicQuote(&sqlite.PostQuote{ID: "1900000000000000801", URL: "https://x.com/other/status/1900000000000000801",
		Media: []sqlite.PostMedia{{URL: &url, Status: "ready", Kind: "video"}}}).(map[string]any)
	if kind := quote["media"].([]any)[0].(map[string]any)["kind"]; kind != "video" {
		t.Fatalf("quote kind=%v", kind)
	}
}

func joined(values []string) string {
	out := ""
	for i, v := range values {
		if i > 0 {
			out += ","
		}
		out += v
	}
	return out
}
