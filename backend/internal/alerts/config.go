// Package alerts tells the operator when the service needs attention. A
// fixed rule catalog is evaluated against the local database once a minute;
// a rule that holds opens an incident, and its opening and resolution are
// delivered to the operator's ntfy, Telegram and webhook channels through a
// durable outbox. A heartbeat pings an external monitor so a stopped process
// is noticed too. Settings live in app_settings; channel secrets, webhook
// addresses and the heartbeat address are sealed with LLM_SECRETS_KEY and
// never leave the process.
package alerts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/secretstore"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// ChannelType names a delivery service.
type ChannelType string

const (
	// TypeNtfy publishes to an ntfy topic, on ntfy.sh or a public self-hosted
	// server, with an optional access token.
	TypeNtfy ChannelType = "ntfy"
	// TypeTelegram sends a bot message to a chat.
	TypeTelegram ChannelType = "telegram"
	// TypeWebhook posts a fixed JSON body, signed with HMAC-SHA256 when a
	// signing key is set.
	TypeWebhook ChannelType = "webhook"
)

// ChannelTypes lists the delivery services in display order.
var ChannelTypes = []ChannelType{TypeNtfy, TypeTelegram, TypeWebhook}

const (
	// settingKey stores the settings as JSON in app_settings.
	settingKey = "alerts"
	// MaxChannels bounds the deliveries each incident queues.
	MaxChannels     = 8
	maxNameLength   = 64
	maxSecretLength = 512
	maxURLLength    = 2048
	// DefaultNtfyServer is used when an ntfy channel names no server.
	DefaultNtfyServer        = "https://ntfy.sh"
	defaultHeartbeatInterval = 5
	minHeartbeatInterval     = 1
	maxHeartbeatInterval     = 60
)

// ErrUnavailable reports stored settings that cannot be parsed. Nothing is
// delivered until an operator save replaces them.
var ErrUnavailable = errors.New("alert settings are unavailable")

// Channel is one delivery destination. Secret is plaintext and exists only
// in memory; storage seals it.
type Channel struct {
	ID      string
	Name    string
	Type    ChannelType
	Enabled bool
	// Server and Topic address an ntfy topic.
	Server string
	Topic  string
	// ChatID is the Telegram chat: a numeric id or an @channel name.
	ChatID string
	// URL is the webhook address. Like Secret it is plaintext in memory only:
	// many webhook addresses carry their token in the path.
	URL string
	// Link adds the console address to messages when an origin is known.
	Link bool
	// Secret is the ntfy access token, the Telegram bot token or the webhook
	// signing key.
	Secret string
	// Unreadable marks a stored secret or webhook address that no longer
	// decrypts, such as after LLM_SECRETS_KEY changed. The channel is skipped
	// until it is saved with them again.
	Unreadable bool
}

// usable reports whether deliveries can be sent through the channel.
func (c Channel) usable() bool {
	return c.Enabled && !c.Unreadable && (c.Type != TypeTelegram || c.Secret != "")
}

// RuleSetting is the operator's choice for one catalog rule.
type RuleSetting struct {
	Enabled  bool     `json:"enabled"`
	Severity Severity `json:"severity"`
	// Threshold is the rule's parameter (runs or hours); 0 for rules without
	// one.
	Threshold int `json:"threshold"`
	// NotifyResolved also delivers the resolution of an incident.
	NotifyResolved bool `json:"notifyResolved"`
}

// Heartbeat pings an external monitor, such as an Uptime Kuma push monitor
// or healthchecks.io, so a stopped process or host is noticed. URL is
// plaintext in memory only: push addresses carry their token.
type Heartbeat struct {
	Enabled         bool
	IntervalMinutes int
	URL             string
	Unreadable      bool
}

// Settings is the whole alert configuration.
type Settings struct {
	Channels  []Channel
	Rules     map[string]RuleSetting
	Heartbeat Heartbeat
}

type storedSettings struct {
	Channels  []storedChannel        `json:"channels"`
	Rules     map[string]RuleSetting `json:"rules"`
	Heartbeat storedHeartbeat        `json:"heartbeat"`
}

