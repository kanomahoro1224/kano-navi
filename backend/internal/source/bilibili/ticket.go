package bilibili

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// The bili_ticket request is signed with a fixed key that Bilibili's web
// script carries: hexsign is HMAC-SHA256 under ticketKey over "ts<unix>".
const (
	ticketKeyID = "ec02"
	ticketKey   = "XgwSnGZ1p"
	// ticketRefresh renews a ticket this long before it expires.
	ticketRefresh = 6 * time.Hour
)

func ticketSign(ts int64) string {
	mac := hmac.New(sha256.New, []byte(ticketKey))
	mac.Write([]byte("ts" + strconv.FormatInt(ts, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// ticketDue reports a session whose ticket is missing or expires soon.
func (s Session) ticketDue(now time.Time) bool {
	if !ticketPattern.MatchString(s.Ticket) || s.TicketTTL <= 0 {
		return true
	}
	return !now.Before(time.Unix(s.TicketAt+s.TicketTTL, 0).Add(-ticketRefresh))
}

// WebTicket requests a bili_ticket for the device identity, without the
// login cookie. The answer also names the current WBI keys.
func (c *Client) WebTicket(ctx context.Context, session Session, now time.Time) (Session, WBIKeys, error) {
	if !session.valid() {
		return session, WBIKeys{}, &Error{"bilibili_invalid_session"}
	}
	ts := now.Unix()
	query := "key_id=" + ticketKeyID + "&hexsign=" + ticketSign(ts) + "&context%5Bts%5D=" + strconv.FormatInt(ts, 10) + "&csrf="
	body, err := c.do(ctx, http.MethodPost, apiHost, "/bapis/bilibili.api.ticket.v1.Ticket/GenWebTicket", query, "https://www.bilibili.com/", "application/json", &session, false)
	if err != nil {
		return session, WBIKeys{}, err
	}
	data, err := envelope(body)
	if err != nil {
		return session, WBIKeys{}, err
	}
	var out struct {
		Ticket    string `json:"ticket"`
		CreatedAt int64  `json:"created_at"`
		TTL       int64  `json:"ttl"`
		Nav       struct {
			Img string `json:"img"`
			Sub string `json:"sub"`
		} `json:"nav"`
	}
	if json.Unmarshal(data, &out) != nil || !ticketPattern.MatchString(out.Ticket) || out.CreatedAt <= 0 || out.TTL <= 0 || out.TTL > 30*24*3600 {
		return session, WBIKeys{}, &Error{"bilibili_invalid_response"}
	}
	session.Ticket, session.TicketAt, session.TicketTTL = out.Ticket, out.CreatedAt, out.TTL
	keys := WBIKeys{Img: keyStem(out.Nav.Img), Sub: keyStem(out.Nav.Sub), At: now.UTC().Format(time.RFC3339Nano)}
	if !keys.valid() {
		keys = WBIKeys{}
	}
	return session, keys, nil
}

// keyStem is a WBI key: the file name of the URL that carries it.
func keyStem(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	name := u.Path
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			name = name[i+1:]
			break
		}
	}
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return name[:i]
		}
	}
	return name
}
