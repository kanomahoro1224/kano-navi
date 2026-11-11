package schedule

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"
	"time"
)

// forwardedQuote is the projection of another author's dynamic that an own
// forward forwards: a talk stream from 20:00 Beijing time tomorrow.
const forwardedQuote = `{"id":"bilibili-900000000000000210","authorHandle":"42","authorName":"Synthetic Agency","text":"明天20点开始杂谈","publishedAt":"2026-11-07T02:53:20Z","url":"https://www.bilibili.com/opus/900000000000000210"}`

func TestBilibiliForwardWordingIsMarkedContext(t *testing.T) {
	f := newFixture(t)
	f.addProvider("p-main", protocolChat, "https://provider.example.test/v1", "model-a", `["text"]`)
	f.queue("p-main")
	opus := f.addBilibiliPost("900000000000000110", "看这个", "2026-11-07T04:04:48Z")
	f.addBilibiliPost("900000000000000111", "", "2026-11-07T05:00:00Z")
	f.exec(`UPDATE posts SET quote_json=? WHERE id IN ('bilibili-900000000000000110','bilibili-900000000000000111')`, forwardedQuote)
	// A forward whose own wording and quote say nothing of a schedule.
	f.addBilibiliPost("900000000000000112", "看这个", "2026-11-07T06:00:00Z")
	f.exec(`UPDATE posts SET quote_json=? WHERE id='bilibili-900000000000000112'`, strings.Replace(forwardedQuote, "明天20点开始杂谈", "谢谢大家", 1))
	bodies := map[string]string{}
	f.provider.respond = func(request recordedRequest) (int, string) {
		for _, id := range []string{"900000000000000110", "900000000000000111"} {
			if strings.Contains(request.body, "opus/"+id) {
				bodies[id] = request.body
			}
		}
		return http.StatusOK, chatReply(stringify(obj(
			"classification", "schedule",
			"events", []any{bilibiliEntry("杂谈", "2026-11-08", "20:00", "stream", nil, nil)},
			"confidence", 0.9, "evidence", "synthetic evidence", "reason", nil, "action", "add")))
	}
	f.scan()
	// The Chinese heuristic reads the quoted wording too.
	if f.provider.count() != 2 || len(bodies) != 2 {
		t.Fatalf("requests=%d bodies=%d", f.provider.count(), len(bodies))
	}
	marker := `[Forwarded dynamic by Synthetic Agency. The source post forwards this wording; it is context, not the source post's own text. Times written in it without a zone are Beijing time, as in the source post.]\n明天20点开始杂谈`
	if body := bodies["900000000000000110"]; !strings.Contains(body, `看这个\n\n`+marker) || strings.Contains(body, "@42") || strings.Contains(body, "Quoted post") {
		t.Fatalf("request lacks the marked forward: %s", body)
	}
	if body := bodies["900000000000000111"]; !strings.Contains(body, `\n\n\n\n`+marker) {
		t.Fatalf("textless forward request: %s", body)
	}
	// The quoted time is Beijing time like the dynamic's own.
	var at string
	if err := f.db.QueryRow(`SELECT starts_at FROM events WHERE source_item_id='bilibili-900000000000000110' AND deleted_at IS NULL`).Scan(&at); err != nil {
		t.Fatal(err)
	}
	if parsed, _ := time.Parse(time.RFC3339Nano, at); parsed.In(jst).Format("2006-01-02 15:04") != "2026-11-08 21:00" {
		t.Fatalf("starts_at=%s", at)
	}
	if got := f.text(`SELECT url FROM events WHERE source_item_id='bilibili-900000000000000110' AND deleted_at IS NULL`); got != opus {
		t.Fatalf("event url=%s", got)
	}
	// A hidden quote gives no context: the forward's own wording alone is
	// not a schedule, so it is no longer selected.
	f.exec(`UPDATE posts SET quote_hidden_at=? WHERE id='bilibili-900000000000000110'`, testTimestamp)
	f.scan()
	if f.provider.count() != 2 {
		t.Fatalf("hidden quote reached the model: %d requests", f.provider.count())
	}
}

func TestBilibiliQuotedContextNamesTheAuthorByDisplayName(t *testing.T) {
	for _, c := range []struct{ source, quote, want string }{
		{"bilibili", forwardedQuote, "[Forwarded dynamic by Synthetic Agency."},
		{"bilibili", strings.Replace(forwardedQuote, `"authorName":"Synthetic Agency",`, "", 1), "[Forwarded dynamic by another account."},
		{"x", `{"id":"1900000000000000001","authorHandle":"agency_synthetic","authorName":"Agency","text":"明日","url":"https://x.com/agency_synthetic/status/1900000000000000001"}`, "[Quoted post by @agency_synthetic."},
	} {
		got := quotedInput(quoteContextOf(c.source, sql.NullString{String: c.quote, Valid: true}))
		if !strings.HasPrefix(got, "\n\n"+c.want) {
			t.Errorf("%s %s => %q", c.source, c.quote, got)
		}
	}
}

func TestForwardedDynamicsAreTranslatedOnceFromThePool(t *testing.T) {
	f, translator := newTranslatorFixture(t)
	f.addProvider("chat", protocolChat, "https://chat.example.test/v1", "synthetic-chat", `["text"]`)
	f.queueTranslation("chat")
	f.exec(`INSERT INTO bilibili_statuses (id, author_mid, author_name, text, published_at, url, forwarded_at, first_seen_at, updated_at) VALUES
		('bilibili-900000000000000210', '42', 'Synthetic Agency', '明天20点开始杂谈', ?, 'https://www.bilibili.com/opus/900000000000000210', ?, ?, ?)`, recent(9), recent(1), recent(1), recent(1))
	kinds := []string{}
	f.provider.respond = func(request recordedRequest) (int, string) {
		if strings.Contains(request.body, "Kind: Bilibili dynamic") {
			kinds = append(kinds, "bilibili")
		}
		return http.StatusOK, chatReply(stringify(obj("sourceLanguage", "zh", "english", "Talk stream tomorrow from 20:00",
			"japanese", "明日20時から雑談配信", "simplifiedChinese", "")))
	}
	result := runTranslator(t, translator)
	if f.provider.count() != 1 || len(kinds) != 1 || result.Counts["success"] != 1 {
		t.Fatalf("result=%v requests=%d kinds=%v", result.Counts, f.provider.count(), kinds)
	}
	if got := f.text(`SELECT group_concat(locale, ',') FROM (SELECT locale FROM translations WHERE owner_type = 'bilibili_status' AND owner_id = 'bilibili-900000000000000210' ORDER BY locale)`); got != "en,ja" {
		t.Fatalf("locales=%q", got)
	}
	runTranslator(t, translator)
	if f.provider.count() != 1 {
		t.Fatalf("a translated dynamic was sent again: %d", f.provider.count())
	}
}
