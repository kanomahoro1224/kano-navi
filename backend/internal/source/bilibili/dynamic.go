package bilibili

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Dynamic is one post (动态) of the account as stored in posts
// (source = 'bilibili').
type Dynamic struct {
	// IDStr is Bilibili's dynamic ID; the stored post ID is PostID().
	IDStr string
	// OwnerMid is the author's account.
	OwnerMid int64
	// Text joins the word, topic, mention and link nodes; emoji nodes are
	// left out, so an emoji-only dynamic has no text.
	Text string
	// PublishedAt is the publication time, RFC 3339 in UTC.
	PublishedAt string
	// Images are the dynamic's own pictures on Bilibili's image CDN, HTTPS.
	Images []string
	// Raw keeps the type and the source nodes, emoji included.
	Raw map[string]any
}

// PostPrefix keeps dynamic IDs apart from X status IDs in posts.id: both are
// decimal numbers of about 19 digits.
const PostPrefix = "bilibili-"

func (d Dynamic) PostID() string { return PostPrefix + d.IDStr }

// URL is the dynamic's public opus page.
func (d Dynamic) URL() string { return "https://" + wwwHost + "/opus/" + d.IDStr }

var dynamicIDPattern = regexp.MustCompile(`^[1-9][0-9]{8,24}$`)

// ParseDynamicID accepts a bilibili.com/opus/<id> or t.bilibili.com/<id>
// link, or a bare dynamic ID, and returns the ID.
func ParseDynamicID(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if dynamicIDPattern.MatchString(input) {
		return input, true
	}
	u, err := url.Parse(input)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch strings.ToLower(u.Hostname()) {
	case "www.bilibili.com", "bilibili.com", "m.bilibili.com":
		if len(parts) == 2 && parts[0] == "opus" && dynamicIDPattern.MatchString(parts[1]) {
			return parts[1], true
		}
	case "t.bilibili.com":
		if len(parts) == 1 && dynamicIDPattern.MatchString(parts[0]) {
			return parts[0], true
		}
	}
	return "", false
}

// Dynamic types the listing keeps: text, picture and opus posts, and an
// article only in its opus form. Reposts are left out as on X, uploads and
// collection updates are the upload read's, and a live-start notice is not a
// post.
var keptDynamicTypes = map[string]bool{"DYNAMIC_TYPE_WORD": true, "DYNAMIC_TYPE_DRAW": true, "DYNAMIC_TYPE_ARTICLE": true}

// keptCommentTypes are the comment areas of the kept types as an opus page
// names them: 11 a picture post, 12 an article, 17 a text post. A video (1)
// or another kind is not a post.
var keptCommentTypes = map[int]bool{11: true, 12: true, 17: true}

const emojiNode = "RICH_TEXT_NODE_TYPE_EMOJI"

// maxDynamicText bounds the stored text of one dynamic, in runes.
const maxDynamicText = 5000

// maxDynamicImages bounds the pictures kept from one dynamic.
const maxDynamicImages = 9

type richNode struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	OrigTxt string `json:"orig_text"`
}

// joinNodes keeps every node's text except emoji.
func joinNodes(nodes []richNode) string {
	var b strings.Builder
	for _, n := range nodes {
		if n.Type == emojiNode {
			continue
		}
		b.WriteString(n.Text)
	}
	return b.String()
}

func boundText(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if r := []rune(text); len(r) > maxDynamicText {
		text = string(r[:maxDynamicText])
	}
	return text
}

