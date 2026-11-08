package bilibili

import (
	"encoding/json"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Video is one upload as stored in videos (source = 'bilibili').
type Video struct {
	BVID, Title, URL, ThumbnailURL, Kind string
	// PublishedAt is the public release time, RFC 3339 in UTC.
	PublishedAt     string
	DurationSeconds int
	AID             int64
	// OwnerMid is the uploader's account, or 0 when the answer omits it.
	OwnerMid int64
	// Description is kept in raw_json only; cross-platform matching reads it.
	Description string
}

// maxDescription bounds the description kept from one upload, in runes.
const maxDescription = 2000

var bvidPattern = regexp.MustCompile(`^BV1[1-9A-HJ-NP-Za-km-z]{9}$`)

// ParseBVID accepts a BV ID or a bilibili.com video URL and returns the ID.
// Short b23.tv links would need a redirect and are not accepted.
func ParseBVID(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if bvidPattern.MatchString(input) {
		return input, true
	}
	u, err := url.Parse(input)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host != "bilibili.com" && !strings.HasSuffix(host, ".bilibili.com") {
		return "", false
	}
	for _, part := range strings.Split(u.Path, "/") {
		if bvidPattern.MatchString(part) {
			return part, true
		}
	}
	return "", false
}

// KindUpload is the form of every Bilibili upload. Bilibili reports no
// broadcast facts for one, and its own categories say nothing about the
// content (a cover and a stream clip can share one); what an upload is about
// is its content tag.
const KindUpload = "upload"

// thumbnail keeps a cover only on Bilibili's image CDN, upgraded to HTTPS.
func thumbnail(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return ""
	}
	if !strings.HasSuffix(strings.ToLower(u.Hostname()), ".hdslb.com") {
		return ""
	}
	u.Scheme, u.Fragment = "https", ""
	return u.String()
}

type upload struct {
	BVID     string          `json:"bvid"`
	AID      int64           `json:"aid"`
	Title    string          `json:"title"`
	Pic      string          `json:"pic"`
	PubDate  int64           `json:"pubdate"`
	Duration json.RawMessage `json:"duration"`
	Desc     string          `json:"desc"`
	Owner    *struct {
		Mid int64 `json:"mid"`
	} `json:"owner"`
	UpMid int64 `json:"upMid"`
}

// video validates one upload; it reports false for an incomplete one.
func (u upload) video() (Video, bool) {
	title := strings.TrimSpace(html.UnescapeString(u.Title))
	if !bvidPattern.MatchString(u.BVID) || title == "" || u.PubDate <= 0 {
		return Video{}, false
	}
	description := strings.TrimSpace(u.Desc)
	if utf8.RuneCountInString(description) > maxDescription {
		description = string([]rune(description)[:maxDescription])
	}
	owner := u.UpMid
	if u.Owner != nil {
		owner = u.Owner.Mid
	}
	return Video{
		BVID: u.BVID, AID: u.AID, OwnerMid: owner, Title: title, Kind: KindUpload,
		URL:             "https://www.bilibili.com/video/" + u.BVID,
		ThumbnailURL:    thumbnail(u.Pic),
		PublishedAt:     time.Unix(u.PubDate, 0).UTC().Format(time.RFC3339),
		DurationSeconds: seconds(u.Duration),
		Description:     description,
	}, true
}

// seconds reads a duration given as seconds or as "mm:ss" / "h:mm:ss".
func seconds(raw json.RawMessage) int {
	var n int
	if json.Unmarshal(raw, &n) == nil && n > 0 {
		return n
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return 0
	}
	total := 0
	for _, part := range strings.Split(text, ":") {
		v, err := strconv.Atoi(part)
		if err != nil || v < 0 {
			return 0
		}
		total = total*60 + v
	}
	return total
}

// ParseUploads reads a recArchivesByKeywords page. Entries of another
// uploader (a collaboration listed in the account's series) are skipped.
func ParseUploads(data json.RawMessage, account string) ([]Video, error) {
	page, err := ParseUploadsPage(data, account)
	return page.Videos, err
}

// UploadsPage is one page of the account's uploads listing.
type UploadsPage struct {
	Videos []Video
	// Seen counts every entry the page listed, kept or not, and Total the
	// entries the whole listing holds, or 0 when the answer omits it.
	Seen, Total int
}

// ParseUploadsPage is ParseUploads with the page's own counts, which tell a
// paged read whether more pages follow.
func ParseUploadsPage(data json.RawMessage, account string) (UploadsPage, error) {
	var page struct {
		Archives []upload `json:"archives"`
		Page     struct {
			Total int `json:"total"`
		} `json:"page"`
	}
	if json.Unmarshal(data, &page) != nil || page.Archives == nil {
		return UploadsPage{}, &Error{"bilibili_invalid_response"}
	}
	owner, _ := strconv.ParseInt(account, 10, 64)
	out := UploadsPage{Videos: []Video{}, Seen: len(page.Archives), Total: max(page.Page.Total, 0)}
	seen := map[string]bool{}
	for _, item := range page.Archives {
		if v, ok := item.video(); ok && !seen[v.BVID] && (v.OwnerMid == 0 || v.OwnerMid == owner) {
			seen[v.BVID] = true
			out.Videos = append(out.Videos, v)
		}
	}
	return out, nil
}

// ParseView reads one /x/web-interface/view answer for bvid.
func ParseView(data json.RawMessage, bvid string) (Video, error) {
	var item upload
	if json.Unmarshal(data, &item) != nil || item.BVID != bvid {
		return Video{}, &Error{"bilibili_invalid_response"}
	}
	v, ok := item.video()
	if !ok {
		return Video{}, &Error{"bilibili_invalid_response"}
	}
	return v, nil
}
