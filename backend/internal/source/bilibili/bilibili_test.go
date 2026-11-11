package bilibili

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type fakeTransport func(*http.Request) (*http.Response, error)

func (f fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

// ticketPath is the bili_ticket request, which the fake transports answer
// with a synthetic ticket.
const ticketPath = "/bapis/bilibili.api.ticket.v1.Ticket/GenWebTicket"

const ticket = `{"code":0,"message":"OK","data":{"ticket":"eyJhbGciOiJIUzI1NiJ9.eyJzeW50aGV0aWMiOnRydWV9.c3ludGhldGljLXNpZ25hdHVyZQ","created_at":1767225600,"ttl":259200,"context":{},"nav":{"img":"https://i0.hdslb.com/bfs/wbi/0123456789abcdef0123456789abcdef.png","sub":"https://i0.hdslb.com/bfs/wbi/fedcba9876543210fedcba9876543210.png"}}}`

const session = `{"code":0,"message":"ok","data":{"b_3":"0A1B2C3D-0A1B-0A1B-0A1B-0A1B2C3D4E5F12345infoc","b_4":"0A1B2C3D-0A1B-0A1B-0A1B-0A1B2C3D4E5F12345-026010100-syntheticBuvid=="}}`

const uploads = `{"code":0,"message":"0","data":{"archives":[
 {"aid":1,"bvid":"BV1AAAAAAAAA","title":"鹿乃 翻唱《奏》 &amp; more","pubdate":1767258000,"pic":"http://i0.hdslb.com/bfs/archive/synthetic.jpg","duration":289,"desc":"synthetic","upMid":316381099},
 {"aid":2,"bvid":"BV1BBBBBBBBB","title":"collaboration","pubdate":1767258000,"pic":"https://i1.hdslb.com/bfs/archive/other.jpg","duration":60,"upMid":42},
 {"aid":3,"bvid":"not-a-bvid","title":"broken","pubdate":1767258000},
 {"aid":4,"bvid":"BV1CCCCCCCCC","title":"杂谈直播回放","pubdate":1767171600,"pic":"https://cover.invalid/cover.jpg","duration":3600,"upMid":316381099}
],"page":{"num":1,"size":20,"total":3}}}`

func TestEnvelopeReportsCaptchaRateLimitsAndMissingVideos(t *testing.T) {
	for body, code := range map[string]string{
		"<!DOCTYPE html><html><title>出错啦!</title></html>": "bilibili_risk_control_page",
		`{"code":-352,"message":"-352"}`:                  "bilibili_risk_control_352",
		`{"code":-412,"message":"请求被拦截"}`:                 "bilibili_risk_control_412",
		`{"code":-799,"message":"请求过于频繁"}`:                "bilibili_rate_limited",
		`{"code":62002,"message":"稿件不可见"}`:                "bilibili_not_found",
		`{"code":-400,"message":"bad request"}`:           "bilibili_api_error",
		`{"message":"no code"}`:                           "bilibili_invalid_response",
	} {
		if _, err := envelope([]byte(body)); ErrorCode(err) != code {
			t.Fatalf("%s: err=%v, want %s", body, err, code)
		}
	}
}

func TestClientMapsStatusesAndLimitsResponses(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		code   string
	}{
		{412, "<!DOCTYPE html>", "bilibili_risk_control_http"},
		{429, "", "bilibili_rate_limited"},
		{302, "", "bilibili_redirect_rejected"},
		{503, "", "bilibili_upstream_5xx"},
		{404, "", "bilibili_http_error"},
		{200, strings.Repeat("x", MaxResponseBytes+1), "bilibili_response_oversized"},
	} {
		c := NewClient(&http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "api.bilibili.com" || r.URL.Scheme != "https" {
				t.Fatalf("unexpected destination %s", r.URL)
			}
			return response(tc.status, tc.body), nil
		})})
		if _, err := c.NewSession(context.Background()); ErrorCode(err) != tc.code {
			t.Fatalf("status %d: err=%v, want %s", tc.status, err, tc.code)
		}
	}
}

