package cdn

import (
	"strings"
	"testing"
)

const assetID = "a111111111111111111111111111111111111111111111111111111111111111"

func TestMediaURL(t *testing.T) {
	media := "/media/" + assetID + "?v=" + assetID
	for _, tc := range []struct {
		base, in, want string
	}{
		{"", media, media},
		{"https://cdn.example.invalid", media, "https://cdn.example.invalid" + media},
		{"https://cdn.example.invalid/kano", "/media/" + assetID + "?v=" + assetID + "&w=640", "https://cdn.example.invalid/kano/media/" + assetID + "?v=" + assetID + "&w=640"},
		{"https://cdn.example.invalid", "/media/profile/avatar?v=" + assetID, "https://cdn.example.invalid/media/profile/avatar?v=" + assetID},
		{"https://cdn.example.invalid", "/media/lineblog/0123456789ab.jpg?w=320", "https://cdn.example.invalid/media/lineblog/0123456789ab.jpg?w=320"},
		// The dashboard's default avatar and banner are fixed public images.
		{"https://cdn.example.invalid/kano", "/assets/kano-avatar.jpg", "https://cdn.example.invalid/kano/assets/kano-avatar.jpg"},
		{"https://cdn.example.invalid", "/assets/icons/icon-192.png", "https://cdn.example.invalid/assets/icons/icon-192.png"},
		{"", "/assets/kano-banner.jpg", "/assets/kano-banner.jpg"},
	} {
		if got := Base(tc.base).MediaURL(tc.in); got != tc.want {
			t.Fatalf("%q: %q => %q want %q", tc.base, tc.in, got, tc.want)
		}
	}
	base := Base("https://cdn.example.invalid")
	// A public read only hands out media routes; every other value is a post's
	// wording, an external address or a bundled asset and stays as it is.
	for _, url := range []string{
		"/assets/",
		"/assets/kano-banner",
		"/assets/index-AbCd1234.js",
		"/assets/../secret.jpg",
		"/assets/./x.jpg",
		"/assets/a/../x.jpg",
		"/assets/kano-banner.jpg?x=1",
		"/assets/kano banner.jpg",
		"see /assets/kano-banner.jpg",
		"/media/hello",
		"/media/" + strings.ToUpper(assetID),
		"/media/journey/0123456789ab.gif",
		"/media/" + assetID + "?v=short",
		"https://cdn.example.invalid/media/" + assetID,
		"https://pbs.twimg.com/media/x.jpg",
	} {
		if got := base.MediaURL(url); got != url {
			t.Fatalf("%q => %q", url, got)
		}
	}
}

func TestValueWalksPublicReads(t *testing.T) {
	base := Base("https://cdn.example.invalid")
	media := "/media/" + assetID + "?v=" + assetID
	value := map[string]any{
		"items": []any{
			map[string]any{"url": media, "text": `See "/media/picture" here`, "status": "ready"},
			map[string]any{"url": "/assets/kano-avatar.jpg"},
		},
		"stickers": []map[string]any{{"url": media}},
		"single":   media,
	}
	got := base.Value(value).(map[string]any)
	items := got["items"].([]any)
	if first := items[0].(map[string]any); first["url"] != "https://cdn.example.invalid"+media || first["text"] != `See "/media/picture" here` {
		t.Fatalf("item = %#v", first)
	}
	if second := items[1].(map[string]any); second["url"] != "https://cdn.example.invalid/assets/kano-avatar.jpg" {
		t.Fatalf("default avatar stayed at the origin: %#v", second)
	}
	if sticker := got["stickers"].([]any)[0].(map[string]any); sticker["url"] != "https://cdn.example.invalid"+media {
		t.Fatalf("sticker = %#v", sticker)
	}
	if got["single"] != "https://cdn.example.invalid"+media {
		t.Fatalf("single = %#v", got["single"])
	}
	// The input value is left alone: the same projection may be reused.
	if value["single"] != media {
		t.Fatal("Value mutated its input")
	}
	// No base leaves the value exactly as the projection built it.
	if _, ok := Base("").Value(value).(map[string]any); !ok {
		t.Fatal("empty base rejected a map")
	}
}

