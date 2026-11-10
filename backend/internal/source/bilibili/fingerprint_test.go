package bilibili

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
	"time"
)

const syntheticLogin = "synthetic0login%2C1767225600%2Cabcde%2A11"

// recorder answers every Bilibili path with a synthetic body and keeps the
// requests.
type recorder struct {
	requests []*http.Request
	replies  map[string]string
	status   map[string]int
}

func (r *recorder) client() *Client {
	return NewClient(&http.Client{Transport: fakeTransport(func(req *http.Request) (*http.Response, error) {
		r.requests = append(r.requests, req)
		status := 200
		if s, ok := r.status[req.URL.Path]; ok {
			status = s
		}
		return response(status, r.replies[req.URL.Path]), nil
	})})
}

func (r *recorder) request(path string) *http.Request {
	for _, req := range r.requests {
		if req.URL.Path == path {
			return req
		}
	}
	return nil
}

func newRecorder() *recorder {
	return &recorder{status: map[string]int{}, replies: map[string]string{
		"/x/frontend/finger/spi":                session,
		ticketPath:                              ticket,
		"/x/web-interface/nav":                  navKeys,
		"/x/polymer/web-dynamic/desktop/v1/feed/space": spaceFeed,
		"/x/polymer/web-dynamic/v1/feed/space":  spaceFeed,
		"/x/series/recArchivesByKeywords":       uploads,
		"/x/web-interface/view":                 `{"code":0,"data":{"bvid":"BV1AAAAAAAAA","aid":1,"title":"song","pubdate":1767258000,"duration":60,"owner":{"mid":316381099}}}`,
		"/opus/900000000000000010":              opusPage,
		"/room/v1/Room/get_status_info_by_uids": `{"code":0,"data":{"316381099":{"title":"t","room_id":1000001,"live_status":0,"live_time":0}}}`,
	}}
}

