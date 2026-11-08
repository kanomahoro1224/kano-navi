package workflow

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const (
	biliSigningKey   = "7cd084941338484aae1ad9425b84077c"
	biliSigningSub   = "4932caff0ff746eab6f01bf08b70ac45"
	biliLoginCookies = "synthetic0login%2C1767225600%2Cabcd"
)

// dynamicsBody is one space feed page: an opus with two pictures and a
// forward of an older opus.
const dynamicsBody = `{"code":0,"data":{"items":[
 {"id_str":"1100000000000000001","type":"DYNAMIC_TYPE_DRAW","modules":{
   "module_author":{"pub_ts":1767258000,"name":"鹿乃","mid":316381099},
   "module_dynamic":{"major":{"opus":{"summary":{"text":"今週のスケジュールです"},
     "pics":[{"url":"https://i0.hdslb.com/bfs/new_dyn/one.jpg"},{"url":"https://i1.hdslb.com/bfs/new_dyn/two.jpg"}]}}}}},
 {"id_str":"1100000000000000003","type":"DYNAMIC_TYPE_FORWARD","modules":{
   "module_author":{"pub_ts":1767258000,"name":"鹿乃","mid":316381099},
   "module_dynamic":{"desc":{"text":"转发"}}},
  "orig":{"id_str":"1100000000000000009","type":"DYNAMIC_TYPE_DRAW","modules":{
   "module_author":{"pub_ts":1767171600,"name":"别的 UP","mid":42},
   "module_dynamic":{"desc":{"text":"原动态正文"},"major":{"draw":{"items":[{"src":"https://i2.hdslb.com/bfs/album/orig.jpg"}]}}}}}},
 {"id_str":"1100000000000000002","type":"DYNAMIC_TYPE_AV","modules":{
   "module_author":{"pub_ts":1767258000,"name":"鹿乃","mid":316381099},
   "module_dynamic":{"major":{"archive":{"bvid":"BV1AAAAAAAAA","title":"翻唱"}}}}}
],"offset":"1100000000000000002","has_more":true}}`