func TestJSON(t *testing.T) {
	media := "/media/journey/0123456789ab.jpg"
	body := []byte(`{"zeta":1.50,"alpha":{"url":"` + media + `","text":"` + media + ` <b>&"},` +
		`"items":[{"url":"/media/works/0123456789ab.png"},null,true,[],{},"plain",12e3],"` + media + `":"key"}`)
	got, err := Base("https://cdn.example.invalid/kano").JSON(body)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"zeta":1.50,"alpha":{"url":"https://cdn.example.invalid/kano` + media + `","text":"` + media + ` <b>&"},` +
		`"items":[{"url":"https://cdn.example.invalid/kano/media/works/0123456789ab.png"},null,true,[],{},"plain",12e3],"` + media + `":"key"}`
	if string(got) != want {
		t.Fatalf("JSON =\n%s\nwant\n%s", got, want)
	}
	// Whitespace goes, keys keep their order and the value is unchanged in meaning.
	spaced := []byte("{\n  \"b\": [ 1, 2 ],\n  \"a\": \"" + media + "\"\n}\n")
	if got, err := Base("https://cdn.example.invalid").JSON(spaced); err != nil || string(got) != `{"b":[1,2],"a":"https://cdn.example.invalid`+media+`"}` {
		t.Fatalf("spaced => %s %v", got, err)
	}
	// No base returns the body itself; a broken body is an error.
	if got, err := Base("").JSON(body); err != nil || string(got) != string(body) {
		t.Fatalf("empty base => %s %v", got, err)
	}
	if _, err := Base("https://cdn.example.invalid").JSON([]byte(`{"a":`)); err == nil {
		t.Fatal("truncated body accepted")
	}
}

func TestPage(t *testing.T) {
	page := []byte(`<!doctype html><html><head>` +
		`<meta property="og:image" content="/assets/kano-banner.jpg">` +
		`<link rel="manifest" href="/manifest.webmanifest">` +
		`<link rel="icon" href="/assets/icons/favicon-32.png">` +
		`</head><body><script type="module" src="/assets/index-AbCd1234.js"></script></body></html>`)
	base := Base("https://cdn.example.invalid/kano")
	got := string(base.Page(page))
	for _, want := range []string{
		`content="https://cdn.example.invalid/kano/assets/kano-banner.jpg"`,
		`href="/manifest.webmanifest"`,
		`href="https://cdn.example.invalid/kano/assets/icons/favicon-32.png"`,
		`src="https://cdn.example.invalid/kano/assets/index-AbCd1234.js"`,
		`<meta name="kano-cdn-base" content="https://cdn.example.invalid/kano"></head>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("page lacks %q: %s", want, got)
		}
	}
	if strings.Contains(got, `="/assets/`) {
		t.Fatalf("page still names fixed files at the origin: %s", got)
	}
	// A build made with the CDN base already addresses its own files there,
	// and the rewrite only claims the remaining root-relative ones.
	built := []byte(`<script type="module" crossorigin src="https://cdn.example.invalid/kano/assets/index-AbCd1234.js"></script>`)
	if got := string(base.Page(built)); got != string(built) {
		t.Fatalf("prefixed build changed: %s", got)
	}
	// No base leaves the page byte for byte.
	if got := string(Base("").Page(page)); got != string(page) {
		t.Fatalf("empty base changed the page: %s", got)
	}
	// A page without a head still gets its references rewritten.
	headless := []byte(`<html><body><img src="/assets/x.png"></body></html>`)
	if got := string(base.Page(headless)); !strings.Contains(got, `src="https://cdn.example.invalid/kano/assets/x.png"`) || strings.Contains(got, "kano-cdn-base") {
		t.Fatalf("headless page = %s", got)
	}
}
