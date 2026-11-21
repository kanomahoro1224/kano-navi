package alerts

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	sendTimeout   = 10 * time.Second
	readLimit     = 64 << 10
	telegramAPI   = "https://api.telegram.org"
	userAgent     = "kano-navi-alerts/1"
	signatureName = "X-Kano-Signature"
)

// Delivery kinds.
const (
	KindOpened   = "opened"
	KindResolved = "resolved"
	KindTest     = "test"
)

// Send error codes. They are the only failure detail a delivery records:
// response bodies, headers and addresses never are.
const (
	codeTimeout     = "timeout"
	codeUnreachable = "unreachable"
	codeRejected    = "rejected"
	codeRedirect    = "http_3xx"
	codeClient      = "http_4xx"
	codeRateLimited = "http_429"
	codeServer      = "http_5xx"
	codeUnreadable  = "secret_unreadable"
	codeFailed      = "failed"
)

// retryable reports whether a later attempt may succeed.
func retryable(code string) bool {
	return code == codeTimeout || code == codeUnreachable || code == codeRateLimited || code == codeServer
}

// message is what every channel delivers: fixed English text built from an
// incident's rule, subject and sanitized detail.
type message struct {
	Kind       string
	Rule       string
	Subject    string
	Severity   Severity
	Title      string
	Body       string
	OpenedAt   string
	ResolvedAt string
	Link       string
	SentAt     string
}

var subjectNames = map[string]string{
	"x": "X", "x_related": "Related X accounts", "youtube": "YouTube", "bilibili": "Bilibili",
	"bilibili_dynamics": "Bilibili dynamics", "shop": "Official shop", "live": "Live-status probe",
	"llm": "model provider", "proxy": "outbound proxy", "web_bot_auth": "Web Bot Auth key", "alerts": "alert channel",
}

func subjectName(subject string) string {
	if name := subjectNames[subject]; name != "" {
		return name
	}
	return subject
}

var tokyo = func() *time.Location {
	location, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		return time.FixedZone("JST", 9*60*60)
	}
	return location
}()

// tokyoTime displays a stored instant in Asia/Tokyo.
func tokyoTime(value string) string {
	at, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return ""
	}
	return at.In(tokyo).Format("2006-01-02 15:04") + " JST"
}

func detailInt(detail map[string]any, key string) int {
	switch value := detail[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	}
	return 0
}

func detailString(detail map[string]any, key string) string {
	value, _ := detail[key].(string)
	return value
}

// render builds the message of one delivery. incident is nil for a test.
func render(kind string, incident *incident, link string, now time.Time) message {
	out := message{Kind: kind, Link: link, SentAt: formatTime(now), Severity: SeverityInfo}
	if incident == nil {
		out.Title = "Test alert"
		out.Body = "This channel receives alerts from the operator console."
		return out
	}
	out.Rule, out.Subject, out.Severity = incident.Rule, incident.Subject, incident.Severity
	out.OpenedAt = incident.OpenedAt
	if incident.ResolvedAt != nil {
		out.ResolvedAt = *incident.ResolvedAt
	}
	name := subjectName(incident.Subject)
	resolved := kind == KindResolved
	switch incident.Rule {
	case RuleRunFailedStreak:
		if resolved {
			out.Title = name + " runs succeed again"
		} else {
			out.Title = name + " runs are failing"
			out.Body = fmt.Sprintf("The last %d runs failed", detailInt(incident.Detail, "runs"))
			if code := detailString(incident.Detail, "error"); code != "" {
				out.Body += " (" + code + ")"
			}
			out.Body += "."
		}
	case RuleSourceStale:
		if resolved {
			out.Title = name + " is read again"
		} else {
			out.Title = name + " has not been read"
			out.Body = fmt.Sprintf("No successful read for %d hours.", detailInt(incident.Detail, "hours"))
		}
	case RuleLoginExpired:
		if resolved {
			out.Title = name + " login works again"
		} else {
			out.Title = name + " login has expired"
			out.Body = "Replace the login cookie."
		}
	case RuleUnreadableCredentials:
		if resolved {
			out.Title = "Stored " + name + " secrets are readable again"
		} else {
			out.Title = "Stored " + name + " secrets cannot be decrypted"
			out.Body = "Check LLM_SECRETS_KEY or enter the secrets again."
		}
	default:
		out.Title = incident.Rule + ": " + name
	}
	times := "Opened " + tokyoTime(incident.OpenedAt)
	if resolved && out.ResolvedAt != "" {
		times += " · resolved " + tokyoTime(out.ResolvedAt)
	}
	out.Body = strings.TrimSpace(out.Body + "\n" + times)
	return out
}

func (m message) text() string {
	text := m.Title
	if m.Body != "" {
		text += "\n" + m.Body
	}
	if m.Link != "" {
		text += "\n" + m.Link
	}
	return text
}

