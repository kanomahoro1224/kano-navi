// Package push delivers update notifications to HarmonyOS devices through
// Huawei Push Kit.
//
// It speaks the v3 REST API: HarmonyOS 5 and later reject the OAuth 2.0 client
// credentials the older endpoints accepted, so every request is authorized
// with a PS256 JWT signed by an AGC service account private key instead.
package push

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// ProjectBase is the v3 endpoint root; the project id sits in the path.
	ProjectBase = "https://push-api.cloud.huawei.com/v3/"
	// DefaultTokenURI issues the access token a signed assertion buys.
	DefaultTokenURI = "https://oauth-login.cloud.huawei.com/oauth2/v3/token"
	// MaxTokensPerSend is the API's per-request limit; larger audiences are
	// split across requests.
	MaxTokensPerSend = 1000
	// tokenLifetime is the assertion lifetime Huawei documents; the access
	// token it returns lives an hour.
	tokenLifetime = time.Hour
	// refreshMargin renews the cached access token before it expires so a
	// send never races the deadline.
	refreshMargin = 5 * time.Minute
)

// ErrDisabled reports a client created without credentials. Callers treat it
// as "push is off", never as a delivery failure.
var ErrDisabled = errors.New("push disabled")

// ServiceAccount is the private.json AGC downloads when a service account key
// is created. Only these four fields matter for signing; the rest describe
// endpoints this client does not call.
type ServiceAccount struct {
	ProjectID  string `json:"project_id"`
	KeyID      string `json:"key_id"`
	PrivateKey string `json:"private_key"`
	SubAccount string `json:"sub_account"`
	TokenURI   string `json:"token_uri"`
}

// Notification is one update ready to deliver. Title and Body are already in
// the recipient's language: the sender picks the wording, this package only
// carries it.
type Notification struct {
	// Title is the bold first line of the shade entry.
	Title string
	// Body is the second line; keep it to one sentence.
	Body string
	// Route is the App screen the tap opens ("feed" or "recent").
	Route string
	// URL is the original post, handed to the App as a fallback target.
	URL string
	// Category picks the Huawei message class. An app without the matching
	// self-classification entitlement has every message demoted to MARKETING,
	// which is silent and capped per device per day.
	Category string
	// Notify is the shade entry id. Entries sharing an id replace each other,
	// which keeps one creator from stacking up a wall of notifications.
	Notify int
}

// Client sends notifications. A nil *Client is usable and reports ErrDisabled,
// which keeps the "not configured" path free of branches at every call site.
type Client struct {
	account  ServiceAccount
	key      *rsa.PrivateKey
	category string
	test     bool
	http     *http.Client
	now      func() time.Time

	mutex     sync.Mutex
	token     string
	expiresAt time.Time
}

// Options configures a client. An empty account leaves push disabled.
type Options struct {
	Account ServiceAccount
	// Category overrides the per-notification default; SUBSCRIPTION suits
	// creator update alerts.
	Category string
	// Test marks every send as a test message, which bypasses the frequency
	// cap during development. Production must leave it off.
	Test   bool
	Client *http.Client
	Now    func() time.Time
}

// New validates the account and parses the private key. It returns nil, nil
// when push is not configured, so the caller can keep the feature optional.
func New(options Options) (*Client, error) {
	account := options.Account
	account.ProjectID = strings.TrimSpace(account.ProjectID)
	account.KeyID = strings.TrimSpace(account.KeyID)
	account.SubAccount = strings.TrimSpace(account.SubAccount)
	if account.ProjectID == "" || account.KeyID == "" || account.SubAccount == "" || strings.TrimSpace(account.PrivateKey) == "" {
		return nil, nil
	}
	key, err := parsePrivateKey(account.PrivateKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(account.TokenURI) == "" {
		account.TokenURI = DefaultTokenURI
	}
	category := strings.TrimSpace(options.Category)
	if category == "" {
		category = "SUBSCRIPTION"
	}
	client := options.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Client{
		account:  account,
		key:      key,
		category: category,
		test:     options.Test,
		http:     client,
		now:      now,
	}, nil
}

// LoadServiceAccount reads a private.json file. A blank path means push is not
// configured; a missing or malformed file is a configuration error the process
// should surface at startup rather than at the first update.
func LoadServiceAccount(path string) (ServiceAccount, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return ServiceAccount{}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ServiceAccount{}, fmt.Errorf("read service account: %w", err)
	}
	var account ServiceAccount
	if err = json.Unmarshal(content, &account); err != nil {
		return ServiceAccount{}, fmt.Errorf("parse service account: %w", err)
	}
	return account, nil
}

// Enabled reports whether this client can deliver anything.
func (c *Client) Enabled() bool { return c != nil && c.key != nil }

