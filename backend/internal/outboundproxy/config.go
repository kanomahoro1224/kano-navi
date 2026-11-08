// Package outboundproxy holds the operator's forward proxies for outbound
// requests: one ordered proxy queue, the areas (X, YouTube, Bilibili, the
// shop, media and model providers) that route through it, and the transport that tries
// an area's proxies one at a time. Settings live in app_settings; proxy
// passwords are sealed with LLM_SECRETS_KEY and never leave the process.
package outboundproxy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yexca/kano-navi/backend/internal/httpx"
)

// Area names a group of outbound requests that share one proxy route.
type Area string

const (
	// AreaX covers the X profile, FxTwitter, vxtwitter and X API reads.
	AreaX Area = "x"
	// AreaYoutube covers YouTube pages, feeds, the Data API and WebSub
	// subscriptions.
	AreaYoutube Area = "youtube"
	// AreaBilibili covers the Bilibili API.
	AreaBilibili Area = "bilibili"
	// AreaShop covers the official shop's product JSON.
	AreaShop Area = "shop"
	// AreaMedia covers post, video and profile media downloads.
	AreaMedia Area = "media"
	// AreaLLM covers model provider requests: extraction, translation, tests
	// and model discovery.
	AreaLLM Area = "llm"
)

// Areas lists every area in display order.
var Areas = []Area{AreaX, AreaYoutube, AreaBilibili, AreaShop, AreaMedia, AreaLLM}

const (
	// MaxProxies bounds the queue and therefore the attempts per request.
	MaxProxies        = 16
	maxNameLength     = 64
	maxCredentialSize = 255
)

// Proxy is one forward proxy. Password is plaintext and exists only in
// memory; storage seals it.
type Proxy struct {
	ID       string
	Name     string
	Scheme   string
	Host     string
	Port     int
	Username string
	Password string
	Enabled  bool
	// Unreadable marks a stored password that no longer decrypts, such as
	// after LLM_SECRETS_KEY changed. The proxy is skipped until it is saved
	// with a new password.
	Unreadable bool
}

// Route turns the proxy queue on for an area. An empty ProxyIDs selects every
// proxy; either way proxies are tried in queue order.
type Route struct {
	Enabled  bool     `json:"enabled"`
	ProxyIDs []string `json:"proxyIds"`
}

// Config is the whole proxy setting.
type Config struct {
	// Proxies is the queue, highest priority first.
	Proxies []Proxy
	Routes  map[Area]Route
	// DirectFallback retries a request directly after every proxy of its
	// route failed to answer.
	DirectFallback bool
}

var (
	idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	schemes   = map[string]bool{"http": true, "https": true, "socks5": true, "socks5h": true}
)

func validArea(area Area) bool {
	for _, known := range Areas {
		if area == known {
			return true
		}
	}
	return false
}

// Normalize validates a configuration, assigns ids to new proxies and
// canonicalizes routes: unknown areas are rejected, every area is present,
// and selections retain stale proxy ids so an explicit selection cannot turn
// into the empty, all-proxies selection after a proxy is removed.
func Normalize(config Config) (Config, error) {
	if len(config.Proxies) > MaxProxies {
		return Config{}, httpx.Invalid("at most %d proxies can be configured", MaxProxies)
	}
	normalized := Config{Proxies: make([]Proxy, 0, len(config.Proxies)), Routes: map[Area]Route{}, DirectFallback: config.DirectFallback}
	seen := map[string]bool{}
	for _, proxy := range config.Proxies {
		proxy.ID = strings.ToLower(strings.TrimSpace(proxy.ID))
		if proxy.ID != "" {
			seen[proxy.ID] = true
		}
	}
	taken := map[string]bool{}
	for index, proxy := range config.Proxies {
		next, err := normalizeProxy(proxy)
		if err != nil {
			return Config{}, httpx.Invalid("proxy %d: %s", index+1, err.Error())
		}
		if next.ID == "" {
			next.ID = newID(seen)
			seen[next.ID] = true
		}
		if taken[next.ID] {
			return Config{}, httpx.Invalid("proxy %d: duplicate id", index+1)
		}
		taken[next.ID] = true
		normalized.Proxies = append(normalized.Proxies, next)
	}
	for area, route := range config.Routes {
		if !validArea(area) {
			return Config{}, httpx.Invalid("unknown proxy area %q", string(area))
		}
		if route.Enabled && len(normalized.Proxies) == 0 {
			return Config{}, httpx.Invalid("add a proxy before routing %s through it", area)
		}
		normalized.Routes[area] = Route{Enabled: route.Enabled, ProxyIDs: normalized.selection(route.ProxyIDs)}
	}
	for _, area := range Areas {
		if _, ok := normalized.Routes[area]; !ok {
			normalized.Routes[area] = Route{ProxyIDs: []string{}}
		}
	}
	return normalized, nil
}

