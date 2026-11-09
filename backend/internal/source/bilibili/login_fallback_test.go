package bilibili

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// videoFeed is a synthetic feed/space page with one upload of the account
// and one of another account.
const videoFeed = `{"code":0,"message":"0","data":{"has_more":false,"offset":"","items":[
 {"id_str":"900000000000000301","type":"DYNAMIC_TYPE_AV","modules":{"module_author":{"mid":316381099,"pub_ts":1794000000},"module_dynamic":{"major":{"type":"MAJOR_TYPE_ARCHIVE","archive":{"bvid":"BV1SYNTHEABC","aid":"113000000000001","title":"翻唱《合成》","cover":"//i0.hdslb.com/bfs/archive/synthetic.jpg","desc":"synthetic description","duration_text":"03:06"}}}}},
 {"id_str":"900000000000000302","type":"DYNAMIC_TYPE_AV","modules":{"module_author":{"mid":42,"pub_ts":1794000100},"module_dynamic":{"major":{"type":"MAJOR_TYPE_ARCHIVE","archive":{"bvid":"BV1SYNTHEABD","aid":"113000000000002","title":"other","cover":"//i0.hdslb.com/bfs/archive/other.jpg","duration_text":"01:00"}}}}}
]}}`

func TestLoginCookiesKeepOnlyKnownWellFormedValues(t *testing.T) {
	login, cookies := ParseLoginCookies("SESSDATA=" + syntheticLogin + "; bili_jct=0123456789abcdef; DedeUserID=10000001; theme=dark; buvid3=bad value; sid=a\r\nX-Injected: 1")
	if login != syntheticLogin || len(cookies) != 2 || cookies["bili_jct"] != "0123456789abcdef" || cookies["DedeUserID"] != "10000001" {
		t.Fatalf("login=%q cookies=%v", login, cookies)
	}
	if login, cookies = ParseLoginCookies("bili_jct=0123456789abcdef"); login != "" || len(cookies) != 1 {
		t.Fatalf("without SESSDATA login=%q cookies=%v", login, cookies)
	}
}

func TestSignedInRequestsCarryTheBrowsersIdentity(t *testing.T) {
	account := LoginCookies{"buvid3": "FFFFFFFF-0000-0000-0000-ACCOUNTDEVICEinfoc", "_uuid": "SYNTHETIC-UUID-infoc", "DedeUserID": "10000001", "bili_jct": "0123456789abcdef"}
	s := Session{Buvid3: "0A1B2C3D-0A1B-0A1B-0A1B-0A1B2C3D4E5F12345infoc", Buvid4: "anonymous-buvid4==", Nut: 1767225600, Login: syntheticLogin, Account: &account}
	signed := s.cookie(true, apiHost)
	want := "buvid3=FFFFFFFF-0000-0000-0000-ACCOUNTDEVICEinfoc; buvid4=anonymous-buvid4==; b_nut=1767225600; _uuid=SYNTHETIC-UUID-infoc; DedeUserID=10000001; bili_jct=0123456789abcdef; SESSDATA=" + syntheticLogin
	if signed != want {
		t.Fatalf("signed cookie=%q", signed)
	}
	// Without the login, and on another host, only the anonymous identity.
	for _, cookie := range []string{s.cookie(false, apiHost), s.cookie(true, wwwHost)} {
		if cookie != "buvid3=0A1B2C3D-0A1B-0A1B-0A1B-0A1B2C3D4E5F12345infoc; buvid4=anonymous-buvid4==; b_nut=1767225600" {
			t.Fatalf("anonymous cookie=%q", cookie)
		}
	}
}

