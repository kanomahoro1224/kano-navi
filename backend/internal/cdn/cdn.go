// Package cdn joins the public addresses of the built frontend and of cached
// media to the optional CDN base from CDN_PUBLIC_BASE. An empty Base leaves
// every address at this origin, which is the behavior without a CDN.
package cdn

import (
	"bytes"
	"encoding/json"
	"html"
	"io"
	"regexp"
	"strings"
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

// assetPath is the shape of a fixed public image of the build that a public
// read may name, such as the default avatar and banner (/assets/kano-avatar.jpg)
// the dashboard falls back to: a whole root-relative /assets/ path of plain
// segments ending in an image extension.
var assetPath = regexp.MustCompile(`^/assets/(?:[A-Za-z0-9_~-][A-Za-z0-9._~-]*/)*[A-Za-z0-9_~-][A-Za-z0-9._~-]*\.(?:jpg|jpeg|png|webp|gif|avif|svg)$`)

// MediaURL returns a public media route, or a fixed public image of the build,
// at the base; any other value, including an already absolute one, is returned
// unchanged.
func (b Base) MediaURL(url string) string {
	if b == "" {
		return url
	}
	if mediaPath.MatchString(url) || assetPath.MatchString(url) && !strings.Contains(url, "..") {
		return string(b) + url
	}
	return url
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

// JSON rewrites the media routes in a public read's JSON body, such as the
// packaged Journey, Works and LINE BLOG archives. An empty base returns the
// body unchanged. Otherwise the result is the compact body with only its
// matching string values moved to the base: object keys keep their order,
// numbers their spelling and strings their characters, so a client sees the
// same document.
func (b Base) JSON(body []byte) ([]byte, error) {
	if b == "" {
		return body, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	type frame struct {
		object bool
		count  int
	}
	var out bytes.Buffer
	out.Grow(len(body))
	var stack []frame
	var scalar bytes.Buffer
	encoder := json.NewEncoder(&scalar)
	encoder.SetEscapeHTML(false)
	// separate writes the comma or colon before the next token in the
	// current container and reports whether that token is an object key.
	separate := func() bool {
		if len(stack) == 0 {
			return false
		}
		top := &stack[len(stack)-1]
		key := top.object && top.count%2 == 0
		switch {
		case top.count == 0:
		case top.object && !key:
			out.WriteByte(':')
		default:
			out.WriteByte(',')
		}
		top.count++
		return key
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF && len(stack) == 0 && out.Len() > 0 {
			break
		}
		if err == io.EOF {
			return nil, io.ErrUnexpectedEOF
		}
		if err != nil {
			return nil, err
		}
		if delim, ok := token.(json.Delim); ok && (delim == '}' || delim == ']') {
			stack = stack[:len(stack)-1]
			out.WriteByte(byte(delim))
			continue
		}
		key := separate()
		switch typed := token.(type) {
		case json.Delim:
			out.WriteByte(byte(typed))
			stack = append(stack, frame{object: typed == '{'})
			continue
		case string:
			if !key {
				typed = b.MediaURL(typed)
			}
			token = typed
		}
		// Strings keep <, > and & as written, as json.Compact does for the
		// origin's copy, so the two copies differ only in their media routes.
		scalar.Reset()
		if err := encoder.Encode(token); err != nil {
			return nil, err
		}
		out.Write(bytes.TrimSuffix(scalar.Bytes(), []byte("\n")))
	}
	return out.Bytes(), nil
}

// Page rewrites index.html's root-relative references to the build's fixed
// files to the base, and adds the meta tag the frontend reads for the media
// addresses it builds itself (src/lib/cdn.ts). Only those attributes change:
// the page's own wording and inline script stay as they are. The web app
// manifest stays at this origin: browsers resolve its start_url and scope
// against the manifest's own address and ignore them on another origin.
func (b Base) Page(page []byte) []byte {
	if b == "" {
		return page
	}
	base := string(b)
	for _, attribute := range []string{"src", "href", "content"} {
		page = bytes.ReplaceAll(page, []byte(attribute+`="/assets/`), []byte(attribute+`="`+base+`/assets/`))
	}
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