func images(urls []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range urls {
		if u := thumbnail(raw); u != "" && !seen[u] && len(out) < maxDynamicImages {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

// flexInt reads a number given as a JSON number or a decimal string.
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	b = bytes.Trim(b, `"`)
	n, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexInt(n)
	return nil
}

type spaceItem struct {
	IDStr   string `json:"id_str"`
	Type    string `json:"type"`
	Visible *bool  `json:"visible"`
	Modules struct {
		Author struct {
			Mid   flexInt `json:"mid"`
			PubTS flexInt `json:"pub_ts"`
		} `json:"module_author"`
		Dynamic struct {
			Desc *struct {
				Nodes []richNode `json:"rich_text_nodes"`
				Text  string     `json:"text"`
			} `json:"desc"`
			Major *struct {
				Type string `json:"type"`
				Opus *struct {
					Title   string `json:"title"`
					Summary *struct {
						Nodes []richNode `json:"rich_text_nodes"`
						Text  string     `json:"text"`
					} `json:"summary"`
					Pics []struct {
						URL string `json:"url"`
					} `json:"pics"`
				} `json:"opus"`
				Draw *struct {
					Items []struct {
						Src string `json:"src"`
					} `json:"items"`
				} `json:"draw"`
			} `json:"major"`
		} `json:"module_dynamic"`
		Tag *struct {
			Text string `json:"text"`
		} `json:"module_tag"`
	} `json:"modules"`
}

// dynamic validates one listing item of the account; it reports false for a
// kind that is not a post or an incomplete item.
func (it spaceItem) dynamic(owner int64) (Dynamic, bool) {
	if !keptDynamicTypes[it.Type] || !dynamicIDPattern.MatchString(it.IDStr) || it.Modules.Author.PubTS <= 0 ||
		int64(it.Modules.Author.Mid) != owner || it.Visible != nil && !*it.Visible {
		return Dynamic{}, false
	}
	m := it.Modules.Dynamic
	if it.Type == "DYNAMIC_TYPE_ARTICLE" && (m.Major == nil || m.Major.Type != "MAJOR_TYPE_OPUS") {
		return Dynamic{}, false
	}
	var nodes []richNode
	title := ""
	pics := []string{}
	if m.Major != nil {
		switch m.Major.Type {
		case "MAJOR_TYPE_OPUS":
			if o := m.Major.Opus; o != nil {
				title = strings.TrimSpace(o.Title)
				if o.Summary != nil {
					nodes = o.Summary.Nodes
				}
				for _, p := range o.Pics {
					pics = append(pics, p.URL)
				}
			}
		case "MAJOR_TYPE_DRAW":
			if d := m.Major.Draw; d != nil {
				for _, p := range d.Items {
					pics = append(pics, p.Src)
				}
			}
		case "", "MAJOR_TYPE_NONE":
		default:
			// Another card (a vote, a reservation or goods) leaves the text.
		}
	}
	if nodes == nil && m.Desc != nil {
		nodes = m.Desc.Nodes
	}
	text := joinNodes(nodes)
	if title != "" {
		text = title + "\n" + text
	}
	raw := map[string]any{"type": it.Type, "nodes": nodes}
	if title != "" {
		raw["title"] = title
	}
	if it.Modules.Tag != nil && it.Modules.Tag.Text != "" {
		raw["tag"] = it.Modules.Tag.Text
	}
	return Dynamic{
		IDStr: it.IDStr, OwnerMid: owner, Text: boundText(text),
		PublishedAt: time.Unix(int64(it.Modules.Author.PubTS), 0).UTC().Format(time.RFC3339),
		Images:      images(pics), Raw: raw,
	}, true
}

// SpacePage is one page of the account's dynamics listing.
type SpacePage struct {
	Dynamics []Dynamic
	// Seen counts every item the page listed, kept or not.
	Seen    int
	HasMore bool
	Offset  string
}

// ParseSpaceFeed reads a feed/space page. A v_voucher answer instead of
// items is Bilibili's risk control.
func ParseSpaceFeed(data json.RawMessage, account string) (SpacePage, error) {
	var page struct {
		Items    []spaceItem `json:"items"`
		HasMore  bool        `json:"has_more"`
		Offset   string      `json:"offset"`
		VVoucher string      `json:"v_voucher"`
	}
	if json.Unmarshal(data, &page) != nil {
		return SpacePage{}, &Error{"bilibili_invalid_response"}
	}
	if page.VVoucher != "" {
		return SpacePage{}, &Error{"bilibili_risk_control_voucher"}
	}
	if page.Items == nil {
		return SpacePage{}, &Error{"bilibili_invalid_response"}
	}
	owner, _ := strconv.ParseInt(account, 10, 64)
	out := SpacePage{Dynamics: []Dynamic{}, Seen: len(page.Items), HasMore: page.HasMore, Offset: page.Offset}
	seen := map[string]bool{}
	for _, item := range page.Items {
		if d, ok := item.dynamic(owner); ok && !seen[d.IDStr] {
			seen[d.IDStr] = true
			out.Dynamics = append(out.Dynamics, d)
		}
	}
	return out, nil
}

// opusDetail is the item of the opus page's server state and of the
// opus/detail API: a module list rather than the listing's module object.
type opusDetail struct {
	IDStr string `json:"id_str"`
	Basic struct {
		CommentType *int `json:"comment_type"`
	} `json:"basic"`
	// Fallback is set when the page shows another kind of content instead.
	Fallback json.RawMessage `json:"fallback"`
	Modules  []struct {
		Type   string `json:"module_type"`
		Author *struct {
			Mid   flexInt `json:"mid"`
			PubTS flexInt `json:"pub_ts"`
		} `json:"module_author"`
		Title *struct {
			Text string `json:"text"`
		} `json:"module_title"`
		Content *struct {
			Paragraphs []struct {
				ParaType int `json:"para_type"`
				Text     *struct {
					Nodes []struct {
						Type string `json:"type"`
						Word *struct {
							Words string `json:"words"`
						} `json:"word"`
						Rich *struct {
							Type string `json:"type"`
							Text string `json:"text"`
						} `json:"rich"`
					} `json:"nodes"`
				} `json:"text"`
				Pic *struct {
					Pics []struct {
						URL string `json:"url"`
					} `json:"pics"`
				} `json:"pic"`
			} `json:"paragraphs"`
		} `json:"module_content"`
	} `json:"modules"`
}

func (d opusDetail) dynamic(id string) (Dynamic, error) {
	if d.IDStr != id {
		return Dynamic{}, &Error{"bilibili_invalid_response"}
	}
	if d.Basic.CommentType == nil || !keptCommentTypes[*d.Basic.CommentType] || len(d.Fallback) > 0 && string(d.Fallback) != "null" {
		return Dynamic{}, &Error{"bilibili_unsupported_dynamic"}
	}
	var owner, published int64
	title := ""
	var paragraphs []string
	nodes := []richNode{}
	pics := []string{}
	content := false
	for _, m := range d.Modules {
		switch {
		case m.Author != nil && m.Type == "MODULE_TYPE_AUTHOR":
			owner, published = int64(m.Author.Mid), int64(m.Author.PubTS)
		case m.Title != nil && m.Type == "MODULE_TYPE_TITLE":
			title = strings.TrimSpace(m.Title.Text)
		case m.Content != nil && m.Type == "MODULE_TYPE_CONTENT":
			content = true
			for _, p := range m.Content.Paragraphs {
				if p.Pic != nil {
					for _, pic := range p.Pic.Pics {
						pics = append(pics, pic.URL)
					}
				}
				if p.Text == nil {
					continue
				}
				var b strings.Builder
				for _, n := range p.Text.Nodes {
					switch {
					case n.Word != nil:
						b.WriteString(n.Word.Words)
						nodes = append(nodes, richNode{Type: "RICH_TEXT_NODE_TYPE_TEXT", Text: n.Word.Words})
					case n.Rich != nil:
						nodes = append(nodes, richNode{Type: n.Rich.Type, Text: n.Rich.Text})
						if n.Rich.Type != emojiNode {
							b.WriteString(n.Rich.Text)
						}
					}
				}
				paragraphs = append(paragraphs, strings.TrimRight(b.String(), "\n"))
			}
		}
	}
	if !content || owner <= 0 || published <= 0 {
		return Dynamic{}, &Error{"bilibili_invalid_response"}
	}
	text := strings.Join(paragraphs, "\n")
	if title != "" {
		text = title + "\n" + text
	}
	raw := map[string]any{"type": "opus", "nodes": nodes}
	if title != "" {
		raw["title"] = title
	}
	return Dynamic{
		IDStr: id, OwnerMid: owner, Text: boundText(text),
		PublishedAt: time.Unix(published, 0).UTC().Format(time.RFC3339),
		Images:      images(pics), Raw: raw,
	}, nil
}

var initialState = []byte("window.__INITIAL_STATE__=")

// ParseOpusPage reads the server-rendered state of a www.bilibili.com/opus
// page for dynamic id. A page without that state is the captcha or an error
// page, which starts the backoff like any risk-control answer.
func ParseOpusPage(body []byte, id string) (Dynamic, error) {
	start := bytes.Index(body, initialState)
	if start < 0 {
		return Dynamic{}, &Error{"bilibili_risk_control_page"}
	}
	var state struct {
		Detail *opusDetail `json:"detail"`
	}
	// The state is one JSON object followed by the page's script.
	if json.NewDecoder(bytes.NewReader(body[start+len(initialState):])).Decode(&state) != nil {
		return Dynamic{}, &Error{"bilibili_invalid_response"}
	}
	if state.Detail == nil {
		return Dynamic{}, &Error{"bilibili_risk_control_page"}
	}
	return state.Detail.dynamic(id)
}
