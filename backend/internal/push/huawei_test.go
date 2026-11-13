package push

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

// testAccount mints a throwaway RSA key so the suite never needs a real
// private.json, which must not live in the repository.
func testAccount(t *testing.T) (ServiceAccount, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	account := ServiceAccount{
		ProjectID:  "project",
		KeyID:      "key-id",
		SubAccount: "sub-account",
		PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	}
	return account, key
}

func TestNewStaysDisabledWithoutAccount(t *testing.T) {
	client, err := New(Options{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if client != nil {
		t.Fatal("client should be nil when push is not configured")
	}
	if client.Enabled() {
		t.Fatal("a nil client must report disabled")
	}
	if _, err = client.Send(t.Context(), Notification{}, []string{"token"}); err != ErrDisabled {
		t.Fatalf("send err = %v", err)
	}
}

func TestAssertionIsVerifiablePS256(t *testing.T) {
	account, key := testAccount(t)
	client, err := New(Options{Account: account})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	now := time.Unix(1700000000, 0)
	assertion, err := client.assertion(now)
	if err != nil {
		t.Fatalf("assertion: %v", err)
	}
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		t.Fatalf("assertion has %d segments", len(parts))
	}

	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("header: %v", err)
	}
	var head map[string]string
	if err = json.Unmarshal(header, &head); err != nil {
		t.Fatalf("header json: %v", err)
	}
	if head["alg"] != "PS256" || head["kid"] != account.KeyID || head["typ"] != "JWT" {
		t.Fatalf("header = %v", head)
	}

	claims, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("claims: %v", err)
	}
	var payload map[string]any
	if err = json.Unmarshal(claims, &payload); err != nil {
		t.Fatalf("claims json: %v", err)
	}
	if payload["iss"] != account.SubAccount || payload["aud"] != DefaultAudienceURI {
		t.Fatalf("payload = %v", payload)
	}
	if payload["exp"].(float64)-payload["iat"].(float64) != 3600 {
		t.Fatalf("lifetime = %v", payload["exp"].(float64)-payload["iat"].(float64))
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("signature: %v", err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err = rsa.VerifyPSS(&key.PublicKey, crypto.SHA256, digest[:], signature,
		&rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthAuto, Hash: crypto.SHA256}); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestWordingPerLocale(t *testing.T) {
	update := Update{Platform: "youtube", Kind: "live_start", Author: "鹿乃", Title: "雑談"}

	// 标题要同时说清谁、在哪个平台、做了什么。
	cases := map[string]string{
		"zh-Hans": "鹿乃 在 YT 的直播开始啦~",
		"zh-Hant": "鹿乃 在 YT 的直播開始啦~",
		"ja-JP":   "鹿乃がYouTubeでライブ配信を始めました",
		"en-US":   "鹿乃 went live on YouTube",
	}
	for locale, want := range cases {
		title, body := wording(update, normalizeLocale(locale))
		if title != want {
			t.Errorf("%s title = %q, want %q", locale, title, want)
		}
		if body != "雑談" {
			t.Errorf("%s body = %q", locale, body)
		}
	}
}

func TestPlatformLabel(t *testing.T) {
	cases := []struct {
		platform string
		locale   string
		want     string
	}{
		{"twitter", "zh", "X"},
		{"youtube", "zh", "YT"},
		{"bilibili", "zh", "B站"},
		{"bilibili", "zh-Hant", "B站"},
		{"youtube", "ja", "YouTube"},
		{"bilibili", "en", "Bilibili"},
		// 未知平台原样显示，不吞掉信息。
		{"weibo", "zh", "weibo"},
	}
	for _, item := range cases {
		if got := platformLabel(item.platform, item.locale); got != item.want {
			t.Errorf("platformLabel(%q, %q) = %q, want %q", item.platform, item.locale, got, item.want)
		}
	}
}

func TestWordingFallsBackToPostText(t *testing.T) {
	update := Update{Kind: "post", Author: "Kano", Text: "  第一行\n\n第二行  "}
	title, body := wording(update, "zh")
	// 没有平台时不留空档。
	if title != "Kano 的动态更新啦~" {
		t.Fatalf("title = %q", title)
	}
	// 多行正文压成一行，通知栏只吃两行。
	if body != "第一行 第二行" {
		t.Fatalf("body = %q", body)
	}
}

func TestCategoryAndRoute(t *testing.T) {
	cases := []struct {
		platform string
		kind     string
		category Category
		route    string
	}{
		{"youtube", "live_start", CategoryLive, "recent"},
		{"bilibili", "live_update", CategoryLive, "recent"},
		{"youtube", "video", CategoryVideo, "feed:youtube"},
		{"youtube", "shorts", CategoryVideo, "feed:youtube"},
		{"bilibili", "video", CategoryVideo, "feed:bilibili"},
		{"twitter", "post", CategoryPost, "feed:x"},
		{"twitter", "quote", CategoryPost, "feed:x"},
	}
	for _, item := range cases {
		update := Update{Platform: item.platform, Kind: item.kind}
		if got := update.Category(); got != item.category {
			t.Errorf("%s/%s category = %q, want %q", item.platform, item.kind, got, item.category)
		}
		if got := routeFor(update); got != item.route {
			t.Errorf("%s/%s route = %q, want %q", item.platform, item.kind, got, item.route)
		}
	}
}

func TestDedupeKeepsNewestPerAuthorAndCaps(t *testing.T) {
	updates := []Update{
		{Platform: "x", Kind: "post", Author: "A"},
		{Platform: "x", Kind: "post", Author: "A"},
		{Platform: "x", Kind: "video", Author: "A"},
		{Platform: "x", Kind: "post", Author: "B"},
	}
	kept := dedupeByAuthor(updates, 8)
	if len(kept) != 3 {
		t.Fatalf("kept %d, want 3 (A/post, A/video, B/post)", len(kept))
	}
	if capped := dedupeByAuthor(updates, 2); len(capped) != 2 {
		t.Fatalf("capped %d, want 2", len(capped))
	}
}

func TestAcceptsHonoursSwitches(t *testing.T) {
	device := sqlite.PushDevice{Enabled: true, NotifyLive: true, NotifyVideo: true, NotifyPost: true}
	if !accepts(device, CategoryLive) {
		t.Fatal("live should pass while notify_live is on")
	}
	device.NotifyVideo = false
	if accepts(device, CategoryVideo) {
		t.Fatal("video should be filtered")
	}
	device.Enabled = false
	if accepts(device, CategoryLive) {
		t.Fatal("master switch must win")
	}
}

// Push v3 takes the signed assertion itself as the bearer credential; trading
// it for an OAuth access token first makes the API answer 80200001. This pins
// the single-request shape and the cached reuse across sends.
func TestSendUsesTheAssertionAsBearer(t *testing.T) {
	account, _ := testAccount(t)
	var seen []*http.Request
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		seen = append(seen, request)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"code":"80000000"}`)),
			Request:    request,
		}, nil
	})
	client, err := New(Options{Account: account, Client: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err = client.Send(context.Background(), Notification{Title: "t", Body: "b"}, []string{"token-a"}); err != nil {
			t.Fatalf("send %d: %v", attempt, err)
		}
	}
	if len(seen) != 2 {
		t.Fatalf("requests = %d, want one per send and no token exchange", len(seen))
	}
	if got := seen[0].URL.String(); got != ProjectBase+"project/messages:send" {
		t.Fatalf("url = %s", got)
	}
	assertion := strings.TrimPrefix(seen[0].Header.Get("Authorization"), "Bearer ")
	header, err := base64.RawURLEncoding.DecodeString(strings.Split(assertion, ".")[0])
	if err != nil {
		t.Fatalf("bearer header: %v", err)
	}
	var head map[string]string
	if err = json.Unmarshal(header, &head); err != nil {
		t.Fatalf("bearer header json: %v", err)
	}
	if head["alg"] != "PS256" || head["kid"] != account.KeyID {
		t.Fatalf("bearer header = %v", head)
	}
	if seen[0].Header.Get("push-type") != "0" {
		t.Fatalf("push-type = %q", seen[0].Header.Get("push-type"))
	}
}
