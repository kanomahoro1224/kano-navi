// Package cdn joins the public addresses of the built frontend and of cached
// media to the optional CDN base from CDN_PUBLIC_BASE. An empty Base leaves
// every address at this origin, which is the behavior without a CDN.
package cdn

import (
	"bytes"
	"html"
	"regexp"
)

// Base is the normalized public base of a CDN, such as
// https://cdn.example.invalid or https://cdn.example.invalid/kano, or empty
// when this process serves everything itself.
type Base string

// mediaPath is the shape of the media routes the public API hands out (see
// docs/architecture/media-cache.md): a cached asset with its version, a
// profile slot, or an archive file, each with at most its version and width
// parameters. Only a whole value of this shape is rewritten, so a post's own
// wording never changes even when it reads like a path.
var mediaPath = regexp.MustCompile(`^/media/(?:[a-f0-9]{64}|profile/(?:avatar|banner)|(?:lineblog|works|journey)/[a-f0-9]{12}\.(?:jpg|png|mp4))(?:[?&]v=[a-f0-9]{64})?(?:[?&]w=(?:320|640|960|1280|1920))?$`)

// MediaURL returns a public media route at the base; any other value, including
// an already absolute one, is returned unchanged.
func (b Base) MediaURL(url string) string {
	if b == "" || !mediaPath.MatchString(url) {
		return url
	}
	return string(b) + url
}

// Value rewrites the media routes in a public read's decoded JSON value: the
// maps, lists and strings the projections build. Other values pass through.
func (b Base) Value(value any) any {
	if b == "" {
		return value
	}
	switch typed := value.(type) {
	case string:
		return b.MediaURL(typed)
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = b.Value(item)
		}
		return out
	case []string:
		out := make([]string, len(typed))
		for index, item := range typed {
			out[index] = b.MediaURL(item)
		}
		return out
	case []map[string]any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = b.Value(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = b.Value(item)
		}
		return out
	}
	return value
}

// Page rewrites index.html's root-relative references to the build's fixed
// files to the base, and adds the meta tag the frontend reads for the media
// addresses it builds itself (src/lib/cdn.ts). Only those attributes change:
// the page's own wording and inline script stay as they are.
func (b Base) Page(page []byte) []byte {
	if b == "" {
		return page
	}
	base := string(b)
	for _, attribute := range []string{"src", "href", "content"} {
		page = bytes.ReplaceAll(page, []byte(attribute+`="/assets/`), []byte(attribute+`="`+base+`/assets/`))
	}
	page = bytes.ReplaceAll(page, []byte(`href="/manifest.webmanifest"`), []byte(`href="`+base+`/manifest.webmanifest"`))
	meta := []byte(`<meta name="kano-cdn-base" content="` + html.EscapeString(base) + `">`)
	head := bytes.Index(page, []byte("</head>"))
	if head < 0 {
		return page
	}
	out := make([]byte, 0, len(page)+len(meta))
	out = append(out, page[:head]...)
	out = append(out, meta...)
	out = append(out, page[head:]...)
	return out
}