func TestRequestsLookLikeTheBrowserPagesTheyComeFrom(t *testing.T) {
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	rec := newRecorder()
	a := NewAdapter(rec.client(), Options{Account: DefaultAccount, MinInterval: 3 * time.Hour, PageSize: 20, SESSDATA: syntheticLogin})
	a.Now = func() time.Time { return now }
	result, err := a.FetchRecent(context.Background(), State{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.FetchDynamics(context.Background(), result.State); err != nil {
		t.Fatal(err)
	}
	if _, _, err = a.FetchVideo(context.Background(), result.State, "BV1AAAAAAAAA", false); err != nil {
		t.Fatal(err)
	}
	if _, _, err = a.FetchDynamic(context.Background(), result.State, "900000000000000010", false); err != nil {
		t.Fatal(err)
	}
	if _, _, err = a.CheckLive(context.Background(), LiveState{}); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]struct{ referer, origin, method string }{
		"/x/frontend/finger/spi":                {"https://www.bilibili.com/", "https://www.bilibili.com", "GET"},
		ticketPath:                              {"https://www.bilibili.com/", "https://www.bilibili.com", "POST"},
		"/x/web-interface/nav":                  {"https://www.bilibili.com/", "https://www.bilibili.com", "GET"},
		"/x/series/recArchivesByKeywords":       {"https://space.bilibili.com/316381099", "https://space.bilibili.com", "GET"},
		"/x/polymer/web-dynamic/desktop/v1/feed/space": {"https://space.bilibili.com/316381099/dynamic", "https://space.bilibili.com", "GET"},
		"/x/web-interface/view":                 {"https://www.bilibili.com/video/BV1AAAAAAAAA", "https://www.bilibili.com", "GET"},
		"/opus/900000000000000010":              {"https://www.bilibili.com/", "", "GET"},
		"/room/v1/Room/get_status_info_by_uids": {"https://live.bilibili.com/", "https://live.bilibili.com", "GET"},
	} {
		req := rec.request(path)
		if req == nil {
			t.Fatalf("%s was not requested", path)
		}
		if req.Method != want.method || req.Header.Get("User-Agent") != BrowserUserAgent || req.Header.Get("Accept-Language") != "zh-CN,zh;q=0.9" ||
			req.Header.Get("Referer") != want.referer || req.Header.Get("Origin") != want.origin {
			t.Fatalf("%s headers=%v method=%s", path, req.Header, req.Method)
		}
		cookie := req.Header.Get("Cookie")
		switch path {
		case "/x/frontend/finger/spi", "/room/v1/Room/get_status_info_by_uids":
			if cookie != "" {
				t.Fatalf("%s carried cookies %q", path, cookie)
			}
		case ticketPath:
			if !strings.Contains(cookie, "b_nut=1767312000") || strings.Contains(cookie, "SESSDATA") || strings.Contains(cookie, "bili_ticket") {
				t.Fatalf("ticket cookie=%q", cookie)
			}
		default:
			wantLogin := path != "/opus/900000000000000010"
			if !strings.Contains(cookie, "b_nut=1767312000") || !strings.Contains(cookie, "bili_ticket=eyJ") || !strings.Contains(cookie, "bili_ticket_expires=1767484800") ||
				strings.Contains(cookie, "SESSDATA="+syntheticLogin) != wantLogin {
				t.Fatalf("%s cookie=%q", path, cookie)
			}
		}
	}
}

func TestTicketIsSignedCachedAndRenewedBeforeItExpires(t *testing.T) {
	now := time.Unix(1767225600, 0).UTC()
	rec := newRecorder()
	a := NewAdapter(rec.client(), Options{Account: DefaultAccount, MinInterval: time.Hour, PageSize: 20})
	a.Now = func() time.Time { return now }
	result, err := a.FetchRecent(context.Background(), State{})
	if err != nil {
		t.Fatal(err)
	}
	req := rec.request(ticketPath)
	mac := hmac.New(sha256.New, []byte("XgwSnGZ1p"))
	mac.Write([]byte("ts1767225600"))
	q := req.URL.Query()
	if q.Get("key_id") != "ec02" || q.Get("hexsign") != hex.EncodeToString(mac.Sum(nil)) || q.Get("context[ts]") != "1767225600" || !q.Has("csrf") || q.Get("csrf") != "" {
		t.Fatalf("ticket query=%v", q)
	}
	// The ticket also brings the WBI keys, so the listing needs no nav for
	// them.
	if result.State.Session.TicketAt != 1767225600 || result.State.Session.TicketTTL != 259200 || !result.State.WBI.valid() {
		t.Fatalf("state=%#v", result.State)
	}
	count := func() int {
		n := 0
		for _, r := range rec.requests {
			if r.URL.Path == ticketPath {
				n++
			}
		}
		return n
	}
	// Reused while fresh; renewed six hours before it expires.
	now = now.Add(2 * 24 * time.Hour)
	if result, err = a.FetchRecent(context.Background(), result.State); err != nil || count() != 1 {
		t.Fatalf("fresh ticket was requested again: %d %v", count(), err)
	}
	now = now.Add(19 * time.Hour)
	if result, err = a.FetchRecent(context.Background(), result.State); err != nil || count() != 2 {
		t.Fatalf("expiring ticket was not renewed: %d %v", count(), err)
	}
	// A failed ticket request is not a failure of the read.
	rec.status[ticketPath] = 412
	now = now.Add(3 * 24 * time.Hour)
	if failed, err := a.FetchRecent(context.Background(), result.State); err != nil || len(failed.Videos) != 2 || !failed.State.BlockedUntil.IsZero() {
		t.Fatalf("ticket failure failed the read: %#v %v", failed, err)
	}
}

func TestLoginIsVerifiedDailyAndAnExpiredCookieHoldsTheListing(t *testing.T) {
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	rec := newRecorder()
	a := NewAdapter(rec.client(), Options{Account: DefaultAccount, MinInterval: 3 * time.Hour, PageSize: 20, SESSDATA: syntheticLogin})
	a.Now = func() time.Time { return now }
	if got := a.LoginStatus(State{}); got != "unchecked" {
		t.Fatalf("status=%s", got)
	}
	rec.replies["/x/web-interface/nav"] = `{"code":-101,"data":{"isLogin":false,"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/0123456789abcdef0123456789abcdef.png","sub_url":"https://i0.hdslb.com/bfs/wbi/fedcba9876543210fedcba9876543210.png"}}}`
	expired, err := a.FetchDynamics(context.Background(), State{})
	if err != nil || expired.Skipped != "login_expired" || expired.State.Dynamics.LastError != "bilibili_login_expired" || a.LoginStatus(expired.State) != "expired" ||
		rec.request("/x/polymer/web-dynamic/v1/feed/space") != nil {
		t.Fatalf("expired=%#v err=%v", expired, err)
	}
	// Until the next daily check: no listing, no nav, and no cookie on other
	// reads.
	rec.requests = nil
	now = now.Add(4 * time.Hour)
	if again, _ := a.FetchDynamics(context.Background(), expired.State); again.Skipped != "login_expired" || len(rec.requests) != 0 {
		t.Fatalf("again=%#v requests=%d", again, len(rec.requests))
	}
	if _, err = a.FetchRecent(context.Background(), expired.State); err != nil || strings.Contains(rec.request("/x/series/recArchivesByKeywords").Header.Get("Cookie"), "SESSDATA") {
		t.Fatalf("an expired cookie was sent: %v", err)
	}
	// The next day's check sends the cookie again and finds it renewed.
	rec.replies["/x/web-interface/nav"] = navKeys
	now = now.Add(21 * time.Hour)
	renewed, err := a.FetchDynamics(context.Background(), expired.State)
	if err != nil || renewed.Skipped != "" || len(renewed.Dynamics) != 3 || a.LoginStatus(renewed.State) != "signed_in" ||
		!strings.Contains(rec.request("/x/web-interface/nav").Header.Get("Cookie"), "SESSDATA="+syntheticLogin) {
		t.Fatalf("renewed=%#v err=%v", renewed, err)
	}
	if got := NewAdapter(nil, Options{}).LoginStatus(State{}); got != "not_configured" {
		t.Fatalf("status=%s", got)
	}
}

func TestEverySplitRiskControlCodeStartsTheBackoff(t *testing.T) {
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	for _, code := range []string{"bilibili_risk_control_http", "bilibili_risk_control_page", "bilibili_risk_control_352", "bilibili_risk_control_412", "bilibili_risk_control_voucher", "bilibili_rate_limited"} {
		if !IsRiskControl(code) {
			t.Fatalf("%s is not risk control", code)
		}
		if state := failed(State{}, &Error{code}, now); !state.BlockedUntil.Equal(now.Add(6*time.Hour)) || state.Failures != 1 {
			t.Fatalf("%s state=%#v", code, state)
		}
	}
	if IsRiskControl("bilibili_login_expired") || IsRiskControl("bilibili_blocked") {
		t.Fatal("a non-captcha code is risk control")
	}
}

func TestForceSendsOneOperatorReadDuringTheBackoff(t *testing.T) {
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	rec := newRecorder()
	a := NewAdapter(rec.client(), Options{Account: DefaultAccount, MinInterval: 3 * time.Hour, PageSize: 20, SESSDATA: syntheticLogin})
	a.Now = func() time.Time { return now }
	blocked := State{BlockedUntil: now.Add(time.Hour), Failures: 2, LastError: "bilibili_risk_control_352"}
	if _, _, err := a.FetchVideo(context.Background(), blocked, "BV1AAAAAAAAA", false); ErrorCode(err) != "bilibili_blocked" || len(rec.requests) != 0 {
		t.Fatalf("unforced err=%v requests=%d", err, len(rec.requests))
	}
	v, state, err := a.FetchVideo(context.Background(), blocked, "BV1AAAAAAAAA", true)
	if err != nil || v.BVID != "BV1AAAAAAAAA" || !state.BlockedUntil.IsZero() || state.Failures != 0 {
		t.Fatalf("forced=%#v state=%#v err=%v", v, state, err)
	}
	// A forced failure extends the backoff as usual.
	rec.status["/opus/900000000000000010"] = 412
	_, failedState, err := a.FetchDynamic(context.Background(), blocked, "900000000000000010", true)
	if ErrorCode(err) != "bilibili_risk_control_http" || failedState.Failures != 3 || !failedState.BlockedUntil.After(blocked.BlockedUntil) {
		t.Fatalf("forced failure state=%#v err=%v", failedState, err)
	}
	// Listing and timer reads never force.
	rec.requests = nil
	if result, _ := a.FetchRecent(context.Background(), blocked); result.Skipped != "blocked" || len(rec.requests) != 0 {
		t.Fatalf("listing ran during the block: %#v", result)
	}
}

func TestOperatorReadsAcceptOnlyPostDynamics(t *testing.T) {
	video := strings.Replace(opusPage, `"comment_type":11`, `"comment_type":1`, 1)
	if _, err := ParseOpusPage([]byte(video), "900000000000000010"); ErrorCode(err) != "bilibili_unsupported_dynamic" {
		t.Fatalf("video err=%v", err)
	}
	article := strings.Replace(opusPage, `"comment_type":11`, `"comment_type":12`, 1)
	if _, err := ParseOpusPage([]byte(article), "900000000000000010"); err != nil {
		t.Fatalf("article err=%v", err)
	}
	// The listing keeps an article only in its opus form.
	data, _ := envelope([]byte(`{"code":0,"data":{"items":[
	 {"id_str":"900000000000000020","type":"DYNAMIC_TYPE_ARTICLE","modules":{"module_author":{"mid":316381099,"pub_ts":1794024288},"module_dynamic":{"major":{"type":"MAJOR_TYPE_OPUS","opus":{"title":"专栏","summary":{"rich_text_nodes":[{"type":"RICH_TEXT_NODE_TYPE_TEXT","text":"正文"}]}}}}}},
	 {"id_str":"900000000000000021","type":"DYNAMIC_TYPE_ARTICLE","modules":{"module_author":{"mid":316381099,"pub_ts":1794024288},"module_dynamic":{"major":{"type":"MAJOR_TYPE_ARTICLE"}}}}]}}`))
	page, err := ParseSpaceFeed(data, DefaultAccount)
	if err != nil || len(page.Dynamics) != 1 || page.Dynamics[0].Text != "专栏\n正文" {
		t.Fatalf("page=%#v err=%v", page, err)
	}
}