func TestBilibiliPostsStepStoresDynamicsOnlyForASignedInOperator(t *testing.T) {
	s, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "data", "database", "snapshot.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	requests, signed, feedReads := 0, false, 0
	rt := xTestRT(func(r *http.Request) (*http.Response, error) {
		requests++
		body := `{"code":0,"data":{"archives":[]}}`
		switch r.URL.Path {
		case "/x/frontend/finger/spi":
			body = `{"code":0,"data":{"b_3":"0A1B2C3D-0A1B-0A1B-0A1B-0A1B2C3D4E5F12345infoc","b_4":"synthetic-buvid4=="}}`
		case "/x/web-interface/nav":
			body = `{"code":0,"data":{"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/` + biliSigningKey + `.png","sub_url":"https://i0.hdslb.com/bfs/wbi/` + biliSigningSub + `.png"}}}`
		case "/x/polymer/web-dynamic/v1/feed/space":
			feedReads++
			query, _ := url.ParseQuery(r.URL.RawQuery)
			signed = len(query.Get("w_rid")) == 32 && query.Get("wts") != "" && query.Get("host_mid") == bilisource.DefaultAccount
			body = dynamicsBody
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	now := time.Now()
	adapter := bilisource.NewAdapter(bilisource.NewClient(&http.Client{Transport: rt}), bilisource.Options{
		Account: bilisource.DefaultAccount, MinInterval: 3 * time.Hour, PageSize: 20,
	})
	adapter.Now = func() time.Time { return now }
	steps := &SourceSteps{Store: s, Bilibili: adapter}
	run := func() AttemptResult {
		t.Helper()
		result, err := steps.Execute(context.Background(), StepInput{StepID: "bilibili-posts", WorkflowID: sqlite.AdhocSyncWorkflow})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	// Without a login cookie the space feed is skipped without a request.
	anonymous := run()
	if anonymous.Status != "succeeded" || anonymous.Result["skipped"] != true || anonymous.Result["reason"] != "login" || feedReads != 0 {
		t.Fatalf("anonymous=%#v feedReads=%d", anonymous, feedReads)
	}
	var posts int
	if err = s.DB().QueryRow(`SELECT COUNT(*) FROM posts WHERE source='bilibili'`).Scan(&posts); err != nil || posts != 0 {
		t.Fatalf("posts=%d err=%v", posts, err)
	}
	// A signed-in operator reads the feed once per interval with a signed
	// query, and its dynamics are stored as posts with their pictures.
	adapter.Options.SESSDATA = biliLoginCookies
	now = now.Add(4 * time.Hour)
	signedIn := run()
	if signedIn.Status != "succeeded" || signedIn.Result["posts"] != 2 || signedIn.Result["newPosts"] != 2 || !signed || feedReads != 1 {
		t.Fatalf("signedIn=%#v signed=%v feedReads=%d", signedIn, signed, feedReads)
	}
	var source, text, handle, postURL string
	if err = s.DB().QueryRow(`SELECT source,text,account_handle,url FROM posts WHERE id='1100000000000000001'`).Scan(&source, &text, &handle, &postURL); err != nil {
		t.Fatal(err)
	}
	if source != "bilibili" || text != "今週のスケジュールです" || handle != "鹿乃" || postURL != "https://t.bilibili.com/1100000000000000001" {
		t.Fatalf("post=%q %q %q %q", source, text, handle, postURL)
	}
	var pictures int
	if err = s.DB().QueryRow(`SELECT COUNT(*) FROM media_links WHERE owner_type='post' AND owner_id='1100000000000000001' AND role='post-image'`).Scan(&pictures); err != nil || pictures != 2 {
		t.Fatalf("pictures=%d err=%v", pictures, err)
	}
	// The repeated run waits for the interval and asks nothing.
	other := now.Add(30 * time.Minute)
	adapter.Now = func() time.Time { return other }
	again := run()
	if again.Result["reason"] != "interval" || feedReads != 1 {
		t.Fatalf("again=%#v feedReads=%d", again, feedReads)
	}
}

// The uploads step used to read the dynamics as a side effect, passing them
// the state of its own request: a video interval, backoff or failed read
// silently suppressed the feed. A source schedule now runs a step of its own
// for them, with its own last-read time in the account state.
func TestBilibiliDynamicsHaveTheirOwnScheduledStep(t *testing.T) {
	s, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "data", "database", "snapshot.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	feedReads := 0
	rt := xTestRT(func(r *http.Request) (*http.Response, error) {
		body := `{"code":0,"data":{"archives":[]}}`
		switch r.URL.Path {
		case "/x/frontend/finger/spi":
			body = `{"code":0,"data":{"b_3":"0A1B2C3D-0A1B-0A1B-0A1B-0A1B2C3D4E5F12345infoc","b_4":"synthetic-buvid4=="}}`
		case "/x/web-interface/nav":
			body = `{"code":0,"data":{"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/` + biliSigningKey + `.png","sub_url":"https://i0.hdslb.com/bfs/wbi/` + biliSigningSub + `.png"}}}`
		case "/x/polymer/web-dynamic/v1/feed/space":
			feedReads++
			body = dynamicsBody
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	adapter := bilisource.NewAdapter(bilisource.NewClient(&http.Client{Transport: rt}), bilisource.Options{
		Account: bilisource.DefaultAccount, MinInterval: 3 * time.Hour, PageSize: 20, SESSDATA: biliLoginCookies,
	})
	steps := &SourceSteps{Store: s, Bilibili: adapter}
	execute := func(id string) AttemptResult {
		t.Helper()
		out, execErr := steps.Execute(context.Background(), StepInput{StepID: id, WorkflowID: sqlite.AdhocSyncWorkflow})
		if execErr != nil {
			t.Fatal(execErr)
		}
		return out
	}
	// Uploads no longer touch the space feed, and their interval does not
	// decide when the dynamics are read.
	uploads := execute("bilibili")
	if uploads.Status != "succeeded" || uploads.Result["posts"] != nil || feedReads != 0 {
		t.Fatalf("uploads=%#v feedReads=%d", uploads, feedReads)
	}
	if skipped := execute("bilibili"); skipped.Result["reason"] != "interval" {
		t.Fatalf("uploads again=%#v", skipped)
	}
	posts := execute("bilibili-posts")
	if posts.Status != "succeeded" || posts.Result["posts"] != 2 || posts.Result["newPosts"] != 2 || feedReads != 1 {
		t.Fatalf("posts=%#v feedReads=%d", posts, feedReads)
	}
	var stored int
	if err = s.DB().QueryRow(`SELECT COUNT(*) FROM posts WHERE source='bilibili'`).Scan(&stored); err != nil || stored != 2 {
		t.Fatalf("stored=%d err=%v", stored, err)
	}
	if !slices.Contains(SourceChains["bilibili"], "bilibili-posts") {
		t.Fatalf("chain=%v", SourceChains["bilibili"])
	}
}
