package bilibili

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// dynamicsPage is one space feed page: an opus with two pictures, an upload
// published as a video dynamic, a forward of an opus, an entry of another
// account, a repeated ID and an empty entry.
const dynamicsPage = `{"items":[
 {"id_str":"1100000000000000001","type":"DYNAMIC_TYPE_DRAW","modules":{
   "module_author":{"pub_ts":1767258000,"name":"鹿乃","mid":316381099},
   "module_dynamic":{"desc":{"text":"今天的直播 &amp; 歌单"},"major":{"opus":{
     "title":null,"summary":{"text":"","rich_text_nodes":[
       {"type":"RICH_TEXT_NODE_TYPE_TEXT","text":"今週のスケジュールです"},
       {"type":"RICH_TEXT_NODE_TYPE_EMOJI","text":"[鹿乃_飞了]","emoji":{"type":"3","text":"[鹿乃_飞了]","icon_url":"http://i0.hdslb.com/bfs/emote/flying.png"}}]},
     "pics":[{"url":"http://i0.hdslb.com/bfs/new_dyn/one.jpg"},{"url":"https://i1.hdslb.com/bfs/new_dyn/two.jpg"}]}}}}},
 {"id_str":"1100000000000000002","type":"DYNAMIC_TYPE_AV","modules":{
   "module_author":{"pub_ts":1767258000,"name":"鹿乃","mid":316381099},
   "module_dynamic":{"major":{"archive":{"bvid":"BV1AAAAAAAAA","title":"翻唱"}}}}},
 {"id_str":"1100000000000000003","type":"DYNAMIC_TYPE_FORWARD","modules":{
   "module_author":{"pub_ts":1767258000,"name":"鹿乃","mid":316381099},
   "module_dynamic":{"desc":{"text":"转 发 &amp; 提醒"}}},
  "orig":{"id_str":"1100000000000000009","type":"DYNAMIC_TYPE_DRAW","modules":{
   "module_author":{"pub_ts":1767171600,"name":"别的 UP","mid":42},
   "module_dynamic":{"desc":{"text":"原动态正文"},"major":{"draw":{"items":[{"src":"https://i2.hdslb.com/bfs/album/orig.jpg"}]}}}}}},
 {"id_str":"1100000000000000004","type":"DYNAMIC_TYPE_WORD","modules":{
   "module_author":{"pub_ts":1767258000,"name":"别的 UP","mid":42},
   "module_dynamic":{"desc":{"text":"别人的动态"}}}},
 {"id_str":"1100000000000000001","type":"DYNAMIC_TYPE_WORD","modules":{
   "module_author":{"pub_ts":1767258000,"name":"鹿乃","mid":316381099},
   "module_dynamic":{"desc":{"text":"重复"}}}},
 {"id_str":"1100000000000000005","type":"DYNAMIC_TYPE_WORD","modules":{
   "module_author":{"pub_ts":1767258000,"name":"鹿乃","mid":316381099},
   "module_dynamic":{"desc":{"text":null}}}}
],"offset":"1100000000000000005","has_more":true}`

func TestParseDynamicsKeepsPostsRepostsAndPictures(t *testing.T) {
	items, err := ParseDynamics([]byte(dynamicsPage), "316381099")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items=%d, want 2: %#v", len(items), items)
	}
	post := items[0]
	if post.ID != "1100000000000000001" || post.Kind != KindDynamicPost || post.URL != "https://t.bilibili.com/1100000000000000001" {
		t.Fatalf("post=%#v", post)
	}
	// The opus summary wins over the desc, its rich-text nodes rebuild the body
	// (the flattened text beside them is empty on most entries), the album
	// keeps its order on HTTPS only, and the sticker keeps its caption and icon.
	if post.Text != "今週のスケジュールです[鹿乃_飞了]" || len(post.Images) != 2 || post.Images[0] != "https://i0.hdslb.com/bfs/new_dyn/one.jpg" {
		t.Fatalf("text=%q images=%#v", post.Text, post.Images)
	}
	if len(post.Emojis) != 1 || post.Emojis[0].Text != "[鹿乃_飞了]" || post.Emojis[0].Icon != "https://i0.hdslb.com/bfs/emote/flying.png" {
		t.Fatalf("emoji=%#v", post.Emojis)
	}
	if post.Author != "鹿乃" || post.AuthorMid != 316381099 || post.PublishedAt != time.Unix(1767258000, 0).UTC().Format(time.RFC3339) {
		t.Fatalf("author=%q mid=%d at=%q", post.Author, post.AuthorMid, post.PublishedAt)
	}
	repost := items[1]
	if repost.Kind != KindDynamicRepost || repost.Text != "转 发 & 提醒" || repost.Original == nil {
		t.Fatalf("repost=%#v", repost)
	}
	if repost.Original.ID != "1100000000000000009" || repost.Original.Text != "原动态正文" || len(repost.Original.Images) != 1 {
		t.Fatalf("original=%#v", repost.Original)
	}
}

