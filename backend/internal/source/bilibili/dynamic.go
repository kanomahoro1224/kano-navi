package bilibili

import (
	"encoding/json"
	"html"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Dynamic is one space dynamic as stored in posts (source = 'bilibili').
type Dynamic struct {
	// ID is the dynamic's numeric ID, as it appears in t.bilibili.com URLs.
	ID string
	// Kind is KindDynamicPost or KindDynamicRepost.
	Kind string
	Text string
	URL  string
	// PublishedAt is the public release time, RFC 3339 in UTC.
	PublishedAt string
	// Author is the uploader's display name, which Bilibili calls the
	// account handle in posts.
	Author    string
	AuthorMid int64
	// Images are the pictures the dynamic carries, in the order shown, on
	// Bilibili's image CDN.
	Images []string
	// Emojis are the custom stickers the text uses, in order. The text keeps
	// their caption ([name]); the icon is what the browser renders.
	Emojis []DynamicEmoji
	// Original is the forwarded dynamic of a repost, nil otherwise.
	Original *Dynamic
}

const (
	// KindDynamicPost is a dynamic the account published itself.
	KindDynamicPost = "post"
	// KindDynamicRepost is a dynamic the account forwarded.
	KindDynamicRepost = "repost"
)

// maxDynamicText bounds one dynamic's text, in runes.
const maxDynamicText = 4000

// maxDynamicImages bounds the pictures kept from one dynamic. A post shows
// nine at most.
const maxDynamicImages = 9

// dynamicIDPattern accepts only the numeric ID a dynamic URL carries.
var dynamicIDPattern = regexp.MustCompile(`^[0-9]{1,32}$`)

// DynamicEmoji is one custom sticker a dynamic's text carries. Bilibili keeps
// the caption in the text and the picture in icon_url.
type DynamicEmoji struct {
	Text string `json:"text"`
	Icon string `json:"icon"`
}

// richTextNode is one piece of a dynamic's text. A modern dynamic splits its
// body into nodes: plain text, a sticker, a mention, a topic or a link. The
// text field of the enclosing summary is a flattened copy that is often empty,
// so the nodes are the reliable source.
type richTextNode struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	OrigText string `json:"orig_text"`
	Emoji    *struct {
		Text    string `json:"text"`
		IconURL string `json:"icon_url"`
	} `json:"emoji"`
}

// richText rebuilds a dynamic's body from its nodes, and collects the stickers
// the body uses. Every node contributes its own wording, which for a sticker is
// its caption, so the text reads the way Bilibili's own page writes it.
func richText(nodes []richTextNode) (string, []DynamicEmoji) {
	if len(nodes) == 0 {
		return "", nil
	}
	var builder strings.Builder
	var emojis []DynamicEmoji
	for _, node := range nodes {
		text := node.Text
		if node.Emoji != nil {
			if node.Emoji.Text != "" {
				text = node.Emoji.Text
			}
			if icon := thumbnail(node.Emoji.IconURL); icon != "" {
				emojis = append(emojis, DynamicEmoji{Text: text, Icon: icon})
			}
		}
		if text == "" {
			text = node.OrigText
		}
		builder.WriteString(text)
	}
	return builder.String(), emojis
}

// flexInt reads an integer Bilibili sends as either a JSON number or a quoted
// string. The space feed quotes timestamps and mids on some entries and not
// on others, and a strict int64 turns that into a whole page of nothing.
type flexInt int64

func (n *flexInt) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(strings.Trim(string(data), `"`))
	if raw == "" || raw == "null" {
		*n = 0
		return nil
	}
	if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
		*n = flexInt(value)
		return nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return err
	}
	*n = flexInt(value)
	return nil
}

// dynamicItem is one entry of the space feed. A forwarded dynamic carries the
// forwarded one in orig, in the same shape.
type dynamicItem struct {
	IDStr   string `json:"id_str"`
	Type    string `json:"type"`
	Modules struct {
		Author *struct {
			PubTS flexInt `json:"pub_ts"`
			Name  string  `json:"name"`
			Mid   flexInt `json:"mid"`
		} `json:"module_author"`
		Dynamic *struct {
			Desc *struct {
				Text string         `json:"text"`
				Rich []richTextNode `json:"rich_text_nodes"`
			} `json:"desc"`
			Major *struct {
				Draw *struct {
					Items []struct {
						Src string `json:"src"`
					} `json:"items"`
				} `json:"draw"`
				Opus *struct {
					Title   string `json:"title"`
					Summary *struct {
						Text string         `json:"text"`
						Rich []richTextNode `json:"rich_text_nodes"`
					} `json:"summary"`
					Pics []struct {
						URL string `json:"url"`
					} `json:"pics"`
				} `json:"opus"`
				Archive *struct {
					BVID  string `json:"bvid"`
					Title string `json:"title"`
				} `json:"archive"`
			} `json:"major"`
		} `json:"module_dynamic"`
	} `json:"modules"`
	Orig *dynamicItem `json:"orig"`
}