func TestUploadsKeepOnlyCompleteOwnUploads(t *testing.T) {
	data, err := envelope([]byte(uploads))
	if err != nil {
		t.Fatal(err)
	}
	videos, err := ParseUploads(data, DefaultAccount)
	if err != nil || len(videos) != 2 {
		t.Fatalf("videos=%#v err=%v", videos, err)
	}
	cover, stream := videos[0], videos[1]
	if cover.BVID != "BV1AAAAAAAAA" || cover.Title != "鹿乃 翻唱《奏》 & more" || cover.Kind != KindUpload ||
		cover.ThumbnailURL != "https://i0.hdslb.com/bfs/archive/synthetic.jpg" || cover.PublishedAt != "2026-01-01T09:00:00Z" ||
		cover.DurationSeconds != 289 || cover.URL != "https://www.bilibili.com/video/BV1AAAAAAAAA" || cover.Description != "synthetic" {
		t.Fatalf("cover=%#v", cover)
	}
	// A cover outside Bilibili's image CDN is not kept.
	if stream.Kind != KindUpload || stream.ThumbnailURL != "" {
		t.Fatalf("stream=%#v", stream)
	}
}

func TestViewReadsOwnerAndTextDuration(t *testing.T) {
	data, err := envelope([]byte(`{"code":0,"data":{"bvid":"BV1AAAAAAAAA","aid":1,"title":"song","pubdate":1767258000,"pic":"//i2.hdslb.com/bfs/archive/x.png","duration":"04:49","owner":{"mid":316381099}}}`))
	if err != nil {
		t.Fatal(err)
	}
	v, err := ParseView(data, "BV1AAAAAAAAA")
	if err != nil || v.OwnerMid != 316381099 || v.DurationSeconds != 289 || v.ThumbnailURL != "https://i2.hdslb.com/bfs/archive/x.png" {
		t.Fatalf("video=%#v err=%v", v, err)
	}
	if _, err = ParseView(data, "BV1BBBBBBBBB"); err == nil {
		t.Fatal("accepted another video's answer")
	}
}

func TestParseBVIDAcceptsIDsAndVideoPages(t *testing.T) {
	for input, want := range map[string]string{
		"BV1CmHw6eEGE": "BV1CmHw6eEGE",
		" https://www.bilibili.com/video/BV1CmHw6eEGE/?spm_id_from=333 ": "BV1CmHw6eEGE",
		"https://m.bilibili.com/video/BV1CmHw6eEGE":                      "BV1CmHw6eEGE",
		"https://short-link.invalid/abcdef":                              "",
		"https://bilibili.com.invalid/video/BV1CmHw6eEGE":                "",
		"BV1CmHw6eEG0":      "",
		"av117493897105505": "",
	} {
		got, ok := ParseBVID(input)
		if got != want || ok != (want != "") {
			t.Fatalf("%q => %q %v, want %q", input, got, ok, want)
		}
	}
}

type script struct {
	paths []string
	reply func(path string) *http.Response
}

func (s *script) adapter(now time.Time) *Adapter {
	c := NewClient(&http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == ticketPath {
			return response(200, ticket), nil
		}
		s.paths = append(s.paths, r.URL.Path)
		return s.reply(r.URL.Path), nil
	})})
	a := NewAdapter(c, Options{Account: DefaultAccount, MinInterval: 3 * time.Hour, PageSize: 20})
	a.Now = func() time.Time { return now }
	return a
}

func TestRecentReadsRespectTheIntervalAndBackOffAfterACaptcha(t *testing.T) {
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	blocked := true
	s := &script{reply: func(path string) *http.Response {
		if path == "/x/frontend/finger/spi" {
			return response(200, session)
		}
		if blocked {
			return response(412, "<!DOCTYPE html>")
		}
		return response(200, uploads)
	}}
	result, err := s.adapter(now).FetchRecent(context.Background(), State{})
	if ErrorCode(err) != "bilibili_risk_control_http" || result.Requested != 3 || !result.State.BlockedUntil.Equal(now.Add(6*time.Hour)) || result.State.Failures != 1 || !result.State.Session.valid() {
		t.Fatalf("first=%#v err=%v", result, err)
	}
	// While blocked, and within the interval, nothing is requested.
	s.paths = nil
	later, err := s.adapter(now.Add(5*time.Hour)).FetchRecent(context.Background(), result.State)
	if err != nil || later.Skipped != "blocked" || len(s.paths) != 0 {
		t.Fatalf("blocked=%#v paths=%v err=%v", later, s.paths, err)
	}
	// A second captcha doubles the wait; the stored session is reused.
	again, err := s.adapter(now.Add(7*time.Hour)).FetchRecent(context.Background(), result.State)
	if ErrorCode(err) != "bilibili_risk_control_http" || strings.Join(s.paths, ",") != "/x/series/recArchivesByKeywords" || !again.State.BlockedUntil.Equal(now.Add(19*time.Hour)) {
		t.Fatalf("again=%#v paths=%v err=%v", again, s.paths, err)
	}
	blocked = false
	done, err := s.adapter(now.Add(20*time.Hour)).FetchRecent(context.Background(), again.State)
	if err != nil || len(done.Videos) != 2 || done.State.Failures != 0 || !done.State.BlockedUntil.IsZero() || !done.State.LastSuccessAt.Equal(now.Add(20*time.Hour)) {
		t.Fatalf("done=%#v err=%v", done, err)
	}
	s.paths = nil
	if soon, _ := s.adapter(now.Add(22*time.Hour)).FetchRecent(context.Background(), done.State); soon.Skipped != "interval" || len(s.paths) != 0 {
		t.Fatalf("soon=%#v paths=%v", soon, s.paths)
	}
}