func TestParseDynamicsRejectsMalformedAnswers(t *testing.T) {
	for _, body := range []string{`{"code":0,"data":{}}`, `{"code":0}`, `not json`} {
		if _, err := ParseDynamics([]byte(body), "316381099"); ErrorCode(err) != "bilibili_invalid_response" {
			t.Fatalf("%s: err=%v", body, err)
		}
	}
}

func TestSignsSortedParametersWithATimestamp(t *testing.T) {
	at := time.Unix(1767258000, 0)
	query := map[string][]string{"host_mid": {"316381099"}, "features": {"itemOpusStyle,opusBigCover"}}
	signed := signWBI(query, "0123456789abcdef0123456789abcdef", at)
	if !strings.HasPrefix(signed, "features=itemOpusStyle%2CopusBigCover&host_mid=316381099&wts=1767258000&w_rid=") {
		t.Fatalf("signed=%s", signed)
	}
	if len(signed) != len("features=itemOpusStyle%2CopusBigCover&host_mid=316381099&wts=1767258000&w_rid=")+32 {
		t.Fatalf("signed=%s", signed)
	}
	// A different key signs differently, and the signing characters are gone.
	stripped := signWBI(params("a!(b)*c"), "0123456789abcdef0123456789abcdef", at)
	if !strings.HasPrefix(stripped, "a=abc&wts=") {
		t.Fatalf("stripped=%s", stripped)
	}
	if signWBI(params("a!(b)*c"), "fedcba9876543210fedcba9876543210", at) == stripped {
		t.Fatal("signature ignores the key")
	}
}

func params(value string) map[string][]string { return map[string][]string{"a": {value}} }

func TestDerivesTheSigningKeyFromTheImageNames(t *testing.T) {
	raw := "7cd084941338484aae1ad9425b84077c" + "4932caff0ff746eab6f01bf08b70ac45"
	if key := deriveMixinKey(raw); len(key) != 32 || key == raw[:32] {
		t.Fatalf("key=%q", key)
	}
	if deriveMixinKey("short") != "" {
		t.Fatal("a short key was accepted")
	}
	if imageStem("https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png") != "7cd084941338484aae1ad9425b84077c" {
		t.Fatal("image stem")
	}
}

func TestDynamicsNeedsTheOperatorsLogin(t *testing.T) {
	requests := 0
	c := NewClient(&http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		return response(200, session), nil
	})})
	s := Session{Buvid3: "0A1B2C3D-0A1B-0A1B-0A1B-0A1B2C3D4E5F12345infoc", Buvid4: "synthetic-buvid4=="}
	if _, err := c.Dynamics(context.Background(), "316381099", 20, s); ErrorCode(err) != "bilibili_login_required" {
		t.Fatalf("err=%v", err)
	}
	if requests != 0 {
		t.Fatalf("requests=%d, want none", requests)
	}
}

// wbiNav publishes the two image names the signing key is derived from.
const wbiNav = `{"code":0,"message":"0","data":{"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png","sub_url":"https://i0.hdslb.com/bfs/wbi/4932caff0ff746eab6f01bf08b70ac45.png"}}}`

// The space feed answers 412 to a request that carries SESSDATA alone, so the
// whole cookie set has to reach the feed request.
func TestDynamicsSendsTheWholeLoginCookie(t *testing.T) {
	login := "SESSDATA=synthetic0login%2C1767225600%2Cabcde%2A11; bili_jct=synthetic; DedeUserID=316381099"
	feedCookie := ""
	c := NewClient(&http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/x/web-interface/nav":
			return response(200, wbiNav), nil
		case "/x/polymer/web-dynamic/v1/feed/space":
			feedCookie = r.Header.Get("Cookie")
			return response(200, `{"code":0,"message":"0","data":`+dynamicsPage+`}`), nil
		}
		return response(404, "{}"), nil
	})})
	s := Session{
		Buvid3: "0A1B2C3D-0A1B-0A1B-0A1B-0A1B2C3D4E5F12345infoc",
		Buvid4: "synthetic-buvid4==",
		Login:  login,
	}
	if _, err := c.Dynamics(context.Background(), "316381099", 20, s); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"buvid3=", "SESSDATA=synthetic0login", "bili_jct=synthetic", "DedeUserID=316381099"} {
		if !strings.Contains(feedCookie, want) {
			t.Fatalf("cookie=%q missing %q", feedCookie, want)
		}
	}
}
