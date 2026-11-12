package sqlite

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
)

// RichSegment is one piece of a Bilibili dynamic's original wording: either
// Text, or a sticker with its caption in Emoji and, when its cached picture
// is ready, that picture's local /media URL in URL. Joined in order, the
// segments' Text is exactly the post's text: captions are never part of it.
type RichSegment struct {
	Text  string
	Emoji string
	URL   *string
}

// stickerSource is the part of a dynamic's raw_json its stickers are read
// from: its own title and rich-text nodes and, for a forward, those of the
// forwarded dynamic under orig.
type stickerSource struct {
	Title string       `json:"title"`
	Nodes []storedNode `json:"nodes"`
	Orig  *struct {
		Title string       `json:"title"`
		Nodes []storedNode `json:"nodes"`
	} `json:"orig"`
}

type storedNode struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Emoji *struct {
		URL string `json:"url"`
	} `json:"emoji"`
}

// mapStickers gives a Bilibili post, and its available quote, the sticker
// segments of their wording from the post's raw_json.
func (s *Store) mapStickers(ctx context.Context, p *Post, raw string) error {
	var src stickerSource
	if json.Unmarshal([]byte(raw), &src) != nil {
		return nil
	}
	var err error
	if p.Rich, err = s.stickerSegments(ctx, p.ID, rolePostEmoji, p.Text, src.Title, src.Nodes); err != nil {
		return err
	}
	if p.Quote != nil && !p.Quote.Unavailable && src.Orig != nil {
		if p.Quote.Rich, err = s.stickerSegments(ctx, p.ID, roleQuoteEmoji, p.Quote.Text, src.Orig.Title, src.Orig.Nodes); err != nil {
			return err
		}
	}
	return nil
}

// stickerSegments reads the ready pictures linked to the post under role and
// places the stickers of nodes into text (see placeStickers).
func (s *Store) stickerSegments(ctx context.Context, postID, role, text, title string, nodes []storedNode) ([]RichSegment, error) {
	if !hasSticker(nodes) {
		return nil, nil
	}
	links, err := s.queryPostMedia(ctx, postID, role)
	if err != nil {
		return nil, err
	}
	ready := map[string]string{}
	for _, link := range links {
		if link.Status == "ready" && link.ID != nil && link.URL != nil && strings.HasPrefix(*link.URL, "/media/") {
			ready[*link.ID] = *link.URL
		}
	}
	return placeStickers(text, title, nodes, ready), nil
}

func hasSticker(nodes []storedNode) bool {
	for _, n := range nodes {
		if n.Type == bilisource.EmojiNode && n.Text != "" {
			return true
		}
	}
	return false
}

// placeStickers splits text, the stored wording the nodes produced (a title
// on its first line, then the nodes' text without emoji, trimmed and
// bounded), at each emoji node's place. Each node's text must start where
// the last one ended, so the cuts follow the nodes in order and never skip
// wording the nodes did not give; a node whose text is not there (cut off by
// the bound) ends the placing, and the stickers after it are left out. A
// sticker whose picture is ready (ready maps a media ID to its local URL)
// carries that URL; any other is its caption alone. It returns nil when no
// sticker was placed.
func placeStickers(text, title string, nodes []storedNode, ready map[string]string) []RichSegment {
	cursor, last := 0, 0
	// A title leads the text on a line of its own; with a body of stickers
	// alone, the trimmed text is the title.
	if title != "" && (text == title || strings.HasPrefix(text, title+"\n")) {
		cursor = min(len(title)+1, len(text))
	}
	out := []RichSegment{}
	placed := false
	for _, n := range nodes {
		if n.Type == bilisource.EmojiNode {
			if n.Text == "" {
				continue
			}
			if cursor > last {
				out = append(out, RichSegment{Text: text[last:cursor]})
				last = cursor
			}
			segment := RichSegment{Emoji: n.Text}
			if n.Emoji != nil {
				if id, _, err := mediaIdentity(n.Emoji.URL); err == nil {
					if u, ok := ready[id]; ok {
						segment.URL = &u
					}
				}
			}
			out = append(out, segment)
			placed = true
			continue
		}
		word := strings.ReplaceAll(n.Text, "\r\n", "\n")
		if strings.TrimSpace(word) == "" {
			if strings.HasPrefix(text[cursor:], word) {
				cursor += len(word)
			}
			continue
		}
		// The stored text is trimmed at both ends, and so is each opus
		// paragraph, so a node may have lost its outer whitespace there.
		matched := false
		for _, w := range []string{word, strings.TrimLeftFunc(word, unicode.IsSpace), strings.TrimRightFunc(word, unicode.IsSpace), strings.TrimSpace(word)} {
			if strings.HasPrefix(text[cursor:], w) {
				cursor += len(w)
				matched = true
				break
			}
		}
		if !matched {
			break
		}
	}
	if !placed {
		return nil
	}
	if last < len(text) {
		out = append(out, RichSegment{Text: text[last:]})
	}
	return out
}