type inputError string

func (e inputError) Error() string { return string(e) }

func normalizeProxy(proxy Proxy) (Proxy, error) {
	proxy.ID = strings.ToLower(strings.TrimSpace(proxy.ID))
	if proxy.ID != "" && !idPattern.MatchString(proxy.ID) {
		return Proxy{}, inputError("id must use lowercase letters, digits, hyphens or underscores")
	}
	proxy.Name = strings.TrimSpace(proxy.Name)
	if utf8.RuneCountInString(proxy.Name) > maxNameLength || hasControl(proxy.Name) {
		return Proxy{}, inputError("name must be at most " + strconv.Itoa(maxNameLength) + " characters")
	}
	proxy.Scheme = strings.ToLower(strings.TrimSpace(proxy.Scheme))
	if !schemes[proxy.Scheme] {
		return Proxy{}, inputError("protocol must be http, https, socks5 or socks5h")
	}
	if proxy.Port < 1 || proxy.Port > 65535 {
		return Proxy{}, inputError("port must be between 1 and 65535")
	}
	host := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(proxy.Host), "["), "]")
	if host == "" || strings.ContainsAny(host, "/@?#[] ") || hasControl(host) || len(host) > 253 {
		return Proxy{}, inputError("address must be a hostname or IP address")
	}
	if strings.Contains(host, ":") && net.ParseIP(host) == nil {
		return Proxy{}, inputError("address must be a hostname or IP address")
	}
	proxy.Host = strings.ToLower(host)
	if len(proxy.Username) > maxCredentialSize || hasControl(proxy.Username) {
		return Proxy{}, inputError("username must be at most " + strconv.Itoa(maxCredentialSize) + " characters")
	}
	if len(proxy.Password) > maxCredentialSize || hasControl(proxy.Password) {
		return Proxy{}, inputError("password must be at most " + strconv.Itoa(maxCredentialSize) + " characters")
	}
	if proxy.Username == "" {
		proxy.Password = ""
		proxy.Unreadable = false
	}
	return proxy, nil
}

// selection normalizes and de-duplicates ids. Known ids are returned in queue
// order, followed by stale ids. An empty result still means every proxy, so a
// non-empty stale selection must remain non-empty.
func (config Config) selection(ids []string) []string {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if id != "" {
			wanted[id] = true
		}
	}
	selected := []string{}
	for _, proxy := range config.Proxies {
		if wanted[proxy.ID] {
			selected = append(selected, proxy.ID)
			delete(wanted, proxy.ID)
		}
	}
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if id != "" && wanted[id] {
			selected = append(selected, id)
			delete(wanted, id)
		}
	}
	return selected
}

// Resolve reports whether area routes through proxies and, if so, the
// usable proxies in queue order: enabled, selected and with a readable
// password. A routed area with no usable proxy has an empty list.
func (config Config) Resolve(area Area) (bool, []Proxy) {
	route, ok := config.Routes[area]
	if !ok || !route.Enabled {
		return false, nil
	}
	wanted := make(map[string]bool, len(route.ProxyIDs))
	for _, id := range route.ProxyIDs {
		wanted[id] = true
	}
	usable := []Proxy{}
	for _, proxy := range config.Proxies {
		if !proxy.Enabled || proxy.Unreadable || len(wanted) > 0 && !wanted[proxy.ID] {
			continue
		}
		usable = append(usable, proxy)
	}
	return true, usable
}

// URL builds the proxy URL, including credentials, for an http.Transport.
func (proxy Proxy) URL() *url.URL {
	u := &url.URL{Scheme: proxy.Scheme, Host: net.JoinHostPort(proxy.Host, strconv.Itoa(proxy.Port))}
	if proxy.Username != "" {
		u.User = url.UserPassword(proxy.Username, proxy.Password)
	}
	return u
}

// fingerprint identifies the endpoint and credentials without keeping them
// readable, so a changed proxy gets a fresh transport and health record.
func (proxy Proxy) fingerprint() string {
	sum := sha256.Sum256([]byte(proxy.URL().String()))
	return hex.EncodeToString(sum[:])
}

func newID(taken map[string]bool) string {
	for {
		buffer := make([]byte, 4)
		if _, err := rand.Read(buffer); err != nil {
			panic("proxy id entropy unavailable")
		}
		id := "p" + hex.EncodeToString(buffer)
		if !taken[id] {
			return id
		}
	}
}

func hasControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}