func TestOperatorReadsOnlyAcceptTheConfiguredAccount(t *testing.T) {
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	owner := "316381099"
	s := &script{reply: func(path string) *http.Response {
		if path == "/x/frontend/finger/spi" {
			return response(200, session)
		}
		return response(200, `{"code":0,"data":{"bvid":"BV1AAAAAAAAA","aid":1,"title":"song","pubdate":1767258000,"duration":60,"owner":{"mid":`+owner+`}}}`)
	}}
	v, state, err := s.adapter(now).FetchVideo(context.Background(), State{}, "BV1AAAAAAAAA", false)
	if err != nil || v.BVID != "BV1AAAAAAAAA" || !state.LastSuccessAt.Equal(now) {
		t.Fatalf("video=%#v state=%#v err=%v", v, state, err)
	}
	owner = "42"
	if _, _, err = s.adapter(now).FetchVideo(context.Background(), state, "BV1AAAAAAAAA", false); ErrorCode(err) != "bilibili_other_account" {
		t.Fatalf("other account err=%v", err)
	}
	s.paths = nil
	if _, _, err = s.adapter(now).FetchVideo(context.Background(), State{BlockedUntil: now.Add(time.Hour)}, "BV1AAAAAAAAA", false); ErrorCode(err) != "bilibili_blocked" || len(s.paths) != 0 {
		t.Fatalf("blocked err=%v paths=%v", err, s.paths)
	}
}

func TestLoginCookieGoesOnlyToApiReadsWhenValid(t *testing.T) {
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	login := "synthetic0login%2C1767225600%2Cabcde%2A11"
	for _, tc := range []struct {
		name, cookie string
		signedIn     bool
	}{
		{"bare session", login, true},
		// The feed refuses a login that carries SESSDATA alone, so the whole
		// cookie set a browser holds is kept and forwarded.
		{"whole cookie", "SESSDATA=" + login + "; bili_jct=synthetic; DedeUserID=316381099", true},
		// A line break would end the cookie header and start another one.
		{"header injection", login + "\r\nX-Injected: 1", false},
		{"absent", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cookies := map[string]string{}
			s := &script{reply: func(path string) *http.Response {
				if path == "/x/frontend/finger/spi" {
					return response(200, session)
				}
				return response(200, uploads)
			}}
			a := s.adapter(now)
			a.Client = NewClient(&http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
				cookies[r.URL.Path] = r.Header.Get("Cookie")
				return s.reply(r.URL.Path), nil
			})})
			if tc.name == "whole cookie" {
				loginValue, extra := ParseLoginCookies(tc.cookie)
				a.Options.SESSDATA, a.Options.Cookies = loginValue, extra
			} else {
				a.Options.SESSDATA = tc.cookie
			}
			if a.Options.SignedIn() != tc.signedIn {
				t.Fatalf("signed in=%v", a.Options.SignedIn())
			}
			result, err := a.FetchRecent(context.Background(), State{})
			if err != nil {
				t.Fatal(err)
			}
			if cookies["/x/frontend/finger/spi"] != "" {
				t.Fatalf("device request carried cookies %q", cookies["/x/frontend/finger/spi"])
			}
			list := cookies["/x/series/recArchivesByKeywords"]
			if strings.Contains(list, "SESSDATA="+login) != tc.signedIn || strings.Contains(list, "X-Injected") {
				t.Fatalf("list cookie=%q", list)
			}
			if tc.name == "whole cookie" && !strings.Contains(list, "bili_jct=synthetic") {
				t.Fatalf("list cookie dropped the rest of the set: %q", list)
			}
			// The login is never part of the state that is persisted.
			if result.State.Session.Login != "" {
				t.Fatalf("state kept the login: %#v", result.State.Session)
			}
		})
	}
}
