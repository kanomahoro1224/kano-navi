package bilibili

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// WBIKeys are the rotating keys Bilibili's web pages sign some API requests
// with. They come from /x/web-interface/nav and change about daily.
type WBIKeys struct {
	Img string `json:"img"`
	Sub string `json:"sub"`
	// At is when the keys were read.
	At string `json:"at,omitempty"`
}

// wbiTTL is how long read keys are reused.
const wbiTTL = 12 * time.Hour

var wbiKeyPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func (k WBIKeys) valid() bool {
	return wbiKeyPattern.MatchString(k.Img) && wbiKeyPattern.MatchString(k.Sub)
}

func (k WBIKeys) fresh(now time.Time) bool {
	at, err := time.Parse(time.RFC3339Nano, k.At)
	return k.valid() && err == nil && now.Sub(at) < wbiTTL && !at.After(now)
}

var mixinKeyEncTab = [64]int{
	46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35, 27, 43, 5, 49,
	33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13, 37, 48, 7, 16, 24, 55, 40,
	61, 26, 17, 0, 1, 60, 51, 30, 4, 22, 25, 54, 21, 56, 59, 6, 63, 57, 62, 11,
	36, 20, 34, 44, 52,
}

// mixinKey reorders img+sub and keeps 32 characters.
func (k WBIKeys) mixinKey() string {
	raw := k.Img + k.Sub
	var b strings.Builder
	for _, i := range mixinKeyEncTab {
		if i < len(raw) {
			b.WriteByte(raw[i])
		}
	}
	out := b.String()
	return out[:min(32, len(out))]
}

// wbiEscape is encodeURIComponent: uppercase escapes and %20 for a space.
func wbiEscape(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

// signWBI returns the query with wts and w_rid added. Values lose the
// characters !'()* first, as Bilibili's own script does.
func signWBI(query url.Values, keys WBIKeys, now time.Time) string {
	params := map[string]string{"wts": strconv.FormatInt(now.Unix(), 10)}
	for key, values := range query {
		if len(values) > 0 {
			params[key] = strings.Map(func(r rune) rune {
				if strings.ContainsRune("!'()*", r) {
					return -1
				}
				return r
			}, values[0])
		}
	}
	names := make([]string, 0, len(params))
	for key := range params {
		names = append(names, key)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names)+1)
	for _, key := range names {
		parts = append(parts, wbiEscape(key)+"="+wbiEscape(params[key]))
	}
	encoded := strings.Join(parts, "&")
	sum := md5.Sum([]byte(encoded + keys.mixinKey()))
	return encoded + "&w_rid=" + hex.EncodeToString(sum[:])
}

// parseNavKeys reads the key file names from a nav answer. A signed-out
// answer (-101) still carries them.
func parseNavKeys(body []byte, now time.Time) (WBIKeys, bool, error) {
	var out struct {
		Code *int `json:"code"`
		Data struct {
			IsLogin bool `json:"isLogin"`
			WBI     struct {
				Img string `json:"img_url"`
				Sub string `json:"sub_url"`
			} `json:"wbi_img"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &out) != nil || out.Code == nil {
		if _, err := envelope(body); err != nil {
			return WBIKeys{}, false, err
		}
		return WBIKeys{}, false, &Error{"bilibili_invalid_response"}
	}
	if *out.Code != 0 && *out.Code != -101 {
		_, err := envelope(body)
		return WBIKeys{}, false, err
	}
	stem := func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		return strings.TrimSuffix(path.Base(u.Path), path.Ext(u.Path))
	}
	keys := WBIKeys{Img: stem(out.Data.WBI.Img), Sub: stem(out.Data.WBI.Sub), At: now.UTC().Format(time.RFC3339Nano)}
	if !keys.valid() {
		return WBIKeys{}, false, &Error{"bilibili_invalid_response"}
	}
	return keys, out.Data.IsLogin, nil
}

// NavKeys reads the current WBI keys and whether the login cookie is
// accepted.
func (c *Client) NavKeys(ctx context.Context, session Session, now time.Time) (WBIKeys, bool, error) {
	body, err := c.fetch(ctx, apiHost, "/x/web-interface/nav", "", "https://www.bilibili.com/", "application/json", &session, true)
	if err != nil {
		return WBIKeys{}, false, err
	}
	return parseNavKeys(body, now)
}

// spaceOffset is a listing continuation as Bilibili answers it: a dynamic
// ID, or empty for the first page.
var spaceOffset = regexp.MustCompile(`^[0-9]{0,20}$`)

// spaceFeatures are the listing features Bilibili's own space page asks for;
// itemOpusStyle returns pictures and text as an opus card.
const spaceFeatures = "itemOpusStyle,listOnlyfans,opusBigCover,onlyfansVote,forwardListHidden,decorationCard,commentsNewVersion,onlyfansAssetsV2,ugcDelete,onlyfansQaCard"

// SpaceDynamics reads the first page of an account's dynamics. It needs the
// login cookie, which it sends, and a WBI signature.
func (c *Client) SpaceDynamics(ctx context.Context, account string, session Session, keys WBIKeys, now time.Time) (SpacePage, error) {
	return c.SpaceDynamicsPage(ctx, account, "", session, keys, now)
}

// SpaceDynamicsPage reads the page of an account's dynamics that follows
// offset, the previous page's offset; an empty offset is the first page.
func (c *Client) SpaceDynamicsPage(ctx context.Context, account, offset string, session Session, keys WBIKeys, now time.Time) (SpacePage, error) {
	if !mid.MatchString(account) {
		return SpacePage{}, &Error{"bilibili_invalid_account"}
	}
	if !session.valid() {
		return SpacePage{}, &Error{"bilibili_invalid_session"}
	}
	if !sessdata.MatchString(session.Login) {
		return SpacePage{}, &Error{"bilibili_login_required"}
	}
	if !keys.valid() {
		return SpacePage{}, &Error{"bilibili_invalid_keys"}
	}
	if !spaceOffset.MatchString(offset) {
		return SpacePage{}, &Error{"bilibili_invalid_page"}
	}
	query := url.Values{"host_mid": {account}, "offset": {offset}, "timezone_offset": {"-480"}, "platform": {"web"}, "features": {spaceFeatures}, "web_location": {"333.1387"}}
	body, err := c.fetch(ctx, apiHost, "/x/polymer/web-dynamic/v1/feed/space", signWBI(query, keys, now), "https://space.bilibili.com/"+account+"/dynamic", "application/json", &session, true)
	if err != nil {
		return SpacePage{}, err
	}
	data, err := envelope(body)
	if err != nil {
		return SpacePage{}, err
	}
	return ParseSpaceFeed(data, account)
}

// OpusPage reads one dynamic from its www.bilibili.com/opus page, which
// renders the dynamic's state on the server and needs no login.
func (c *Client) OpusPage(ctx context.Context, id string, session Session) (Dynamic, error) {
	if !dynamicIDPattern.MatchString(id) {
		return Dynamic{}, &Error{"bilibili_invalid_dynamic"}
	}
	if !session.valid() {
		return Dynamic{}, &Error{"bilibili_invalid_session"}
	}
	body, err := c.fetch(ctx, wwwHost, "/opus/"+id, "", "https://www.bilibili.com/", "text/html", &session, false)
	if err != nil {
		return Dynamic{}, err
	}
	return ParseOpusPage(body, id)
}
