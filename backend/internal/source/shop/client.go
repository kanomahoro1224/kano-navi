// Package shop reads the sale periods of the official shop, a Shopify
// storefront, from its public product JSON and its catalog's Atom feed.
//
// The client requests four fixed paths on one host and nothing else: the
// newest products of the new-arrivals collection, the whole catalog a page at
// a time, the catalog's Atom feed, and the collection list. No URL comes from
// data. Every request names the project honestly as kano-navi-go/shop; the
// shop rate-limits clients that pose as a browser. With a Web Bot Auth key a
// request may also be signed (internal/webbotauth).
package shop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yexca/kano-navi/backend/internal/sourcecapture"
	"github.com/yexca/kano-navi/backend/internal/webbotauth"
)

// Host is the shop's storefront, the only host the client requests.
const Host = "shop.milpr.com"

// UserAgent names this client. It is never a browser's.
const UserAgent = "kano-navi-go/shop"

// MaxResponseBytes bounds one response. The newest 50 products are about
// 300 KB and a catalog page of 250 about 2 MB.
const MaxResponseBytes = 4 * 1024 * 1024

// RequestTimeout bounds one request, including reading its body.
const RequestTimeout = 10 * time.Second

const (
	// ArrivalsLimit is how many of the newest products a timed read lists.
	ArrivalsLimit = 50
	// CatalogPageSize is the largest page Shopify serves.
	CatalogPageSize = 250
	// MaxCatalogPages bounds a full read; the catalog had four pages when
	// this source was added.
	MaxCatalogPages  = 6
	collectionsLimit = 250
)

// Retry-After bounds: a missing or unreadable value waits a minute, and no
// answer holds the shop back for more than a day.
const (
	defaultRetryAfter = time.Minute
	maxRetryAfter     = 24 * time.Hour
)

// Error is a controlled failure: Code is what the run records. RetryAfter is
// set on a 429 answer.
type Error struct {
	Code       string
	RetryAfter time.Duration
}

func (e *Error) Error() string { return e.Code }

// ErrNoSigningKey is the signed method's outcome without a Web Bot Auth key;
// it makes no request.
const ErrNoSigningKey = "shop_no_signing_key"

// ErrorCode is the controlled code of a client or adapter error.
func ErrorCode(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return "shop_unavailable"
}

type Client struct {
	http *http.Client
	// now reads the clock for an HTTP-date Retry-After.
	now func() time.Time
	// Signer signs the requests a read asks to sign; nil leaves every
	// request unsigned and the signed method unavailable.
	Signer *webbotauth.Signer
}

// Signs reports whether a Web Bot Auth key is configured.
func (c *Client) Signs() bool { return c.Signer != nil }

// NewClient wraps an HTTP client (for its transport) with the request
// timeout and redirect rejection.
func NewClient(client *http.Client) *Client {
	if client == nil {
		client = &http.Client{}
	}
	copy := *client
	copy.Timeout = RequestTimeout
	copy.Transport = sourcecapture.NewTransport("shop", copy.Transport)
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{http: &copy, now: time.Now}
}

// NewArrivals lists the newest products, newest first, signed when sign is
// set.
func (c *Client) NewArrivals(ctx context.Context, limit int, sign bool) ([]Product, error) {
	limit = min(max(limit, 1), CatalogPageSize)
	body, err := c.get(ctx, "/collections/new-arrivals/products.json", url.Values{"limit": {strconv.Itoa(limit)}}, "application/json", sign)
	if err != nil {
		return nil, err
	}
	return ParseProducts(body)
}

// CatalogPage lists one page of the whole catalog, from 1, with the number
// of products the page lists before unusable ones are left out.
func (c *Client) CatalogPage(ctx context.Context, page int, sign bool) ([]Product, int, error) {
	if page < 1 || page > MaxCatalogPages {
		return nil, 0, &Error{Code: "shop_invalid_request"}
	}
	body, err := c.get(ctx, "/products.json", url.Values{"limit": {strconv.Itoa(CatalogPageSize)}, "page": {strconv.Itoa(page)}}, "application/json", sign)
	if err != nil {
		return nil, 0, err
	}
	return parseProductPage(body)
}

// Collections lists the shop's collections.
func (c *Client) Collections(ctx context.Context, sign bool) ([]Collection, error) {
	body, err := c.get(ctx, "/collections.json", url.Values{"limit": {strconv.Itoa(collectionsLimit)}}, "application/json", sign)
	if err != nil {
		return nil, err
	}
	return ParseCollections(body)
}

// Atom lists the newest AtomLimit products of the whole catalog from its
// Atom feed.
func (c *Client) Atom(ctx context.Context, sign bool) ([]Product, error) {
	body, err := c.get(ctx, "/collections/all.atom", nil, "application/atom+xml", sign)
	if err != nil {
		return nil, err
	}
	return ParseAtom(body)
}

// get requests one fixed path on Host and returns the bounded body. A
// signed request without a key fails before any request.
func (c *Client) get(ctx context.Context, path string, query url.Values, accept string, sign bool) ([]byte, error) {
	if sign && c.Signer == nil {
		return nil, &Error{Code: ErrNoSigningKey}
	}
	u := url.URL{Scheme: "https", Host: Host, Path: path, RawQuery: query.Encode()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &Error{Code: "shop_invalid_request"}
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", accept)
	if sign && c.Signer.SignRequest(req) != nil {
		return nil, &Error{Code: "shop_signing_failed"}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, c.transportError(ctx, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return nil, &Error{Code: "shop_redirect_rejected"}
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, &Error{Code: "shop_rate_limited", RetryAfter: c.retryAfter(resp.Header.Get("Retry-After"))}
	case resp.StatusCode != http.StatusOK:
		return nil, &Error{Code: "shop_http_" + strconv.Itoa(resp.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, c.transportError(ctx, err)
	}
	if len(body) > MaxResponseBytes {
		return nil, &Error{Code: "shop_response_oversized"}
	}
	return body, nil
}

func (c *Client) transportError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var timeout interface{ Timeout() bool }
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
		return &Error{Code: "shop_timeout"}
	}
	return &Error{Code: "shop_unavailable"}
}

// retryAfter reads Retry-After as seconds or an HTTP date.
func (c *Client) retryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	wait := defaultRetryAfter
	if seconds, err := strconv.Atoi(value); err == nil {
		wait = time.Duration(seconds) * time.Second
	} else if at, err := http.ParseTime(value); err == nil {
		wait = at.Sub(c.now())
	}
	if wait <= 0 {
		wait = defaultRetryAfter
	}
	return min(wait, maxRetryAfter)
}

func decode(body []byte, out any) error {
	if json.Unmarshal(body, out) != nil {
		return &Error{Code: "shop_invalid_json"}
	}
	return nil
}