func TestVideoDynamicsAreReadAsUploads(t *testing.T) {
	page, err := ParseSpaceFeed(mustEnvelope(t, videoFeed), DefaultAccount)
	if err != nil || len(page.Dynamics) != 0 || len(page.Uploads) != 1 {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	v := page.Uploads[0]
	if v.BVID != "BV1SYNTHEABC" || v.AID != 113000000000001 || v.Title != "翻唱《合成》" || v.DurationSeconds != 186 || v.Kind != KindUpload ||
		v.ThumbnailURL != "https://i0.hdslb.com/bfs/archive/synthetic.jpg" || v.Description != "synthetic description" ||
		v.URL != "https://www.bilibili.com/video/BV1SYNTHEABC" || v.PublishedAt != time.Unix(1794000000, 0).UTC().Format(time.RFC3339) {
		t.Fatalf("upload=%#v", v)
	}
}

func mustEnvelope(t *testing.T, body string) []byte {
	t.Helper()
	data, err := envelope([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// syntheticAppKeys is a well-formed key pair that no client uses.
var syntheticAppKeys = AppKeys{"0123456789abcdef", "fedcba9876543210fedcba9876543210"}

// appListing is a synthetic page of the official client's space listing: an
// upload, a non-upload card and an incomplete item.
const appListing = `{"code":0,"message":"0","ttl":1,"data":{"count":3,"has_next":false,"item":[
 {"goto":"av","param":"113000000000001","bvid":"BV1SYNTHEABC","title":"翻唱《合成》","cover":"http://i0.hdslb.com/bfs/archive/synthetic.jpg","duration":186,"ctime":1794000000},
 {"goto":"article","param":"42","title":"not an upload","ctime":1794000000},
 {"goto":"av","param":"113000000000003","bvid":"BV1SYNTHEABE","title":"","ctime":1794000000}
]}}`

func TestUploadListFallsBackToTheDynamicsThenTheAppListing(t *testing.T) {
	now := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	type call struct{ host, path, cookie, referer, query string }
	var calls []call
	status := map[string]int{"/x/series/recArchivesByKeywords": 412}
	reply := map[string]string{"/x/frontend/finger/spi": session, ticketPath: ticket, "/x/web-interface/nav": navKeys,
		"/x/polymer/web-dynamic/v1/feed/space": videoFeed, "/x/v2/space/archive/cursor": appListing}
	adapter := func(login string, app AppKeys) *Adapter {
		c := NewClient(&http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
			calls = append(calls, call{r.URL.Host, r.URL.Path, r.Header.Get("Cookie"), r.Header.Get("Referer"), r.URL.RawQuery})
			if code := status[r.URL.Path]; code != 0 {
				return response(code, "<!DOCTYPE html><title>412</title>"), nil
			}
			return response(200, reply[r.URL.Path]), nil
		})})
		a := NewAdapter(c, Options{Account: DefaultAccount, MinInterval: 3 * time.Hour, PageSize: 20, SESSDATA: login, Cookies: LoginCookies{"DedeUserID": "10000001", "bili_jct": "0123456789abcdef"}, App: app})
		a.Now = func() time.Time { return now }
		return a
	}
	paths := func() string {
		out := []string{}
		for _, c := range calls {
			out = append(out, c.path)
		}
		return strings.Join(out, " ")
	}

	// Signed in: the list's captcha falls back to the dynamics listing.
	result, err := adapter(syntheticLogin, syntheticAppKeys).FetchRecent(context.Background(), State{})
	if err != nil || result.Fallback != "dynamics" || result.ListError != "bilibili_risk_control_http" || len(result.Videos) != 1 ||
		result.Videos[0].BVID != "BV1SYNTHEABC" || result.Requested != 5 || !result.State.BlockedUntil.IsZero() || result.State.Failures != 0 ||
		!result.State.LastSuccessAt.Equal(now) || !result.State.Dynamics.LastAttemptAt.IsZero() || len(result.FallbackErrors) != 0 {
		t.Fatalf("dynamics fallback=%#v err=%v", result, err)
	}
	if paths() != "/x/frontend/finger/spi "+ticketPath+" /x/series/recArchivesByKeywords /x/web-interface/nav /x/polymer/web-dynamic/v1/feed/space" {
		t.Fatalf("paths=%s", paths())
	}
	for _, c := range calls[2:] {
		if !strings.Contains(c.cookie, "DedeUserID=10000001; bili_jct=0123456789abcdef; SESSDATA="+syntheticLogin) {
			t.Fatalf("%s cookie=%q", c.path, c.cookie)
		}
	}

	// Without a login or an app key pair there is no fallback.
	calls = nil
	bare, err := adapter("", AppKeys{}).FetchRecent(context.Background(), State{})
	if ErrorCode(err) != "bilibili_risk_control_http" || bare.Fallback != "" || len(bare.FallbackErrors) != 0 || len(calls) != 3 || bare.State.Failures != 1 {
		t.Fatalf("bare=%#v calls=%v err=%v", bare, calls, err)
	}

	// Without the login the app listing follows the list, unsigned in and
	// without cookies or Referer, signed with the configured key pair.
	calls = nil
	anonymous, err := adapter("", syntheticAppKeys).FetchRecent(context.Background(), State{})
	if err != nil || anonymous.Fallback != "app" || len(anonymous.Videos) != 1 || anonymous.Videos[0].DurationSeconds != 186 || anonymous.Videos[0].AID != 113000000000001 ||
		anonymous.Videos[0].ThumbnailURL != "https://i0.hdslb.com/bfs/archive/synthetic.jpg" || anonymous.Requested != 4 || anonymous.State.Failures != 0 || !anonymous.State.BlockedUntil.IsZero() {
		t.Fatalf("app fallback=%#v err=%v", anonymous, err)
	}
	app := calls[len(calls)-1]
	query, _ := url.ParseQuery(app.query)
	if app.host != "app.bilibili.com" || app.path != "/x/v2/space/archive/cursor" || app.cookie != "" || app.referer != "" ||
		query.Get("vmid") != DefaultAccount || query.Get("ps") != "20" || query.Get("order") != "pubdate" ||
		query.Get("appkey") != syntheticAppKeys.Key || query.Get("ts") != strconv.FormatInt(now.Unix(), 10) {
		t.Fatalf("app request=%#v", app)
	}
	unsigned := strings.TrimSuffix(app.query, "&sign="+query.Get("sign"))
	if sum := md5.Sum([]byte(unsigned + syntheticAppKeys.Secret)); hex.EncodeToString(sum[:]) != query.Get("sign") {
		t.Fatalf("sign=%s over %s", query.Get("sign"), unsigned)
	}

	// Every listing failing counts the block once and names each failure.
	reply["/x/polymer/web-dynamic/v1/feed/space"] = `{"code":0,"data":{"v_voucher":"voucher_synthetic"}}`
	status["/x/v2/space/archive/cursor"] = 412
	all, err := adapter(syntheticLogin, syntheticAppKeys).FetchRecent(context.Background(), State{})
	if ErrorCode(err) != "bilibili_risk_control_http" || all.State.Failures != 1 || !all.State.BlockedUntil.Equal(now.Add(6*time.Hour)) || all.State.WBI.valid() ||
		strings.Join(all.FallbackErrors, ",") != "dynamics: bilibili_risk_control_voucher,app: bilibili_risk_control_http" || len(all.Videos) != 0 {
		t.Fatalf("all=%#v err=%v", all, err)
	}

	// A list failing without a captcha leaves the backoff to the fallback's.
	status["/x/series/recArchivesByKeywords"] = 503
	other, err := adapter("", syntheticAppKeys).FetchRecent(context.Background(), State{})
	if ErrorCode(err) != "bilibili_upstream_5xx" || other.State.Failures != 1 || strings.Join(other.FallbackErrors, ",") != "app: bilibili_risk_control_http" {
		t.Fatalf("other=%#v err=%v", other, err)
	}
}

func TestAppListingKeepsOnlyCompleteUploads(t *testing.T) {
	videos, err := ParseAppUploads(mustEnvelope(t, appListing), DefaultAccount)
	if err != nil || len(videos) != 1 || videos[0].BVID != "BV1SYNTHEABC" || videos[0].Description != "" ||
		videos[0].PublishedAt != time.Unix(1794000000, 0).UTC().Format(time.RFC3339) {
		t.Fatalf("videos=%#v err=%v", videos, err)
	}
	if empty, err := ParseAppUploads(mustEnvelope(t, `{"code":0,"data":{"count":0,"has_next":false}}`), DefaultAccount); err != nil || len(empty) != 0 {
		t.Fatalf("empty=%#v err=%v", empty, err)
	}
}

func TestAppKeysComeOnlyFromAWellFormedPair(t *testing.T) {
	t.Setenv("BILIBILI_APP_KEY", syntheticAppKeys.Key)
	t.Setenv("BILIBILI_APP_SECRET", syntheticAppKeys.Secret)
	if got := OptionsFromEnv().App; got != syntheticAppKeys {
		t.Fatalf("app=%#v", got)
	}
	t.Setenv("BILIBILI_APP_SECRET", "not-hex")
	if got := OptionsFromEnv().App; got.Valid() || got.Key != "" {
		t.Fatalf("malformed app=%#v", got)
	}
	if _, err := NewClient(nil).AppUploads(context.Background(), DefaultAccount, 20, AppKeys{}, time.Now()); ErrorCode(err) != "bilibili_app_keys_missing" {
		t.Fatalf("err=%v", err)
	}
}
