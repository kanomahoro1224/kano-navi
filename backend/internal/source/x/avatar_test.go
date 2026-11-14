package x

import (
	"testing"
)

func TestAvatarExtractionAndNormalization(t *testing.T) {
	body := []byte(`{
		"code": 200,
		"results": [
			{
				"type": "status",
				"id": "1900000000000000901",
				"text": "hello",
				"created_timestamp": 1792537200,
				"author": {
					"screen_name": "Kano_2525",
					"name": "Kano",
					"avatar_url": "https://pbs.twimg.com/profile_images/1/main_normal.jpg#frag"
				},
				"quote": {
					"type": "status",
					"id": "1900000000000000902",
					"text": "quoted",
					"author": {
						"screen_name": "Partner_X",
						"name": "Partner",
						"avatar_url": "https://pbs.twimg.com/profile_images/2/quote_normal.png"
					}
				}
			},
			{
				"type": "status",
				"id": "1900000000000000903",
				"text": "reposted notice",
				"created_timestamp": 1792537100,
				"reposted_by": "Kano_2525",
				"author": {
					"screen_name": "Agency_PR",
					"name": "Agency",
					"avatar_url": "https://pbs.twimg.com/profile_images/3/agency_400x400.jpg"
				}
			}
		]
	}`)

	page, err := parseTimelineStatuses(body, "Kano_2525")
	if err != nil {
		t.Fatalf("parseTimelineStatuses: %v", err)
	}
	if len(page.Statuses) != 1 || page.Statuses[0].Post == nil {
		t.Fatalf("statuses = %+v", page.Statuses)
	}
	post := page.Statuses[0].Post
	wantMain := "https://pbs.twimg.com/profile_images/1/main_400x400.jpg"
	wantQuote := "https://pbs.twimg.com/profile_images/2/quote_400x400.png"
	wantAgency := "https://pbs.twimg.com/profile_images/3/agency_400x400.jpg"

	if post.AuthorAvatarURL != wantMain {
		t.Fatalf("AuthorAvatarURL = %q, want %q", post.AuthorAvatarURL, wantMain)
	}
	if post.Quote == nil || post.Quote.AuthorAvatarURL != wantQuote {
		t.Fatalf("Quote.AuthorAvatarURL = %v, want %q", post.Quote, wantQuote)
	}
	// Trimmed Raw author stays free of profile metadata like avatar_url.
	if author, _ := post.Raw["author"].(map[string]any); author["avatar_url"] != nil {
		t.Fatalf("raw author should not retain avatar_url: %v", author)
	}
	if page.Avatars["kano_2525"] != wantMain {
		t.Fatalf("page.Avatars[kano_2525] = %q, want %q", page.Avatars["kano_2525"], wantMain)
	}
	if page.Avatars["partner_x"] != wantQuote {
		t.Fatalf("page.Avatars[partner_x] = %q, want %q", page.Avatars["partner_x"], wantQuote)
	}
	if page.Avatars["agency_pr"] != wantAgency {
		t.Fatalf("page.Avatars[agency_pr] = %q, want %q", page.Avatars["agency_pr"], wantAgency)
	}
}
