package schedule

import (
	"net/http"
	"strings"
	"testing"
)

func TestBilibiliDynamicsAreTranslatedIntoEnglishAndJapanese(t *testing.T) {
	f, translator := newTranslatorFixture(t)
	f.addProvider("chat", protocolChat, "https://chat.example.test/v1", "synthetic-chat", `["text"]`)
	f.queueTranslation("chat")
	f.exec(`INSERT INTO posts (id, source, account_handle, type, text, published_at, url) VALUES
		('bilibili-900000000000000010', 'bilibili', '316381099', 'daily', '今天北京时间17点上传新歌', ?, 'https://www.bilibili.com/opus/900000000000000010'),
		('bilibili-900000000000000011', 'bilibili', '316381099', 'daily', '', ?, 'https://www.bilibili.com/opus/900000000000000011')`, recent(1), recent(2))
	f.provider.respond = func(request recordedRequest) (int, string) {
		if !strings.Contains(request.body, "Kind: Bilibili dynamic") {
			t.Errorf("unexpected kind: %s", request.body)
		}
		return http.StatusOK, chatReply(stringify(obj("sourceLanguage", "zh", "english", "New song upload today at 17:00 Beijing time",
			"japanese", "今日の北京時間17:00に新曲をアップロード", "simplifiedChinese", "")))
	}
	result := runTranslator(t, translator)
	// The emoji-only dynamic has no text and costs no call.
	if f.provider.count() != 1 || result.Counts["success"] != 1 || result.Counts["skipped"] != 1 {
		t.Fatalf("result=%v requests=%d", result.Counts, f.provider.count())
	}
	if got := f.text(`SELECT group_concat(locale, ',') FROM (SELECT locale FROM translations WHERE owner_id = 'bilibili-900000000000000010' ORDER BY locale)`); got != "en,ja" {
		t.Fatalf("locales=%q", got)
	}
	if got := f.text(`SELECT source_language FROM translation_states WHERE owner_id = 'bilibili-900000000000000011'`); got != "none" {
		t.Fatalf("emoji-only language=%q", got)
	}
	// A Chinese source needs no zh-CN, so the dynamic is complete.
	runTranslator(t, translator)
	if f.provider.count() != 1 {
		t.Fatalf("a complete dynamic was sent again: %d", f.provider.count())
	}
}

func TestJapaneseIsRequiredUnlessItIsTheSource(t *testing.T) {
	if _, err := normalizeTranslation(obj("sourceLanguage", "zh", "english", "Hello", "japanese", "", "simplifiedChinese", ""), "你好"); err == nil || !strings.Contains(err.Error(), "missing ja") {
		t.Fatalf("err=%v", err)
	}
	result, err := normalizeTranslation(obj("sourceLanguage", "ja", "english", "Hello", "japanese", "", "simplifiedChinese", "你好"), "こんにちは")
	if err != nil || len(result.Texts) != 2 || result.Texts["ja"] != "" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