// dynamic validates one entry; it reports false for one that is incomplete or
// is not a post: an upload published as a video dynamic is read by the
// uploads list, so storing it again as a post would duplicate it.
func (d dynamicItem) dynamic(account int64) (Dynamic, bool) {
	author := d.Modules.Author
	if !dynamicIDPattern.MatchString(d.IDStr) || author == nil || author.PubTS <= 0 {
		return Dynamic{}, false
	}
	if author.Mid != 0 && int64(author.Mid) != account {
		return Dynamic{}, false
	}
	if d.Type == dynamicTypeVideo {
		return Dynamic{}, false
	}
	text, images, emojis := d.body()
	if text == "" && len(images) == 0 {
		return Dynamic{}, false
	}
	out := Dynamic{
		ID: d.IDStr, Kind: KindDynamicPost, Text: text,
		URL:         "https://t.bilibili.com/" + d.IDStr,
		PublishedAt: time.Unix(int64(author.PubTS), 0).UTC().Format(time.RFC3339),
		Author:      strings.TrimSpace(html.UnescapeString(author.Name)),
		AuthorMid:   int64(author.Mid),
		Images:      images,
		Emojis:      emojis,
	}
	if d.Type == dynamicTypeForward && d.Orig != nil {
		// The forwarded dynamic belongs to whoever published it, so it is
		// read without the account restriction.
		mid := int64(0)
		if d.Orig.Modules.Author != nil {
			mid = int64(d.Orig.Modules.Author.Mid)
		}
		if original, ok := d.Orig.dynamic(mid); ok {
			out.Kind, out.Original = KindDynamicRepost, &original
		}
	}
	return out, true
}

// body is the text, the pictures and the stickers a dynamic shows. Newer
// dynamics keep them in an opus, older ones in a desc with a draw album. The
// body is rebuilt from the rich-text nodes, because the flattened text beside
// them is empty on most entries and would leave an image post with no words.
func (d dynamicItem) body() (string, []string, []DynamicEmoji) {
	var text, title string
	var emojis []DynamicEmoji
	images := []string{}
	dynamic := d.Modules.Dynamic
	if dynamic == nil {
		return "", images, emojis
	}
	if dynamic.Desc != nil {
		if built, stickers := richText(dynamic.Desc.Rich); strings.TrimSpace(built) != "" {
			text, emojis = built, stickers
		} else {
			text = dynamic.Desc.Text
		}
	}
	if major := dynamic.Major; major != nil {
		if major.Opus != nil {
			title = major.Opus.Title
			if major.Opus.Summary != nil {
				if built, stickers := richText(major.Opus.Summary.Rich); strings.TrimSpace(built) != "" {
					text, emojis = built, stickers
				} else if strings.TrimSpace(major.Opus.Summary.Text) != "" {
					text, emojis = major.Opus.Summary.Text, nil
				}
			}
			for _, pic := range major.Opus.Pics {
				images = append(images, pic.URL)
			}
		}
		if major.Draw != nil {
			for _, item := range major.Draw.Items {
				images = append(images, item.Src)
			}
		}
		if major.Archive != nil && strings.TrimSpace(text) == "" {
			// A forwarded upload says nothing itself; its title is the text.
			title = major.Archive.Title
		}
	}
	text = strings.TrimSpace(html.UnescapeString(text))
	if text == "" {
		text = strings.TrimSpace(html.UnescapeString(title))
	}
	if utf8.RuneCountInString(text) > maxDynamicText {
		text = string([]rune(text)[:maxDynamicText])
	}
	kept := []string{}
	seen := map[string]bool{}
	for _, raw := range images {
		url := thumbnail(raw)
		if url == "" || seen[url] || len(kept) >= maxDynamicImages {
			continue
		}
		seen[url] = true
		kept = append(kept, url)
	}
	return text, kept, emojis
}

const (
	dynamicTypeVideo   = "DYNAMIC_TYPE_AV"
	dynamicTypeForward = "DYNAMIC_TYPE_FORWARD"
)

// ParseDynamics reads one space feed page. Entries of another account and
// entries that carry nothing showable are skipped, and so are repeated IDs. An
// entry the parser cannot read is skipped on its own: one entry whose shape
// changed must not hide the rest of the page.
func ParseDynamics(data json.RawMessage, account string) ([]Dynamic, error) {
	var page struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		log.Printf("kano-go: bilibili feed is unreadable: %v", err)
		return nil, &Error{"bilibili_invalid_response"}
	}
	if page.Items == nil {
		// An answer without items is either a pagination edge or a shape
		// change; its opening bytes are the only way to tell which.
		log.Printf("kano-go: bilibili feed has no items: %q", string(data[:min(len(data), 200)]))
		return nil, &Error{"bilibili_invalid_response"}
	}
	owner, _ := strconv.ParseInt(account, 10, 64)
	out := []Dynamic{}
	seen := map[string]bool{}
	unreadable := 0
	for _, raw := range page.Items {
		var item dynamicItem
		if err := json.Unmarshal(raw, &item); err != nil {
			unreadable++
			if unreadable == 1 {
				log.Printf("kano-go: bilibili dynamic is unreadable: %v", err)
			}
			continue
		}
		if d, ok := item.dynamic(owner); ok && !seen[d.ID] {
			seen[d.ID] = true
			out = append(out, d)
		}
	}
	if unreadable > 0 {
		log.Printf("kano-go: bilibili dynamics skipped=%d read=%d", unreadable, len(out))
	}
	return out, nil
}
