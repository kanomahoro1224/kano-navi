package x

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTimelineListsRepostsApartFromOwnStatuses(t *testing.T) {
	at := time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)
	own := statusFixture{id: "1900000000000000400", text: "own", at: at}
	reposted := statusFixture{id: "1900000000000000401", text: "agency notice", at: at.Add(-time.Hour), author: "agency_synthetic",
		media: []mediaFixture{{kind: "photo", file: "https://pbs.twimg.com/media/notice.jpg"}}}
	repost := reposted.timeline()
	repost["reposted_by"] = map[string]any{"screen_name": "Kano", "name": "Synthetic"}
	// Another author's status that names no repost of the account is skipped.
	stray := statusFixture{id: "1900000000000000402", text: "stray", at: at, author: "someone"}.timeline()
	strayRepost := statusFixture{id: "1900000000000000403", text: "other repost", at: at, author: "someone"}.timeline()
	strayRepost["reposted_by"] = "other_account"
	page, err := parseTimelineStatuses([]byte(fxPage(t, "next", own.timeline(), repost, stray, strayRepost)), "kano")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Statuses) != 1 || page.Statuses[0].ID != own.id {
		t.Fatalf("statuses=%+v", page.Statuses)
	}
	if len(page.Reposts) != 1 {
		t.Fatalf("reposts=%+v", page.Reposts)
	}
	r := page.Reposts[0]
	if r.ID != reposted.id || r.Author != "agency_synthetic" || r.Status == nil {
		t.Fatalf("repost=%+v", r)
	}
	// The reposted status maps as a quote of it would.
	quoting, err := parseTimelineStatuses([]byte(fxPage(t, "", statusFixture{id: "1900000000000000404", text: "これ", at: at, quote: &reposted}.timeline())), "kano")
	if err != nil || len(quoting.Statuses) != 1 || quoting.Statuses[0].Post == nil {
		t.Fatalf("quoting page=%+v err=%v", quoting, err)
	}
	if want := quoting.Statuses[0].Post.Quote; quoteSummary(t, r.Status) != quoteSummary(t, want) {
		t.Fatalf("status=%s want=%s", quoteSummary(t, r.Status), quoteSummary(t, want))
	}
}

func TestAPIRepostsReadRetweetedReferences(t *testing.T) {
	own := statusFixture{id: "1900000000000000500", text: "own", at: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)}
	answer := apiAnswer(t, "", own)
	ownAuthor := answer["data"].([]any)[0].(map[string]any)["author_id"]
	answer["data"] = append(answer["data"].([]any),
		map[string]any{"id": "1900000000000000501", "author_id": ownAuthor, "created_at": "2026-10-20T00:00:00.000Z", "text": "RT @agency_synthetic: notice",
			"referenced_tweets": []any{map[string]any{"type": "retweeted", "id": "1900000000000000001"}}},
		// A retweet whose status X did not return keeps only the link.
		map[string]any{"id": "1900000000000000502", "author_id": ownAuthor, "created_at": "2026-10-20T00:00:00.000Z", "text": "RT @gone: x",
			"referenced_tweets": []any{map[string]any{"type": "retweeted", "id": "1900000000000000002"}}})
	includes := answer["includes"].(map[string]any)
	includes["users"] = append(includes["users"].([]any), map[string]any{"id": "9001", "username": "agency_synthetic", "name": "Agency"})
	includes["tweets"] = append(list(includes["tweets"]), map[string]any{"id": "1900000000000000001", "author_id": "9001", "created_at": "2026-10-19T00:00:00.000Z", "text": "notice"})
	var payload map[string]any
	if err := json.Unmarshal(decoded(t, answer), &payload); err != nil {
		t.Fatal(err)
	}
	reposts := APIReposts(payload, "kano")
	if len(reposts) != 2 {
		t.Fatalf("reposts=%+v", reposts)
	}
	if r := reposts[0]; r.ID != "1900000000000000001" || r.Author != "agency_synthetic" || r.Status == nil || r.Status.Text != "notice" {
		t.Fatalf("first=%+v status=%+v", r, r.Status)
	}
	if r := reposts[1]; r.ID != "1900000000000000002" || r.Author != "" || r.Status != nil {
		t.Fatalf("second=%+v", r)
	}
	// The posts themselves still leave the reposts out.
	if posts, _, _, err := APIPosts(payload, "kano"); err != nil || len(posts) != 1 {
		t.Fatalf("posts=%+v err=%v", posts, err)
	}
}
