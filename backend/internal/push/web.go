package push

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// ErrWebPushGone reports a subscription the push service no longer knows: the
// browser was uninstalled, its data cleared, or the endpoint rotated. The row
// is retired rather than retried.
var ErrWebPushGone = errors.New("web push subscription gone")

// DefaultWebPushIcon is the site avatar, shown beside every notification. A
// caller may override it with WEB_PUSH_ICON, which must be a root-relative
// path or an absolute URL.
const DefaultWebPushIcon = "/assets/kano-avatar.jpg"

// defaultWebPushHosts are the push services a browser may hand us. The server
// posts to whatever endpoint it stored, so this list is the SSRF boundary.
var defaultWebPushHosts = []string{
	"fcm.googleapis.com",
	"*.push.services.mozilla.com",
	"*.push.apple.com",
	"*.notify.windows.com",
	"*.push.samsungosp.com",
}

// WebOptions configures the Web Push sender. Empty keys leave the feature off,
// which is how a deployment without HTTPS keeps it hidden.
type WebOptions struct {
	PublicKey  string
	PrivateKey string
	// Subject is the VAPID contact, a mailto: or https: URL the push service
	// may use to reach the operator about this sender.
	Subject string
	TTL     time.Duration
	// Icon is the avatar shown in the notification; a relative path is
	// resolved against Origin.
	Icon string
	// Origin is the public site origin, used to make Icon and an update's
	// relative URL absolute.
	Origin string
	// Hosts replaces the default push-service allowlist.
	Hosts []string
	// HTTPClient is the transport; nil uses a client with a hard timeout.
	HTTPClient *http.Client
}

// WebSender delivers notifications to browsers that subscribed through the
// site. A nil *WebSender is usable and reports disabled, mirroring Client.
type WebSender struct {
	public  string
	private string
	subject string
	icon    string
	origin  string
	ttl     int
	hosts   []string
	client  *http.Client
}

// NewWebSender validates the VAPID pair and returns nil when the feature is
// off. A configured but malformed pair is an error, so a typo fails startup
// instead of every delivery.
func NewWebSender(options WebOptions) (*WebSender, error) {
	public := strings.TrimSpace(options.PublicKey)
	private := strings.TrimSpace(options.PrivateKey)
	if public == "" || private == "" {
		return nil, nil
	}
	if raw, err := base64.RawURLEncoding.DecodeString(public); err != nil || len(raw) != 65 {
		return nil, fmt.Errorf("web push: VAPID public key must be 65 base64url bytes")
	}
	if raw, err := base64.RawURLEncoding.DecodeString(private); err != nil || len(raw) != 32 {
		return nil, fmt.Errorf("web push: VAPID private key must be 32 base64url bytes")
	}
	subject := strings.TrimSpace(options.Subject)
	if subject == "" {
		subject = "mailto:webpush@localhost"
	}
	ttl := int(options.TTL / time.Second)
	if ttl <= 0 {
		ttl = int((24 * time.Hour) / time.Second)
	}
	hosts := options.Hosts
	if len(hosts) == 0 {
		hosts = defaultWebPushHosts
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	icon := strings.TrimSpace(options.Icon)
	if icon == "" {
		icon = DefaultWebPushIcon
	}
	origin := strings.TrimRight(strings.TrimSpace(options.Origin), "/")
	if !strings.HasPrefix(icon, "http://") && !strings.HasPrefix(icon, "https://") {
		if !strings.HasPrefix(icon, "/") {
			icon = "/" + icon
		}
		icon = origin + icon
	}
	return &WebSender{
		public:  public,
		private: private,
		subject: subject,
		icon:    icon,
		origin:  origin,
		ttl:     ttl,
		hosts:   hosts,
		client:  client,
	}, nil
}

// Enabled reports whether a key pair is loaded.
func (s *WebSender) Enabled() bool { return s != nil && s.public != "" }

// PublicKey is the applicationServerKey the browser subscribes with.
func (s *WebSender) PublicKey() string {
	if s == nil {
		return ""
	}
	return s.public
}

// AllowsEndpoint reports whether raw is an https URL on a known push service.
// An unchecked endpoint would let anyone point the server at an internal host.
func (s *WebSender) AllowsEndpoint(raw string) bool {
	if s == nil {
		return false
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, allowed := range s.hosts {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		if allowed == "" {
			continue
		}
		if strings.HasPrefix(allowed, "*.") {
			if strings.HasSuffix(host, allowed[1:]) && host != allowed[2:] {
				return true
			}
			continue
		}
		if host == allowed {
			return true
		}
	}
	return false
}

// WebPayload is what a browser shows: a title, one line of body, the page to
// open and the avatar. The App's Notification carries a route instead, which a
// service worker cannot use.
type WebPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
	Tag   string `json:"tag,omitempty"`
	Icon  string `json:"icon,omitempty"`
}

// Send encrypts and posts one payload to one subscription. The payload is
// capped by the push service at 4 KB, which a title and one line never reach.
func (s *WebSender) Send(ctx context.Context, sub sqlite.WebPushSubscription, payload WebPayload) error {
	if !s.Enabled() {
		return ErrDisabled
	}
	payload.Icon = s.icon
	if payload.URL != "" && strings.HasPrefix(payload.URL, "/") {
		payload.URL = s.origin + payload.URL
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	options := &webpush.Options{
		Subscriber:      s.subject,
		VAPIDPublicKey:  s.public,
		VAPIDPrivateKey: s.private,
		TTL:             s.ttl,
		Urgency:         webpush.UrgencyNormal,
		HTTPClient:      s.client,
	}
	if payload.Tag != "" {
		options.Topic = payload.Tag
	}
	if payload.Tag == string(CategoryLive) {
		options.Urgency = webpush.UrgencyHigh
	}
	response, err := webpush.SendNotificationWithContext(ctx, body, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256DH, Auth: sub.Auth},
	}, options)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return nil
	case response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone:
		return ErrWebPushGone
	default:
		return fmt.Errorf("web push: push service answered %d", response.StatusCode)
	}
}

// TestWebPayload is the console's test message for browsers. It mirrors
// TestNotification's wording, with the site avatar and no target page.
func TestWebPayload(locale string) WebPayload {
	title, body := testWording(normalizeLocale(locale))
	return WebPayload{Title: title, Body: body, Tag: "test", Icon: DefaultWebPushIcon}
}

// acceptsWeb applies one subscription's master switch and category switch.
func acceptsWeb(sub sqlite.WebPushSubscription, category Category) bool {
	if !sub.Enabled {
		return false
	}
	switch category {
	case CategoryLive:
		return sub.NotifyLive
	case CategoryVideo:
		return sub.NotifyVideo
	default:
		return sub.NotifyPost
	}
}