type storedChannel struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Type    ChannelType `json:"type"`
	Enabled bool        `json:"enabled"`
	Server  string      `json:"server,omitempty"`
	Topic   string      `json:"topic,omitempty"`
	ChatID  string      `json:"chatId,omitempty"`
	// LegacyURL is a webhook address stored in plaintext before addresses
	// were sealed; it is read once and sealed by the next save.
	LegacyURL    string `json:"url,omitempty"`
	SealedURL    string `json:"sealedUrl,omitempty"`
	Link         bool   `json:"link"`
	SealedSecret string `json:"secret,omitempty"`
}

type storedHeartbeat struct {
	Enabled         bool   `json:"enabled"`
	IntervalMinutes int    `json:"intervalMinutes"`
	SealedURL       string `json:"url,omitempty"`
}

var (
	idPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	topicPattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	chatIDPattern   = regexp.MustCompile(`^(-?[0-9]{1,20}|@[A-Za-z][A-Za-z0-9_]{4,31})$`)
	botTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}:[A-Za-z0-9_-]{8,128}$`)
)

// Normalize validates settings, assigns ids to new channels and fills every
// catalog rule, so a missing rule takes its default.
func Normalize(settings Settings) (Settings, error) {
	if len(settings.Channels) > MaxChannels {
		return Settings{}, httpx.Invalid("at most %d channels can be configured", MaxChannels)
	}
	out := Settings{Channels: make([]Channel, 0, len(settings.Channels)), Rules: map[string]RuleSetting{}}
	seen := map[string]bool{}
	for _, channel := range settings.Channels {
		if id := strings.ToLower(strings.TrimSpace(channel.ID)); id != "" {
			seen[id] = true
		}
	}
	taken := map[string]bool{}
	for index, channel := range settings.Channels {
		next, err := normalizeChannel(channel)
		if err != nil {
			return Settings{}, httpx.Invalid("channel %d: %s", index+1, err.Error())
		}
		if next.ID == "" {
			next.ID = newID(seen)
			seen[next.ID] = true
		}
		if taken[next.ID] {
			return Settings{}, httpx.Invalid("channel %d: duplicate id", index+1)
		}
		taken[next.ID] = true
		out.Channels = append(out.Channels, next)
	}
	for code := range settings.Rules {
		if _, ok := ruleByCode(code); !ok {
			return Settings{}, httpx.Invalid("unknown alert rule %q", code)
		}
	}
	for _, rule := range Catalog {
		setting, ok := settings.Rules[rule.Code]
		if !ok {
			out.Rules[rule.Code] = rule.defaults()
			continue
		}
		normalized, err := rule.normalize(setting)
		if err != nil {
			return Settings{}, httpx.Invalid("rule %s: %s", rule.Code, err.Error())
		}
		out.Rules[rule.Code] = normalized
	}
	heartbeat, err := normalizeHeartbeat(settings.Heartbeat)
	if err != nil {
		return Settings{}, httpx.Invalid("heartbeat: %s", err.Error())
	}
	out.Heartbeat = heartbeat
	return out, nil
}

type inputError string

func (e inputError) Error() string { return string(e) }

func normalizeChannel(channel Channel) (Channel, error) {
	channel.ID = strings.ToLower(strings.TrimSpace(channel.ID))
	if channel.ID != "" && !idPattern.MatchString(channel.ID) {
		return Channel{}, inputError("id must use lowercase letters, digits, hyphens or underscores")
	}
	channel.Name = strings.TrimSpace(channel.Name)
	if utf8.RuneCountInString(channel.Name) > maxNameLength || hasControl(channel.Name) {
		return Channel{}, inputError("name must be at most 64 characters")
	}
	if len(channel.Secret) > maxSecretLength || hasControl(channel.Secret) {
		return Channel{}, inputError("secret must be at most 512 characters")
	}
	channel.Secret = strings.TrimSpace(channel.Secret)
	if channel.Secret != "" {
		channel.Unreadable = false
	}
	switch channel.Type {
	case TypeNtfy:
		server := strings.TrimSpace(channel.Server)
		if server == "" {
			server = DefaultNtfyServer
		}
		// Checked on the input: validateURL drops a fragment.
		if strings.ContainsAny(server, "?#") {
			return Channel{}, inputError("ntfy server must not have a query or fragment")
		}
		parsed, err := validateURL(server, false)
		if err != nil {
			return Channel{}, inputError("ntfy server: " + err.Error())
		}
		channel.Server = strings.TrimSuffix(parsed.String(), "/")
		channel.Topic = strings.TrimSpace(channel.Topic)
		if !topicPattern.MatchString(channel.Topic) {
			return Channel{}, inputError("ntfy topic must be 1-64 letters, digits, hyphens or underscores")
		}
		channel.ChatID, channel.URL = "", ""
	case TypeTelegram:
		channel.ChatID = strings.TrimSpace(channel.ChatID)
		if !chatIDPattern.MatchString(channel.ChatID) {
			return Channel{}, inputError("chat must be a numeric chat id or an @channel name")
		}
		if channel.Secret != "" && !botTokenPattern.MatchString(channel.Secret) {
			return Channel{}, inputError("bot token must be the bot id, a colon and its secret")
		}
		if channel.Secret == "" && !channel.Unreadable {
			return Channel{}, inputError("bot token is required")
		}
		channel.Server, channel.Topic, channel.URL = "", "", ""
	case TypeWebhook:
		channel.Server, channel.Topic, channel.ChatID = "", "", ""
		channel.URL = strings.TrimSpace(channel.URL)
		if channel.URL == "" && channel.Unreadable {
			// The sealed address is kept until it is entered again.
			break
		}
		parsed, err := validateURL(channel.URL, false)
		if err != nil {
			return Channel{}, inputError("webhook address: " + err.Error())
		}
		channel.URL = parsed.String()
	default:
		return Channel{}, inputError("type must be ntfy, telegram or webhook")
	}
	return channel, nil
}

func normalizeHeartbeat(heartbeat Heartbeat) (Heartbeat, error) {
	if heartbeat.IntervalMinutes == 0 {
		heartbeat.IntervalMinutes = defaultHeartbeatInterval
	}
	if heartbeat.IntervalMinutes < minHeartbeatInterval || heartbeat.IntervalMinutes > maxHeartbeatInterval {
		return Heartbeat{}, inputError("interval must be between 1 and 60 minutes")
	}
	heartbeat.URL = strings.TrimSpace(heartbeat.URL)
	if heartbeat.URL != "" {
		// A push address carries its token in the path or query; it is
		// sealed, so only embedded user credentials are refused.
		parsed, err := validateURL(heartbeat.URL, true)
		if err != nil {
			return Heartbeat{}, inputError("address: " + err.Error())
		}
		heartbeat.URL, heartbeat.Unreadable = parsed.String(), false
	}
	if heartbeat.Enabled && heartbeat.URL == "" && !heartbeat.Unreadable {
		return Heartbeat{}, inputError("address is required")
	}
	return heartbeat, nil
}

// sameDestination reports whether a saved secret may be kept: it is only
// ever sent to the destination it was entered for. Telegram's host is fixed.
func sameDestination(saved, next Channel) bool {
	return saved.Type == next.Type && destination(saved) == destination(next)
}

func destination(channel Channel) string {
	switch channel.Type {
	case TypeNtfy:
		server := strings.TrimSpace(channel.Server)
		if server == "" {
			server = DefaultNtfyServer
		}
		return strings.ToLower(strings.TrimSuffix(server, "/"))
	case TypeWebhook:
		raw := strings.TrimSpace(channel.URL)
		if parsed, err := validateURL(raw, false); err == nil {
			return parsed.String()
		}
		return raw
	}
	return ""
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// load reads the stored settings. No row is the default configuration; a
// secret that no longer decrypts marks its channel or the heartbeat
// unreadable.
func load(ctx context.Context, db queryer, key string) (Settings, error) {
	stored, found, err := readStored(ctx, db)
	if err != nil {
		return Settings{}, err
	}
	if !found {
		return Normalize(Settings{})
	}
	settings := Settings{Rules: stored.Rules, Channels: make([]Channel, 0, len(stored.Channels))}
	for _, item := range stored.Channels {
		channel := Channel{ID: item.ID, Name: item.Name, Type: item.Type, Enabled: item.Enabled, Server: item.Server, Topic: item.Topic, ChatID: item.ChatID, URL: item.LegacyURL, Link: item.Link}
		if item.SealedSecret != "" {
			plain, decryptErr := secretstore.Decrypt(item.SealedSecret, key)
			channel.Secret, channel.Unreadable = plain, decryptErr != nil
		}
		if item.SealedURL != "" {
			plain, decryptErr := secretstore.Decrypt(item.SealedURL, key)
			channel.URL = plain
			channel.Unreadable = channel.Unreadable || decryptErr != nil
		}
		settings.Channels = append(settings.Channels, channel)
	}
	settings.Heartbeat = Heartbeat{Enabled: stored.Heartbeat.Enabled, IntervalMinutes: stored.Heartbeat.IntervalMinutes}
	if stored.Heartbeat.SealedURL != "" {
		plain, decryptErr := secretstore.Decrypt(stored.Heartbeat.SealedURL, key)
		settings.Heartbeat.URL, settings.Heartbeat.Unreadable = plain, decryptErr != nil
	}
	normalized, err := Normalize(settings)
	if err != nil {
		return Settings{}, ErrUnavailable
	}
	return normalized, nil
}

func readStored(ctx context.Context, db queryer) (storedSettings, bool, error) {
	var raw string
	err := db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, settingKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return storedSettings{}, false, nil
	}
	if err != nil {
		return storedSettings{}, false, err
	}
	var stored storedSettings
	if json.Unmarshal([]byte(raw), &stored) != nil {
		return storedSettings{}, false, ErrUnavailable
	}
	return stored, true, nil
}

// save writes normalized settings in tx. An unreadable secret is kept sealed
// as it was, so a later key change back can recover it.
func save(ctx context.Context, tx *sql.Tx, settings Settings, key string, now time.Time) error {
	previous, _, err := readStored(ctx, tx)
	if err != nil && !errors.Is(err, ErrUnavailable) {
		return err
	}
	sealed := map[string]storedChannel{}
	for _, channel := range previous.Channels {
		sealed[channel.ID] = channel
	}
	stored := storedSettings{Rules: settings.Rules, Channels: make([]storedChannel, 0, len(settings.Channels))}
	for _, channel := range settings.Channels {
		item := storedChannel{ID: channel.ID, Name: channel.Name, Type: channel.Type, Enabled: channel.Enabled, Server: channel.Server, Topic: channel.Topic, ChatID: channel.ChatID, Link: channel.Link}
		switch {
		case channel.URL != "":
			sealedURL, err := secretstore.Encrypt(channel.URL, key)
			if err != nil {
				return err
			}
			item.SealedURL = sealedURL
		case channel.Unreadable && channel.Type == TypeWebhook:
			item.SealedURL = sealed[channel.ID].SealedURL
		}
		switch {
		case channel.Unreadable:
			item.SealedSecret = sealed[channel.ID].SealedSecret
		case channel.Secret != "":
			sealed, err := secretstore.Encrypt(channel.Secret, key)
			if err != nil {
				return err
			}
			item.SealedSecret = sealed
		}
		stored.Channels = append(stored.Channels, item)
	}
	stored.Heartbeat = storedHeartbeat{Enabled: settings.Heartbeat.Enabled, IntervalMinutes: settings.Heartbeat.IntervalMinutes}
	switch {
	case settings.Heartbeat.Unreadable:
		stored.Heartbeat.SealedURL = previous.Heartbeat.SealedURL
	case settings.Heartbeat.URL != "":
		sealed, err := secretstore.Encrypt(settings.Heartbeat.URL, key)
		if err != nil {
			return err
		}
		stored.Heartbeat.SealedURL = sealed
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app_settings(key,value,updated_at) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, settingKey, string(encoded), sqlite.ISOTime(now))
	return err
}

func newID(taken map[string]bool) string {
	for {
		buffer := make([]byte, 4)
		if _, err := rand.Read(buffer); err != nil {
			panic("alert channel id entropy unavailable")
		}
		id := "c" + hex.EncodeToString(buffer)
		if !taken[id] {
			return id
		}
	}
}

func hasControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}
