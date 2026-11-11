package adminapi

import (
	"fmt"
	"net/http"
	"testing"
)

func TestBilibiliStatusPoolAndItsTranslations(t *testing.T) {
	c := newConsole(t, "development", "", &gatedSteps{})
	pool := func(id, mid, name, text, published, url string) {
		c.exec(t, `INSERT INTO bilibili_statuses(id,author_mid,author_name,text,published_at,url,raw_json,forwarded_at,first_seen_at,updated_at)
			VALUES(?,?,?,?,?,?,'{"mediaURLs":["https://i0.hdslb.com/bfs/new_dyn/synthetic.jpg"]}','2026-11-07T06:00:00Z','2026-11-07T06:00:00.000Z','2026-11-07T06:00:00.000Z')`,
			id, mid, name, text, published, url)
	}
	forward := func(id, quoted, published string, hidden bool) {
		var hiddenAt any
		if hidden {
			hiddenAt = "2026-11-07T07:00:00.000Z"
		}
		c.exec(t, `INSERT INTO posts(id,source,account_handle,type,text,published_at,url,quote_json,hidden_at) VALUES(?,'bilibili','316381099','daily','fwd',?,'https://www.bilibili.com/opus/1',?,?)`,
			id, published, fmt.Sprintf(`{"id":%q,"authorHandle":"42","text":"quoted","url":"https://www.bilibili.com/opus/1"}`, quoted), hiddenAt)
	}
	pool("bilibili-900000000000000501", "42", "Synthetic Agency", "明天20点直播", "2026-11-07T04:00:00Z", "https://www.bilibili.com/opus/900000000000000501")
	pool("bilibili-900000000000000502", "43", "Synthetic Studio", "新视频", "2026-11-07T05:00:00Z", "https://www.bilibili.com/video/BV1synthetic0")
	pool("bilibili-900000000000000503", "316381099", "Kano", "自己的", "2026-11-07T06:00:00Z", "https://www.bilibili.com/opus/900000000000000503")
	// The self-forward's original is a stored post, so Posts owns it.
	c.exec(t, `INSERT INTO posts(id,source,account_handle,type,text,published_at,url) VALUES('bilibili-900000000000000503','bilibili','316381099','daily','自己的','2026-11-07T06:00:00Z','https://www.bilibili.com/opus/900000000000000503')`)
	for i := 0; i < 6; i++ {
		forward(fmt.Sprintf("bilibili-90000000000000060%d", i), "bilibili-900000000000000501", fmt.Sprintf("2026-11-07T08:0%d:00Z", i), i > 0)
	}
	forward("bilibili-900000000000000610", "bilibili-900000000000000502", "2026-11-07T08:00:00Z", true)

	code, payload := c.call(t, http.MethodGet, "/api/admin/bilibili-statuses", "")
	items, _ := payload["items"].([]any)
	if code != http.StatusOK || payload["total"] != float64(2) || len(items) != 2 {
		t.Fatalf("list=%d %v", code, payload)
	}
	upload, dynamic := items[0].(map[string]any), items[1].(map[string]any)
	if upload["kind"] != "video" || upload["shown"] != "hidden" || dynamic["kind"] != "dynamic" || dynamic["shown"] != "public" ||
		dynamic["authorMid"] != "42" || dynamic["images"] != float64(1) || fmt.Sprint(dynamic["reasons"]) != "[forwarded]" {
		t.Fatalf("items=%v", items)
	}
	forwards := dynamic["forwardedBy"].([]any)
	if first := forwards[0].(map[string]any); len(forwards) != 5 || first["postId"] != "bilibili-900000000000000605" || first["hidden"] != true || first["quoteAvailable"] != true {
		t.Fatalf("forwardedBy=%v", forwards)
	}
	for path, want := range map[string]float64{
		"/api/admin/bilibili-statuses?kind=video":                     1,
		"/api/admin/bilibili-statuses?kind=dynamic&author=agency":     1,
		"/api/admin/bilibili-statuses?author=43":                      1,
		"/api/admin/bilibili-statuses?search=900000000000000501":      1,
		"/api/admin/bilibili-statuses?search=%E8%87%AA%E5%B7%B1":      0,
		"/api/admin/bilibili-statuses?page=9&pageSize=1&kind=dynamic": 1,
		"/api/admin/bilibili-statuses?author=Synthetic&pageSize=1000": 2,
	} {
		if code, payload := c.call(t, http.MethodGet, path, ""); code != http.StatusOK || payload["total"] != want {
			t.Fatalf("%s=%d %v", path, code, payload)
		}
	}
	if code, payload := c.call(t, http.MethodGet, "/api/admin/bilibili-statuses?kind=post", ""); code != http.StatusBadRequest || payload["error"] != "invalid_input" {
		t.Fatalf("kind=%d %v", code, payload)
	}
	// Read-only: no decision route.
	if code, _ := c.call(t, http.MethodPut, "/api/admin/bilibili-statuses/bilibili-900000000000000501/decision", `{"decision":"promote","reason":"x"}`); code != http.StatusNotFound && code != http.StatusMethodNotAllowed {
		t.Fatalf("decision=%d", code)
	}

	// The translation routes edit, clear and request the bilibili_status owner.
	code, payload = c.call(t, http.MethodPut, "/api/admin/translations/bilibili_status/bilibili-900000000000000501/en", `{"text":"Live at 20:00 tomorrow"}`)
	en := payload["translation"].(map[string]any)["locales"].([]any)[0].(map[string]any)
	if code != http.StatusOK || en["origin"] != "manual" || en["text"] != "Live at 20:00 tomorrow" {
		t.Fatalf("put=%d %v", code, payload)
	}
	_, payload = c.call(t, http.MethodGet, "/api/admin/bilibili-statuses?kind=dynamic", "")
	summary := payload["items"].([]any)[0].(map[string]any)["translation"].(map[string]any)
	if summary["status"] != "incomplete" || summary["locales"].([]any)[0].(map[string]any)["manual"] != "Live at 20:00 tomorrow" {
		t.Fatalf("summary=%v", summary)
	}
	if code, payload = c.call(t, http.MethodPut, "/api/admin/translations/bilibili_status/bilibili-900000000000000999/en", `{"text":"x"}`); code != http.StatusNotFound {
		t.Fatalf("missing owner=%d %v", code, payload)
	}
	code, payload = c.call(t, http.MethodDelete, "/api/admin/translations/bilibili_status/bilibili-900000000000000501/en", "")
	if en = payload["translation"].(map[string]any)["locales"].([]any)[0].(map[string]any); code != http.StatusOK || en["origin"] != nil {
		t.Fatalf("delete=%d %v", code, payload)
	}
	code, payload = c.call(t, http.MethodPost, "/api/admin/translations/request", `{"ownerType":"bilibili_status","ids":["bilibili-900000000000000501","bilibili-900000000000000503"]}`)
	if code != http.StatusAccepted || payload["queued"] != float64(1) {
		t.Fatalf("request=%d %v", code, payload)
	}
	c.waitForRun(t, runID(t, payload), "completed")
	if got := <-c.steps.seen; got != "translate" {
		t.Fatalf("step=%s", got)
	}
	_, payload = c.call(t, http.MethodGet, "/api/admin/bilibili-statuses?kind=dynamic", "")
	if summary = payload["items"].([]any)[0].(map[string]any)["translation"].(map[string]any); summary["requested"] != true {
		t.Fatalf("requested=%v", summary)
	}
}
