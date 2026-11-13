package journey

import (
	"encoding/json"
	"regexp"
	"testing"
)

var (
	localURL  = regexp.MustCompile(`^/media/(?:journey|works)/[0-9a-f]{12}\.(?:jpg|png)$`)
	isoDate   = regexp.MustCompile(`^[0-9]{4}(?:-[0-9]{2}(?:-[0-9]{2})?)?$`)
	locales   = []string{"ja", "zh-CN", "en"}
	protected = []string{"sha256", "byteSize", "filename", "checkedAt", "mediaId", "revision", "milestoneId"}
)

// walk visits every object of a decoded JSON value.
func walk(value any, visit func(map[string]any)) {
	switch typed := value.(type) {
	case map[string]any:
		visit(typed)
		for _, child := range typed {
			walk(child, visit)
		}
	case []any:
		for _, child := range typed {
			walk(child, visit)
		}
	}
}

func checkText(t *testing.T, where string, value any) {
	t.Helper()
	text, ok := value.(map[string]any)
	if !ok || len(text) != len(locales) {
		t.Fatalf("%s is not a localized text: %#v", where, value)
	}
	for _, locale := range locales {
		if _, ok := text[locale]; !ok {
			t.Fatalf("%s lacks %s: %#v", where, locale, value)
		}
	}
}

func checkMedia(t *testing.T, where string, value any) {
	t.Helper()
	item := value.(map[string]any)
	url, _ := item["url"].(string)
	if !localURL.MatchString(url) || item["width"].(float64) < 1 || item["height"].(float64) < 1 || item["mimeType"] == nil {
		t.Fatalf("%s media=%#v", where, item)
	}
	checkText(t, where+".title", item["title"])
	switch item["origin"] {
	case "fixed":
		if item["releaseId"] != nil || item["cover"] != nil || item["sourcePage"] == nil {
			t.Fatalf("%s fixed media=%#v", where, item)
		}
	case "release":
		cover, _ := item["cover"].(map[string]any)
		if item["releaseId"] == nil || cover == nil || cover["url"] != url {
			t.Fatalf("%s release cover=%#v", where, item)
		}
	default:
		t.Fatalf("%s origin=%v", where, item["origin"])
	}
}

func TestPackagedJourneyKeepsItsContract(t *testing.T) {
	var body map[string]any
	if err := json.Unmarshal(Packaged(), &body); err != nil {
		t.Fatal(err)
	}
	if updated, _ := body["updatedAt"].(string); len(updated) != 10 || !isoDate.MatchString(updated) {
		t.Fatalf("updatedAt=%v", body["updatedAt"])
	}
	branches := map[string]bool{}
	for _, raw := range body["branches"].([]any) {
		branch := raw.(map[string]any)
		branches[branch["id"].(string)] = true
		checkText(t, "branch label", branch["label"])
		checkMedia(t, "portrait", branch["portrait"])
	}
	milestones := body["milestones"].([]any)
	previous := ""
	for _, raw := range milestones {
		milestone := raw.(map[string]any)
		id, date := milestone["id"].(string), milestone["date"].(string)
		precision := map[int]string{4: "year", 7: "month", 10: "day"}[len(date)]
		if !isoDate.MatchString(date) || milestone["datePrecision"] != precision || date < previous || !branches[milestone["branch"].(string)] {
			t.Fatalf("milestone %s date=%s precision=%v branch=%v", id, date, milestone["datePrecision"], milestone["branch"])
		}
		previous = date
		checkText(t, id+".title", milestone["title"])
		for _, link := range milestone["links"].([]any) {
			if label := link.(map[string]any)["label"].(string); regexp.MustCompile(`[.]`).MatchString(label) {
				t.Fatalf("%s link label is a message key: %s", id, label)
			}
		}
		for _, item := range milestone["media"].([]any) {
			checkMedia(t, id, item)
		}
	}
	if len(milestones) == 0 {
		t.Fatal("no milestones")
	}
	for _, item := range body["galleryOnly"].([]any) {
		checkMedia(t, "gallery", item)
	}
	walk(body, func(object map[string]any) {
		for _, key := range protected {
			if _, ok := object[key]; ok {
				t.Fatalf("the body exposes %s: %#v", key, object)
			}
		}
	})
}
