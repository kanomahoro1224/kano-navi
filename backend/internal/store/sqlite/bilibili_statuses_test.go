package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// poolDynamic stores a synthetic pooled dynamic.
func poolDynamic(t *testing.T, s *Store, id, mid, name, text, published, url string, images int) {
	t.Helper()
	media := make([]string, images)
	for i := range media {
		media[i] = fmt.Sprintf(`"https://i0.hdslb.com/bfs/new_dyn/synthetic-%s-%d.jpg"`, id, i)
	}
	raw := `{"id":"` + id + `","mediaURLs":[` + strings.Join(media, ",") + `]}`
	if _, err := s.DB().Exec(`INSERT INTO bilibili_statuses(id,author_mid,author_name,text,published_at,url,raw_json,forwarded_at,first_seen_at,updated_at)
		VALUES(?,?,?,?,?,?,?,'2026-11-07T06:00:00Z','2026-11-07T06:00:00.000Z','2026-11-07T06:00:00.000Z')`, id, mid, name, text, published, url, raw); err != nil {
		t.Fatal(err)
	}
}

// poolForward stores a synthetic own forward of a pooled dynamic.
func poolForward(t *testing.T, s *Store, id, quoted, published string, hidden, quoteHidden, unavailable bool) {
	t.Helper()
	stamp := func(on bool) any {
		if on {
			return "2026-11-07T07:00:00.000Z"
		}
		return nil
	}
	quote := fmt.Sprintf(`{"id":%q,"url":"https://www.bilibili.com/opus/%s","unavailable":%t}`, quoted, strings.TrimPrefix(quoted, "bilibili-"), unavailable)
	if !unavailable {
		quote = fmt.Sprintf(`{"id":%q,"authorHandle":"42","text":"quoted","url":"https://www.bilibili.com/opus/%s"}`, quoted, strings.TrimPrefix(quoted, "bilibili-"))
	}
	if _, err := s.DB().Exec(`INSERT INTO posts(id,source,account_handle,type,text,published_at,url,quote_json,hidden_at,quote_hidden_at)
		VALUES(?,'bilibili','316381099','daily','fwd',?,?,?,?,?)`, id, published, "https://www.bilibili.com/opus/"+strings.TrimPrefix(id, "bilibili-"), quote, stamp(hidden), stamp(quoteHidden)); err != nil {
		t.Fatal(err)
	}
}

