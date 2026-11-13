// Package lineblog holds the LINE BLOG archive as the public API serves it.
// scripts/export-lineblog.ts writes archive.json from the tracked files the
// web page reads (src/lineblog/), already in response shape; the archive
// ships with each release and the server never changes it.
package lineblog

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"time"
)

//go:embed archive.json
var packaged []byte

var (
	articleID = regexp.MustCompile(`^[0-9]{1,12}$`)
	isoDate   = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

// ValidID reports whether id has the shape of a LINE BLOG article number.
func ValidID(id string) bool { return articleID.MatchString(id) }

// Article is one article's prepared response bodies.
type Article struct {
	// Post is the body of GET /api/lineblog/posts/{id}.
	Post json.RawMessage
	// Comments is the body of GET /api/lineblog/posts/{id}/comments.
	Comments json.RawMessage
}

// Archive is the packaged archive with its response bodies prepared once.
type Archive struct {
	UpdatedAt string
	// Index is the body of GET /api/lineblog.
	Index    json.RawMessage
	articles map[string]Article
}

// Article returns a known article's bodies.
func (a Archive) Article(id string) (Article, bool) {
	article, ok := a.articles[id]
	return article, ok
}

// Len counts the archive's articles.
func (a Archive) Len() int { return len(a.articles) }

type packagedArchive struct {
	UpdatedAt       string          `json:"updatedAt"`
	OriginalBlogURL string          `json:"originalBlogUrl"`
	Posts           json.RawMessage `json:"posts"`
	Months          json.RawMessage `json:"months"`
	Articles        []struct {
		ID         string          `json:"id"`
		Post       json.RawMessage `json:"post"`
		PreviousID *string         `json:"previousId"`
		NextID     *string         `json:"nextId"`
		Comments   json.RawMessage `json:"comments"`
	} `json:"articles"`
}

var emptyIndex = json.RawMessage(`{"updatedAt":null,"originalBlogUrl":null,"posts":[],"months":[]}`)

// Parse prepares an archive. Content that does not parse leaves an empty
// archive, and articles without a well-formed, unique number are left out.
func Parse(content []byte) Archive {
	empty := Archive{Index: emptyIndex, articles: map[string]Article{}}
	var raw packagedArchive
	if json.Unmarshal(content, &raw) != nil || !isoDate.MatchString(raw.UpdatedAt) {
		return empty
	}
	if _, err := time.Parse(time.DateOnly, raw.UpdatedAt); err != nil {
		return empty
	}
	index, err := json.Marshal(struct {
		UpdatedAt       string          `json:"updatedAt"`
		OriginalBlogURL string          `json:"originalBlogUrl"`
		Posts           json.RawMessage `json:"posts"`
		Months          json.RawMessage `json:"months"`
	}{raw.UpdatedAt, raw.OriginalBlogURL, orEmptyList(raw.Posts), orEmptyList(raw.Months)})
	if err != nil {
		return empty
	}
	out := Archive{UpdatedAt: raw.UpdatedAt, Index: index, articles: make(map[string]Article, len(raw.Articles))}
	for _, article := range raw.Articles {
		if !ValidID(article.ID) || len(article.Post) == 0 || string(article.Post) == "null" {
			continue
		}
		if _, seen := out.articles[article.ID]; seen {
			continue
		}
		post, postErr := json.Marshal(struct {
			UpdatedAt  string          `json:"updatedAt"`
			Post       json.RawMessage `json:"post"`
			PreviousID *string         `json:"previousId"`
			NextID     *string         `json:"nextId"`
		}{raw.UpdatedAt, article.Post, article.PreviousID, article.NextID})
		comments, commentsErr := json.Marshal(struct {
			UpdatedAt string          `json:"updatedAt"`
			Comments  json.RawMessage `json:"comments"`
		}{raw.UpdatedAt, orEmptyList(article.Comments)})
		if postErr != nil || commentsErr != nil {
			continue
		}
		out.articles[article.ID] = Article{Post: post, Comments: comments}
	}
	return out
}

func orEmptyList(value json.RawMessage) json.RawMessage {
	if len(value) == 0 || string(value) == "null" {
		return json.RawMessage(`[]`)
	}
	return value
}

var packagedArchiveValue = Parse(packaged)

// Packaged is the archive this build ships with.
func Packaged() Archive { return packagedArchiveValue }
