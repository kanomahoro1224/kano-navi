package bilibili

import (
	"context"
	"net/url"
)

// DesktopSpaceDynamics reads one page from Bilibili's desktop dynamics feed.
// The endpoint needs the signed-in session but does not use WBI signing.
func (c *Client) DesktopSpaceDynamics(ctx context.Context, account, offset string, session Session) (SpacePage, error) {
	if !mid.MatchString(account) {
		return SpacePage{}, &Error{"bilibili_invalid_account"}
	}
	if !session.valid() {
		return SpacePage{}, &Error{"bilibili_invalid_session"}
	}
	if !sessdata.MatchString(session.Login) {
		return SpacePage{}, &Error{"bilibili_login_required"}
	}
	if !spaceOffset.MatchString(offset) {
		return SpacePage{}, &Error{"bilibili_invalid_page"}
	}
	query := url.Values{
		"features":        {"itemOpusStyle"},
		"host_mid":        {account},
		"offset":          {offset},
		"timezone_offset": {"-480"},
	}
	body, err := c.fetch(ctx, apiHost, "/x/polymer/web-dynamic/desktop/v1/feed/space", query.Encode(), "https://space.bilibili.com/"+account+"/dynamic", "application/json", &session, true)
	if err != nil {
		return SpacePage{}, err
	}
	data, err := envelope(body)
	if err != nil {
		return SpacePage{}, err
	}
	return ParseDesktopSpaceFeed(data, account)
}