// Send delivers one notification to every token and returns the tokens Huawei
// rejected as invalid so the caller can retire them. Transport failures are
// returned as errors; per-token rejections are not.
func (c *Client) Send(ctx context.Context, notification Notification, tokens []string) ([]string, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	tokens = compactTokens(tokens)
	if len(tokens) == 0 {
		return nil, nil
	}
	category := strings.TrimSpace(notification.Category)
	if category == "" {
		category = c.category
	}
	var stale []string
	for start := 0; start < len(tokens); start += MaxTokensPerSend {
		end := start + MaxTokensPerSend
		if end > len(tokens) {
			end = len(tokens)
		}
		rejected, err := c.sendBatch(ctx, notification, category, tokens[start:end])
		if errors.Is(err, errAllTokensInvalid) {
			// Huawei refused the whole batch as unregistered; every token in it
			// is dead, not just one.
			stale = append(stale, tokens[start:end]...)
			continue
		}
		stale = append(stale, rejected...)
		if err != nil {
			return stale, err
		}
	}
	return stale, nil
}

func (c *Client) sendBatch(ctx context.Context, notification Notification, category string, tokens []string) ([]string, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"payload": map[string]any{
			"notification": map[string]any{
				"category": category,
				"title":    clip(notification.Title, 100),
				"body":     clip(notification.Body, 300),
				// actionType 0 opens the app; the data map rides along in the
				// want, which is how the App learns which screen to show.
				"clickAction": map[string]any{
					"actionType": 0,
					"data": map[string]string{
						"route": clip(notification.Route, 32),
						"url":   clip(notification.URL, 512),
					},
				},
				// A foreground device already sees the update in the App; a
				// banner on top of that would be noise.
				"foregroundShow": false,
				"notifyId":       notification.Notify,
			},
		},
		"target": map[string]any{"token": tokens},
		"pushOptions": map[string]any{
			"ttl":         86400,
			"testMessage": c.test,
		},
	})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, ProjectBase+c.account.ProjectID+"/messages:send", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("push-type", "0")
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("push send: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("push send: %w", err)
	}
	return rejectedTokens(response.StatusCode, body)
}

// rejectedTokens interprets a send response. Huawei answers 200 with a code
// per request; a bad token is reported as that code rather than an HTTP error.
func rejectedTokens(status int, body []byte) ([]string, error) {
	var result struct {
		Code      string `json:"code"`
		Message   string `json:"msg"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		if status != http.StatusOK {
			return nil, fmt.Errorf("push send: HTTP %d", status)
		}
		return nil, errors.New("push send: unreadable response")
	}
	switch result.Code {
	case "80000000", "80000001":
		// Accepted. Per-token rejections arrive at the receipt server, not
		// here; this endpoint reports one code for the batch.
		return nil, nil
	case "80300007":
		return nil, errAllTokensInvalid
	}
	return nil, fmt.Errorf("push send: %s %s", result.Code, result.Message)
}

// errAllTokensInvalid marks a batch Huawei refused because none of its tokens
// is registered any more. Send rewrites it into a full rejection.
var errAllTokensInvalid = errors.New("push tokens are not registered")

// accessToken returns a cached token, minting a new one when the cached value
// is near expiry. The assertion is signed per request, so a cold start needs
// exactly one network round trip before the first send.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	now := c.now()
	if c.token != "" && now.Before(c.expiresAt.Add(-refreshMargin)) {
		return c.token, nil
	}
	assertion, err := c.assertion(now)
	if err != nil {
		return "", err
	}
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.account.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.http.Do(request)
	if err != nil {
		return "", fmt.Errorf("push token: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("push token: %w", err)
	}
	var issued struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err = json.Unmarshal(body, &issued); err != nil {
		return "", fmt.Errorf("push token: HTTP %d", response.StatusCode)
	}
	if issued.AccessToken == "" {
		return "", fmt.Errorf("push token: %s %s", issued.Error, issued.Description)
	}
	lifetime := tokenLifetime
	if issued.ExpiresIn > 0 {
		lifetime = time.Duration(issued.ExpiresIn) * time.Second
	}
	c.token = issued.AccessToken
	c.expiresAt = now.Add(lifetime)
	return c.token, nil
}

// assertion builds the PS256-signed JWT the token endpoint accepts as proof of
// the service account's identity.
func (c *Client) assertion(now time.Time) (string, error) {
	header, err := json.Marshal(map[string]string{"kid": c.account.KeyID, "typ": "JWT", "alg": "PS256"})
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]any{
		"aud": DefaultTokenURI,
		"iss": c.account.SubAccount,
		"iat": now.Unix(),
		"exp": now.Add(tokenLifetime).Unix(),
	})
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPSS(rand.Reader, c.key, crypto.SHA256, digest[:], &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash,
		Hash:       crypto.SHA256,
	})
	if err != nil {
		return "", fmt.Errorf("push assertion: %w", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// parsePrivateKey accepts the PKCS#8 PEM AGC issues.
func parsePrivateKey(raw string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(raw)))
	if block == nil {
		return nil, errors.New("push private key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("push private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("push private key is not RSA")
	}
	return key, nil
}

// compactTokens drops blanks and duplicates while preserving order.
func compactTokens(tokens []string) []string {
	seen := make(map[string]bool, len(tokens))
	compacted := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" || seen[token] {
			continue
		}
		seen[token] = true
		compacted = append(compacted, token)
	}
	return compacted
}

// clip bounds a field by bytes; Huawei caps the whole message at 4 KiB.
func clip(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	cut := limit
	for cut > 0 && !utf8Start(value[cut]) {
		cut--
	}
	return value[:cut]
}

// utf8Start reports whether b begins a UTF-8 sequence, so clip never splits a
// rune in half.
func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