func TestBilibiliPoolListsForwardedOriginals(t *testing.T) {
	s := biliStore(t)
	ctx := context.Background()
	const (
		public  = "bilibili-900000000000000301"
		video   = "bilibili-900000000000000302"
		gone    = "bilibili-900000000000000303"
		quoteHd = "bilibili-900000000000000304"
		self    = "bilibili-900000000000000305"
	)
	poolDynamic(t, s, public, "42", "Synthetic Agency", "明天20点直播 100%", "2026-11-07T04:00:00Z", "https://www.bilibili.com/opus/900000000000000301", 2)
	poolDynamic(t, s, video, "43", "Synthetic Studio", "新视频", "2026-11-07T05:00:00Z", "https://www.bilibili.com/video/BV1synthetic0", 1)
	poolDynamic(t, s, gone, "42", "Synthetic Agency", "已删除", "2026-11-07T03:00:00Z", "https://www.bilibili.com/opus/900000000000000303", 0)
	poolDynamic(t, s, quoteHd, "44", "Other", "其他", "2026-11-07T02:00:00Z", "https://www.bilibili.com/opus/900000000000000304", 0)
	// A forward of an own dynamic: the original is a stored post.
	poolDynamic(t, s, self, "316381099", "Kano", "自己的", "2026-11-07T06:00:00Z", "https://www.bilibili.com/opus/900000000000000305", 0)
	if _, err := s.DB().Exec(`INSERT INTO posts(id,source,account_handle,type,text,published_at,url) VALUES(?,'bilibili','316381099','daily','自己的','2026-11-07T06:00:00Z','https://www.bilibili.com/opus/900000000000000305')`, self); err != nil {
		t.Fatal(err)
	}
	// Six forwards of the public one: the oldest is the only shown one with
	// the quote available, and the list keeps the newest five.
	poolForward(t, s, "bilibili-900000000000000410", public, "2026-11-07T08:00:00Z", false, false, false)
	for i := 1; i <= 5; i++ {
		poolForward(t, s, fmt.Sprintf("bilibili-90000000000000041%d", i), public, fmt.Sprintf("2026-11-07T09:0%d:00Z", i), i%2 == 0, i%2 == 1, false)
	}
	poolForward(t, s, "bilibili-900000000000000420", video, "2026-11-07T08:00:00Z", true, false, false)
	poolForward(t, s, "bilibili-900000000000000430", gone, "2026-11-07T08:00:00Z", false, false, true)
	poolForward(t, s, "bilibili-900000000000000440", quoteHd, "2026-11-07T08:00:00Z", false, true, false)
	poolForward(t, s, "bilibili-900000000000000441", quoteHd, "2026-11-07T08:30:00Z", false, false, true)

	page, err := s.ListBilibiliStatuses(ctx, AdminBilibiliStatusFilter{})
	if err != nil || page.Total != 4 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	var order []string
	byID := map[string]AdminBilibiliStatus{}
	for _, item := range page.Items {
		order = append(order, item.ID)
		byID[item.ID] = item
	}
	if want := []string{video, public, gone, quoteHd}; strings.Join(order, " ") != strings.Join(want, " ") {
		t.Fatalf("order=%v", order)
	}
	for id, want := range map[string]string{public: "public", video: "hidden", gone: "gone", quoteHd: "hidden"} {
		if got := byID[id].Shown; got != want {
			t.Fatalf("%s shown=%s, want %s", id, got, want)
		}
	}
	item := byID[public]
	var forwards []string
	for _, f := range item.ForwardedBy {
		forwards = append(forwards, f.PostID)
	}
	if want := "bilibili-900000000000000415 bilibili-900000000000000414 bilibili-900000000000000413 bilibili-900000000000000412 bilibili-900000000000000411"; strings.Join(forwards, " ") != want {
		t.Fatalf("forwardedBy=%v", forwards)
	}
	if first := item.ForwardedBy[0]; first.Hidden || !first.QuoteHidden || !first.QuoteAvailable {
		t.Fatalf("first forward=%+v", first)
	}
	if item.Kind != "dynamic" || item.Images != 2 || item.AuthorMid != "42" || *item.AuthorName != "Synthetic Agency" ||
		strings.Join(item.Reasons, ",") != "forwarded" || *item.ForwardedAt != "2026-11-07T06:00:00Z" {
		t.Fatalf("item=%+v", item)
	}
	if byID[video].Kind != "video" || byID[gone].ForwardedBy[0].QuoteAvailable {
		t.Fatalf("video=%+v gone=%+v", byID[video], byID[gone])
	}

	for _, c := range []struct {
		filter AdminBilibiliStatusFilter
		want   string
	}{
		{AdminBilibiliStatusFilter{Author: "42"}, public + " " + gone},
		{AdminBilibiliStatusFilter{Author: "4"}, ""},
		{AdminBilibiliStatusFilter{Author: "studio"}, video},
		{AdminBilibiliStatusFilter{Kind: "video"}, video},
		{AdminBilibiliStatusFilter{Kind: "dynamic"}, public + " " + gone + " " + quoteHd},
		{AdminBilibiliStatusFilter{Search: "直播"}, public},
		{AdminBilibiliStatusFilter{Search: "100%"}, public},
		{AdminBilibiliStatusFilter{Search: "%"}, public},
		{AdminBilibiliStatusFilter{Search: "_"}, ""},
		{AdminBilibiliStatusFilter{Search: "900000000000000303"}, gone},
		{AdminBilibiliStatusFilter{Search: gone}, gone},
		{AdminBilibiliStatusFilter{Search: "自己的"}, ""},
	} {
		got, err := s.ListBilibiliStatuses(ctx, c.filter)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, item := range got.Items {
			ids = append(ids, item.ID)
		}
		if strings.Join(ids, " ") != c.want {
			t.Fatalf("%+v=%v, want %s", c.filter, ids, c.want)
		}
	}
	second, err := s.ListBilibiliStatuses(ctx, AdminBilibiliStatusFilter{Page: 2, PageSize: 3})
	if err != nil || second.TotalPages != 2 || len(second.Items) != 1 || second.Items[0].ID != quoteHd {
		t.Fatalf("second=%+v err=%v", second, err)
	}

	// The summary is the bilibili_status owner's.
	if err = s.SetManualTranslation(ctx, TranslationOwnerBilibiliStatus, public, "en", "Live at 20:00 tomorrow"); err != nil {
		t.Fatal(err)
	}
	page, _ = s.ListBilibiliStatuses(ctx, AdminBilibiliStatusFilter{Search: "直播"})
	summary := page.Items[0].Translation
	if summary == nil || summary.Status != "incomplete" {
		t.Fatalf("translation=%+v", summary)
	}
	locales := map[string]AdminTranslationLocale{}
	for _, l := range summary.Locales {
		locales[l.Locale] = l
	}
	if l := locales["en"]; l.Origin == nil || *l.Origin != "manual" {
		t.Fatalf("en=%+v", locales["en"])
	}
}
