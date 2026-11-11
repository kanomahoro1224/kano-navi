package adminapi

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// syntheticOpus is a www.bilibili.com/opus page of dynamic 900000000000000010
// by owner, of the given comment area.
func syntheticOpus(owner, commentType string) string {
	return `<html><script>window.__INITIAL_STATE__={"detail":{"id_str":"900000000000000010","basic":{"comment_type":` + commentType + `},"modules":[
 {"module_type":"MODULE_TYPE_AUTHOR","module_author":{"mid":` + owner + `,"pub_ts":1794024288}},
 {"module_type":"MODULE_TYPE_CONTENT","module_content":{"paragraphs":[{"para_type":1,"text":{"nodes":[{"type":"TEXT_NODE_TYPE_WORD","word":{"words":"今天20点开播"}}]}}]}}]}};(function(){}())</script></html>`
}

func TestBilibiliDynamicsAreAddedByOperatorLink(t *testing.T) {
	store, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "database", "kano.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	owner, commentType, status, requests := "316381099", "11", 200, 0
	client := bilisource.NewClient(&http.Client{Transport: bilibiliTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		body, code := "", 200
		switch r.URL.Path {
		case "/x/frontend/finger/spi":
			body = `{"code":0,"data":{"b_3":"0A1B2C3D-0A1B-0A1B-0A1B-0A1B2C3D4E5F12345infoc","b_4":"synthetic-buvid4=="}}`
		case "/opus/900000000000000010":
			body, code = syntheticOpus(owner, commentType), status
		default:
			body = `{"code":-400}`
		}
		return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	adapter := bilisource.NewAdapter(client, bilisource.Options{Account: "316381099", MinInterval: 3 * time.Hour, PageSize: 20})
	c := &console{store: store, handler: New(Options{Store: store, Auth: newAuth(t, "development", ""), Bilibili: adapter})}

	for body, want := range map[string]int{`{"input":"https://www.bilibili.com/video/BV1AAAAAAAAA"}`: 400, `{}`: 400, `{"input":"https://t.bilibili.com/900000000000000010","force":"yes"}`: 400} {
		if code, payload := c.call(t, "POST", "/api/admin/posts/bilibili", body); code != want {
			t.Fatalf("%s=%d %v", body, code, payload)
		}
	}
	owner = "42"
	if code, payload := c.call(t, "POST", "/api/admin/posts/bilibili", `{"input":"https://t.bilibili.com/900000000000000010"}`); code != 400 || payload["error"] != "invalid_input" {
		t.Fatalf("other account=%d %v", code, payload)
	}
	owner, commentType = "316381099", "1"
	if code, payload := c.call(t, "POST", "/api/admin/posts/bilibili", `{"input":"https://t.bilibili.com/900000000000000010"}`); code != 400 || payload["error"] != "bilibili_unsupported_dynamic" {
		t.Fatalf("video dynamic=%d %v", code, payload)
	}
	commentType = "11"
	// A stored post whose ID merely contains the dynamic's is not returned.
	if _, err = store.DB().Exec(`INSERT INTO posts (id, source, account_handle, type, text, published_at, url) VALUES ('bilibili-9000000000000000100', 'bilibili', '316381099', 'daily', 'other', '2026-11-08T00:00:00Z', 'https://www.bilibili.com/opus/9000000000000000100')`); err != nil {
		t.Fatal(err)
	}
	code, payload := c.call(t, "POST", "/api/admin/posts/bilibili", `{"input":"https://www.bilibili.com/opus/900000000000000010"}`)
	post, _ := payload["post"].(map[string]any)
	if code != 200 || post["id"] != "bilibili-900000000000000010" || post["text"] != "今天20点开播" || post["source"] != "bilibili" {
		t.Fatalf("add=%d %v", code, payload)
	}

	// A captcha starts the backoff and is saved; the next read waits unless
	// the operator forces it once.
	status = 412
	if code, payload = c.call(t, "POST", "/api/admin/posts/bilibili", `{"input":"https://t.bilibili.com/900000000000000010"}`); code != 503 ||
		payload["error"] != "bilibili_blocked" || payload["blockedUntil"] == nil || payload["force"] != true || payload["code"] != "bilibili_risk_control_http" {
		t.Fatalf("captcha=%d %v", code, payload)
	}
	state, err := store.BilibiliState(context.Background(), "316381099")
	if err != nil || !state.BlockedUntil.After(time.Now()) {
		t.Fatalf("state=%#v err=%v", state, err)
	}
	before := requests
	if code, payload = c.call(t, "POST", "/api/admin/posts/bilibili", `{"input":"https://t.bilibili.com/900000000000000010"}`); code != 503 || payload["code"] != "bilibili_blocked" || requests != before {
		t.Fatalf("waiting=%d %v requests %d→%d", code, payload, before, requests)
	}
	status = 200
	if code, payload = c.call(t, "POST", "/api/admin/posts/bilibili", `{"input":"https://t.bilibili.com/900000000000000010","force":true}`); code != 200 || requests == before {
		t.Fatalf("forced=%d %v", code, payload)
	}
	if state, err = store.BilibiliState(context.Background(), "316381099"); err != nil || !state.BlockedUntil.IsZero() {
		t.Fatalf("forced success kept the block: %#v %v", state, err)
	}

	// Without the source the route refuses.
	off := &console{store: store, handler: New(Options{Store: store, Auth: newAuth(t, "development", ""), Bilibili: adapter, SkippedSteps: []string{"bilibili"}})}
	if code, payload = off.call(t, "POST", "/api/admin/posts/bilibili", `{"input":"https://t.bilibili.com/900000000000000010"}`); code != 400 {
		t.Fatalf("disabled=%d %v", code, payload)
	}
}
