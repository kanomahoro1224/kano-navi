package lineblog

import (
	"encoding/json"
	"testing"
)

func TestParsePreparesBodiesAndKeepsWellFormedArticles(t *testing.T) {
	archive := Parse([]byte(`{"updatedAt":"2019-11-18","originalBlogUrl":"https://blog.example.test/",
		"posts":[{"id":"2","published":"2019-01-02T00:00:00.000Z"}],"months":[{"month":"2019-01","total":2}],
		"articles":[
			{"id":"2","post":{"id":"2","blocks":[{"type":"rule"}]},"previousId":"1","nextId":null,"comments":[{"number":1}]},
			{"id":"1","post":{"id":"1","blocks":[]},"previousId":null,"nextId":"2"},
			{"id":"1","post":{"id":"duplicate"},"previousId":null,"nextId":null,"comments":[]},
			{"id":"../1","post":{"id":"x"},"previousId":null,"nextId":null,"comments":[]},
			{"id":"3","post":null,"previousId":null,"nextId":null,"comments":[]}]}`))
	if archive.UpdatedAt != "2019-11-18" || archive.Len() != 2 {
		t.Fatalf("archive=%#v", archive)
	}
	var index map[string]any
	if err := json.Unmarshal(archive.Index, &index); err != nil || index["updatedAt"] != "2019-11-18" ||
		index["originalBlogUrl"] != "https://blog.example.test/" || len(index["posts"].([]any)) != 1 || len(index["months"].([]any)) != 1 {
		t.Fatalf("index=%s err=%v", archive.Index, err)
	}
	if _, ok := index["articles"]; ok {
		t.Fatalf("the index leaves out the article bodies: %s", archive.Index)
	}
	newest, _ := archive.Article("2")
	if string(newest.Post) != `{"updatedAt":"2019-11-18","post":{"id":"2","blocks":[{"type":"rule"}]},"previousId":"1","nextId":null}` {
		t.Fatalf("post=%s", newest.Post)
	}
	if string(newest.Comments) != `{"updatedAt":"2019-11-18","comments":[{"number":1}]}` {
		t.Fatalf("comments=%s", newest.Comments)
	}
	oldest, _ := archive.Article("1")
	if string(oldest.Comments) != `{"updatedAt":"2019-11-18","comments":[]}` || string(oldest.Post) == "" {
		t.Fatalf("oldest=%s %s", oldest.Post, oldest.Comments)
	}
	for _, id := range []string{"../1", "3", ""} {
		if _, ok := archive.Article(id); ok {
			t.Fatalf("article %q should be left out", id)
		}
	}
}

func TestParseLeavesAnEmptyArchiveForBrokenContent(t *testing.T) {
	for _, content := range []string{``, `[]`, `{"updatedAt":"2019-13-40","articles":[{"id":"1","post":{}}]}`, `{"articles":[{"id":"1","post":{}}]}`} {
		archive := Parse([]byte(content))
		if archive.Len() != 0 || string(archive.Index) != `{"updatedAt":null,"originalBlogUrl":null,"posts":[],"months":[]}` {
			t.Fatalf("%q: archive=%#v", content, archive)
		}
	}
}

func TestPackagedArchiveParses(t *testing.T) {
	archive := Packaged()
	if archive.UpdatedAt == "" || archive.Len() == 0 {
		t.Fatalf("the packaged archive did not parse: %#v", archive.UpdatedAt)
	}
	var index struct {
		Posts []struct {
			ID string `json:"id"`
		} `json:"posts"`
	}
	if err := json.Unmarshal(archive.Index, &index); err != nil || len(index.Posts) != archive.Len() {
		t.Fatalf("index lists %d posts for %d articles: %v", len(index.Posts), archive.Len(), err)
	}
	for _, post := range index.Posts {
		if _, ok := archive.Article(post.ID); !ok || !ValidID(post.ID) {
			t.Fatalf("listed post %q has no article", post.ID)
		}
	}
}
