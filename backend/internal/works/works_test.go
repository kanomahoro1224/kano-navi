package works

import "testing"

func TestParseKeepsWellFormedUploadsAndVideos(t *testing.T) {
	index := Parse([]byte(`{"updatedAt":"2026-10-29","uploads":[
		{"id":"2026-01-01-abcdef","date":"2026-01-01","videos":[
			{"platform":"youtube","id":"aaaaaaaaaaa","members":true},
			{"platform":"youtube","id":"short"},
			{"platform":"niconico","id":"sm9","members":true},
			{"platform":"bilibili","id":"BV1AAAAAAAAA"},
			{"platform":"vimeo","id":"1"}]},
		{"id":"2026-01-01-abcdef","date":"2026-01-01","videos":[]},
		{"id":"../x","date":"2026-01-01","videos":[]}]}`))
	if index.UpdatedAt != "2026-10-29" || len(index.Uploads) != 1 {
		t.Fatalf("index=%#v", index)
	}
	videos := index.Uploads[0].Videos
	if len(videos) != 3 || !videos[0].Members || videos[1].Members || videos[2].ID != "BV1AAAAAAAAA" {
		t.Fatalf("videos=%#v", videos)
	}
	if bad := Parse([]byte("not json")); len(bad.Uploads) != 0 {
		t.Fatalf("bad=%#v", bad)
	}
}

func TestPackagedIndexIsReadable(t *testing.T) {
	index := Packaged()
	if index.UpdatedAt == "" || len(index.Uploads) == 0 {
		t.Fatalf("packaged index is empty: %q, %d uploads", index.UpdatedAt, len(index.Uploads))
	}
}