// sender delivers messages with the guarded client.
type sender struct {
	client *http.Client
	// telegram is the Bot API base; tests replace it.
	telegram string
}

// send delivers one message through channel and returns "" or a send error
// code.
func (s sender) send(ctx context.Context, channel Channel, out message) string {
	if channel.Unreadable {
		return codeUnreadable
	}
	if !channel.Link {
		out.Link = ""
	}
	var request *http.Request
	var err error
	switch channel.Type {
	case TypeNtfy:
		request, err = ntfyRequest(ctx, channel, out)
	case TypeTelegram:
		request, err = telegramRequest(ctx, s.telegram, channel, out)
	case TypeWebhook:
		request, err = webhookRequest(ctx, channel, out)
	default:
		return codeFailed
	}
	if err != nil {
		return codeFailed
	}
	return s.do(request)
}

// get sends the heartbeat ping.
func (s sender) get(ctx context.Context, address string) string {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return codeFailed
	}
	request.Header.Set("User-Agent", userAgent)
	return s.do(request)
}

func (s sender) do(request *http.Request) string {
	ctx, cancel := context.WithTimeout(request.Context(), sendTimeout)
	defer cancel()
	response, err := s.client.Do(request.WithContext(ctx))
	if err != nil {
		// The error wraps the request address, which may carry a token
		// (Telegram), so only its kind is kept.
		switch {
		case errors.Is(err, errRejected):
			return codeRejected
		case errors.Is(err, context.DeadlineExceeded):
			return codeTimeout
		case ctx.Err() != nil:
			return codeTimeout
		}
		return codeUnreachable
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, readLimit))
	_ = response.Body.Close()
	switch status := response.StatusCode; {
	case status >= 200 && status < 300:
		return ""
	case status == http.StatusTooManyRequests:
		return codeRateLimited
	case status >= 500:
		return codeServer
	case status >= 400:
		return codeClient
	default:
		return codeRedirect
	}
}

var ntfyPriority = map[Severity]int{SeverityInfo: 3, SeverityWarning: 4, SeverityCritical: 5}

var ntfyTags = map[string]string{KindOpened: "warning", KindResolved: "white_check_mark", KindTest: "bell"}

// ntfyRequest publishes as JSON to the server root, so titles need no header
// encoding.
func ntfyRequest(ctx context.Context, channel Channel, out message) (*http.Request, error) {
	payload := map[string]any{
		"topic": channel.Topic, "title": out.Title, "message": out.Body, "priority": ntfyPriority[out.Severity],
		"tags": []string{ntfyTags[out.Kind]},
	}
	if out.Body == "" {
		payload["message"] = out.Title
	}
	if out.Link != "" {
		payload["click"] = out.Link
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, channel.Server+"/", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", userAgent)
	if channel.Secret != "" {
		request.Header.Set("Authorization", "Bearer "+channel.Secret)
	}
	return request, nil
}

// telegramRequest calls sendMessage. The Bot API takes the token in the URL
// path; this is the documented exception to keeping tokens out of URLs, so
// the URL exists only here and is never logged, recorded or returned.
func telegramRequest(ctx context.Context, base string, channel Channel, out message) (*http.Request, error) {
	body, err := json.Marshal(map[string]any{"chat_id": channel.ChatID, "text": out.text(), "disable_web_page_preview": true})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/bot"+channel.Secret+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", userAgent)
	return request, nil
}

// WebhookPayload is the fixed JSON body of a webhook delivery.
type WebhookPayload struct {
	Version    int      `json:"version"`
	Kind       string   `json:"kind"`
	Rule       string   `json:"rule,omitempty"`
	Subject    string   `json:"subject,omitempty"`
	Severity   Severity `json:"severity"`
	Title      string   `json:"title"`
	Message    string   `json:"message"`
	OpenedAt   string   `json:"openedAt,omitempty"`
	ResolvedAt string   `json:"resolvedAt,omitempty"`
	SentAt     string   `json:"sentAt"`
	Link       string   `json:"link,omitempty"`
}

// webhookRequest posts the payload, signed with HMAC-SHA256 over the body
// when the channel has a signing key.
func webhookRequest(ctx context.Context, channel Channel, out message) (*http.Request, error) {
	body, err := json.Marshal(WebhookPayload{
		Version: 1, Kind: out.Kind, Rule: out.Rule, Subject: out.Subject, Severity: out.Severity,
		Title: out.Title, Message: out.Body, OpenedAt: out.OpenedAt, ResolvedAt: out.ResolvedAt, SentAt: out.SentAt, Link: out.Link,
	})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, channel.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("X-Kano-Event", out.Kind)
	if channel.Secret != "" {
		request.Header.Set(signatureName, Signature(channel.Secret, body))
	}
	return request, nil
}

// Signature is the X-Kano-Signature value of body: "sha256=" and the
// lowercase hex HMAC-SHA256 under key.
func Signature(key string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
