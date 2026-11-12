package workflow

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
	ytsource "github.com/yexca/kano-navi/backend/internal/source/youtube"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const (
	syntheticTicket = `{"code":0,"data":{"ticket":"eyJhbGciOiJIUzI1NiJ9.eyJzeW50aGV0aWMiOnRydWV9.c3ludGhldGljLXNpZ25hdHVyZQ","created_at":1767225600,"ttl":259200,"nav":{"img":"","sub":""}}}`
	syntheticLogin  = "synthetic0login%2C1767225600%2Cabcde%2A11"
	syntheticNav    = `{"code":0,"data":{"isLogin":true,"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/0123456789abcdef0123456789abcdef.png","sub_url":"https://i0.hdslb.com/bfs/wbi/fedcba9876543210fedcba9876543210.png"}}}`
	syntheticFeed   = `{"code":0,"data":{"has_more":false,"items":[{"id_str":"900000000000000010","type":"DYNAMIC_TYPE_WORD","modules":{"module_author":{"mid":316381099,"pub_ts":1794024288},"module_dynamic":{"desc":{"rich_text_nodes":[{"type":"RICH_TEXT_NODE_TYPE_TEXT","text":"今天20点开播"}]}}}}]}}`
)

func biliResponder(t *testing.T, paths *[]string, live string) func(*http.Request) (*http.Response, error) {
	return func(r *http.Request) (*http.Response, error) {
		*paths = append(*paths, r.URL.Host+r.URL.Path)
		body := ""
		switch r.URL.Path {
		case "/x/frontend/finger/spi":
			body = `{"code":0,"data":{"b_3":"0A1B2C3D-0A1B-0A1B-0A1B-0A1B2C3D4E5F12345infoc","b_4":"synthetic-buvid4=="}}`
		case "/x/series/recArchivesByKeywords":
			body = `{"code":0,"data":{"archives":[]}}`
		case "/x/web-interface/nav":
			body = syntheticNav
		case "/bapis/bilibili.api.ticket.v1.Ticket/GenWebTicket":
			// A ticket issued now, so later runs reuse it.
			body = strings.Replace(syntheticTicket, "1767225600", strconv.FormatInt(time.Now().Unix(), 10), 1)
		case "/x/polymer/web-dynamic/desktop/v1/feed/space":
			body = syntheticFeed
		case "/x/polymer/web-dynamic/v1/feed/space":
			body = syntheticFeed
		case "/room/v1/Room/get_status_info_by_uids":
			body = live
		default:
			t.Fatalf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}
}

func TestBilibiliDynamicsStepReadsOnlyWithALoginOnItsOwnClock(t *testing.T) {
	s, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "data", "database", "snapshot.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	paths := []string{}
	adapter := bilisource.NewAdapter(bilisource.NewClient(&http.Client{Transport: xTestRT(biliResponder(t, &paths, ""))}),
		bilisource.Options{Account: bilisource.DefaultAccount, MinInterval: 3 * time.Hour, PageSize: 20})
	steps := &SourceSteps{Store: s, Bilibili: adapter}
	run := func(step string) AttemptResult {
		t.Helper()
		result, err := steps.Execute(context.Background(), StepInput{StepID: step, WorkflowID: sqlite.AdhocSyncWorkflow})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	// The upload step reads only the upload list.
	uploads := run("bilibili")
	if uploads.Status != "succeeded" || uploads.Result["dynamics"] != nil || strings.Contains(strings.Join(paths, ","), "feed/space") {
		t.Fatalf("uploads=%#v paths=%v", uploads, paths)
	}
	// Without a login the dynamics step is skipped without a request.
	paths = nil
	anonymous := run(sqlite.BilibiliDynamicsSource)
	if anonymous.Status != "succeeded" || anonymous.Result["reason"] != "login_required" || anonymous.Result["requested"] != 0 || len(paths) != 0 {
		t.Fatalf("anonymous=%#v paths=%v", anonymous, paths)
	}
	// With a login it checks the cookie and reads the listing, reusing the
	// device identity the upload read left.
	adapter.Options.SESSDATA = syntheticLogin
	clock := time.Now().Add(4 * time.Hour)
	adapter.Now = func() time.Time { return clock }
	paths = nil
	signedIn := run(sqlite.BilibiliDynamicsSource)
	if signedIn.Status != "succeeded" || signedIn.Result["posts"] != 1 || signedIn.Result["changed"] != true || signedIn.Result["listing"] != bilisource.MethodDesktop || strings.Join(paths, ",") != "api.bilibili.com/x/web-interface/nav,api.bilibili.com/x/polymer/web-dynamic/desktop/v1/feed/space" {
		t.Fatalf("signed in=%#v paths=%v", signedIn, paths)
	}
	page, err := s.ListAdminPosts(context.Background(), sqlite.AdminPostFilter{Page: 1, PageSize: 10, Source: "bilibili"})
	if err != nil || page.Total != 1 || page.Items[0].ID != "bilibili-900000000000000010" || page.Items[0].Translation == nil {
		t.Fatalf("posts=%#v err=%v", page, err)
	}
	// Each read keeps its own interval: the listing waits, the upload list,
	// last read four hours ago, is due.
	paths = nil
	if again := run(sqlite.BilibiliDynamicsSource); again.Result["reason"] != "interval" || len(paths) != 0 {
		t.Fatalf("again=%#v paths=%v", again, paths)
	}
	clock = clock.Add(4 * time.Hour)
	paths = nil
	chain := sqlite.SourceChain{Methods: []sqlite.ChainMethod{{ID: bilisource.MethodDynamics, Enabled: true}, {ID: bilisource.MethodDesktop, Enabled: false}}}
	configured, err := steps.Execute(context.Background(), StepInput{StepID: sqlite.BilibiliDynamicsSource, WorkflowID: sqlite.AdhocSyncWorkflow, Chain: &chain})
	if err != nil || configured.Status != "succeeded" || configured.Result["listing"] != bilisource.MethodDynamics || strings.Join(paths, ",") != "api.bilibili.com/x/polymer/web-dynamic/v1/feed/space" {
		t.Fatalf("configured=%#v paths=%v err=%v", configured, paths, err)
	}
	paths = nil
	if uploads = run("bilibili"); uploads.Status != "succeeded" || strings.Join(paths, ",") != "api.bilibili.com/x/series/recArchivesByKeywords" {
		t.Fatalf("uploads due=%#v paths=%v", uploads, paths)
	}
}

func TestLiveStepChecksTheBilibiliRoomOncePerDueTime(t *testing.T) {
	s, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "data", "database", "snapshot.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	paths := []string{}
	live := `{"code":0,"data":{"316381099":{"title":"synthetic","room_id":1000001,"live_status":1,"live_time":1794049200}}}`
	adapter := bilisource.NewAdapter(bilisource.NewClient(&http.Client{Transport: xTestRT(biliResponder(t, &paths, live))}),
		bilisource.Options{Account: bilisource.DefaultAccount, MinInterval: 3 * time.Hour})
	clock := time.Now()
	adapter.Now = func() time.Time { return clock }
	steps := &SourceSteps{Store: s, Bilibili: adapter}
	run := func() AttemptResult {
		t.Helper()
		result, err := steps.Execute(context.Background(), StepInput{StepID: "live", WorkflowID: sqlite.AdhocLiveWorkflow})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := run()
	checked, _ := first.Result["bilibili"].(map[string]any)
	if first.Status != "succeeded" || checked["requested"] != 1 || checked["live"] != true || checked["phase"] != sqlite.LivePhaseBaseline || len(paths) != 1 || paths[0] != "api.live.bilibili.com/room/v1/Room/get_status_info_by_uids" {
		t.Fatalf("first=%#v paths=%v", first, paths)
	}
	// Within the baseline interval nothing is due.
	clock = clock.Add(10 * time.Minute)
	if again := run(); again.Result["bilibili"] != nil || len(paths) != 1 {
		t.Fatalf("again=%#v paths=%v", again, paths)
	}
	// SKIP_BILIBILI leaves the room alone.
	clock = clock.Add(time.Hour)
	steps.Skip = map[string]bool{"bilibili": true}
	if skipped := run(); skipped.Result["bilibili"] != nil || len(paths) != 1 {
		t.Fatalf("skipped=%#v paths=%v", skipped, paths)
	}
	// The probe queues a run only when the room is due.
	probe := &LiveProbe{Store: s, Enabled: true, BilibiliAccount: bilisource.DefaultAccount, Now: func() time.Time { return clock },
		Submit: func(context.Context) (bool, error) { return true, nil }}
	if accepted, err := probe.Tick(context.Background()); err != nil || !accepted {
		t.Fatalf("due tick=%v err=%v", accepted, err)
	}
	probe.Now = func() time.Time { return clock.Add(-65 * time.Minute) }
	if accepted, _ := probe.Tick(context.Background()); accepted {
		t.Fatal("a tick within the interval queued a run")
	}
}

func TestYoutubeProbeSwitchStopsYoutubeRequestsWhileBilibiliIsDue(t *testing.T) {
	s, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "data", "database", "snapshot.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	// A YouTube reservation that would be due.
	reservation := ytsource.Video{ID: "AAAAAAAAAAA", Title: "synthetic stream", PublishedAt: now.Add(-24 * time.Hour).Format(time.RFC3339), ScheduledAt: now.Add(-time.Minute).Format(time.RFC3339),
		URL: "https://www.youtube.com/watch?v=AAAAAAAAAAA", Kind: "UPCOMING LIVE", IsUpcoming: true, FeedOnly: true}
	if err = s.WriteYoutubeRecent(ctx, "UCAAAAAAAAAAAAAAAAAAAAAA", ytsource.Result{Videos: []ytsource.Video{reservation}, NextCursor: "AAAAAAAAAAA", NextCursorTime: reservation.PublishedAt}); err != nil {
		t.Fatal(err)
	}
	youtubeRequests := 0
	yt := ytsource.NewAdapter(ytsource.NewClient(&http.Client{Transport: xTestRT(func(r *http.Request) (*http.Response, error) {
		youtubeRequests++
		return &http.Response{StatusCode: 500, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}), ytsource.Options{ChannelID: "UCAAAAAAAAAAAAAAAAAAAAAA", MaxDetailRequests: 1})
	paths := []string{}
	bili := bilisource.NewAdapter(bilisource.NewClient(&http.Client{Transport: xTestRT(biliResponder(t, &paths, `{"code":0,"data":{"316381099":{"title":"t","room_id":1000001,"live_status":0,"live_time":0}}}`))}),
		bilisource.Options{Account: bilisource.DefaultAccount, MinInterval: 3 * time.Hour})
	if err = s.SaveLiveProbeSettings(ctx, sqlite.LiveProbeSettings{Enabled: false, IntervalMinutes: 15, BilibiliEnabled: true, BilibiliIntervalMinutes: 30}); err != nil {
		t.Fatal(err)
	}
	steps := &SourceSteps{Store: s, Youtube: yt, Bilibili: bili}
	result, err := steps.Execute(ctx, StepInput{StepID: "live", WorkflowID: sqlite.AdhocLiveWorkflow})
	if err != nil {
		t.Fatal(err)
	}
	checked, _ := result.Result["bilibili"].(map[string]any)
	if youtubeRequests != 0 || result.Result["disabled"] != true || checked["requested"] != 1 || len(paths) != 1 {
		t.Fatalf("result=%#v youtube=%d paths=%v", result, youtubeRequests, paths)
	}
}
