package works

import (
	"encoding/json"
	"regexp"
	"testing"
)

var coverURL = regexp.MustCompile(`^/media/works/[0-9a-f]{12}\.(?:jpg|png)$`)

func decode(t *testing.T, content []byte) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(content, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

// images visits every image object: the values of the keys that hold one or
// a list of them.
func images(value any, visit func(map[string]any)) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch key {
			case "covers", "bonusImages", "images":
				for _, image := range child.([]any) {
					visit(image.(map[string]any))
				}
			case "cover", "disc", "cutout", "image":
				if image, ok := child.(map[string]any); ok {
					visit(image)
				}
			default:
				images(child, visit)
			}
		}
	case []any:
		for _, child := range typed {
			images(child, visit)
		}
	}
}

func TestPackagedCatalogKeepsItsContract(t *testing.T) {
	body := decode(t, Catalog())
	summary := body["summary"].(map[string]any)
	if body["updatedAt"] != Packaged().UpdatedAt || summary["updatedAt"] != nil || body["revision"] != nil ||
		float64(len(body["personal"].([]any))) != summary["personal"] ||
		float64(len(body["participation"].([]any))) != summary["participation"] {
		t.Fatalf("updatedAt=%v summary=%v", body["updatedAt"], summary)
	}
	count := 0
	images(body, func(image map[string]any) {
		count++
		url, _ := image["url"].(string)
		mimeType, _ := image["mimeType"].(string)
		if !coverURL.MatchString(url) || (mimeType != "image/jpeg" && mimeType != "image/png") || image["width"].(float64) < 1 {
			t.Fatalf("image=%#v", image)
		}
		for _, key := range []string{"caption", "sourceUrl", "sourceLabel", "credit", "thirdParty"} {
			if _, ok := image[key]; !ok {
				t.Fatalf("image lacks %s: %#v", key, image)
			}
		}
		if caption, ok := image["caption"].(map[string]any); ok && len(caption) != 3 {
			t.Fatalf("caption=%#v", caption)
		}
	})
	if count == 0 {
		t.Fatal("no images")
	}
}

func TestPackagedUploadListMatchesTheVideoIndex(t *testing.T) {
	body := decode(t, UploadList())
	uploads := body["uploads"].([]any)
	index := Packaged()
	if body["updatedAt"] != index.UpdatedAt || len(uploads) != len(index.Uploads) {
		t.Fatalf("updatedAt=%v uploads=%d index=%d", body["updatedAt"], len(uploads), len(index.Uploads))
	}
	for at, raw := range uploads {
		upload := raw.(map[string]any)
		if upload["id"] != index.Uploads[at].ID || upload["date"] != index.Uploads[at].Date {
			t.Fatalf("upload %d=%v, index has %s", at, upload["id"], index.Uploads[at].ID)
		}
		for _, id := range upload["youtube"].([]any) {
			if !ValidVideo(YouTube, id.(string)) {
				t.Fatalf("upload %v has YouTube video %v", upload["id"], id)
			}
		}
	}
}
