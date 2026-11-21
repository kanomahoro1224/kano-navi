package alerts

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/yexca/kano-navi/backend/internal/urlguard"
)

// errRejected refuses a destination that is not a public address. Alerts go
// only to public services: private networks, this machine and shared address
// space (which includes Tailscale) are never reached.
var errRejected = errors.New("alert destination is not a public address")

// nonPublic lists ranges that are not globally reachable destinations, beyond
// what netip classifies as private, loopback, link-local or multicast.
var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("fc00::/7"),
}

// localSuffixes are names only this machine or its network resolves.
var localSuffixes = []string{".localhost", ".local", ".lan", ".internal", ".home.arpa", ".test", ".invalid"}

// publicAddress reports whether address is a globally reachable unicast
// address.
func publicAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() ||
		address.IsLinkLocalUnicast() || address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	for _, prefix := range nonPublic {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

// validateURL checks an operator-entered address before it is saved: https,
// a hostname or public IP address, and no embedded user credentials.
// sealed allows credential-bearing query parameters, for an address stored
// sealed.
func validateURL(raw string, sealed bool) (*url.URL, error) {
	if raw == "" {
		return nil, inputError("address is required")
	}
	if len(raw) > maxURLLength || hasControl(raw) {
		return nil, inputError("address must be at most 2048 characters")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Opaque != "" {
		return nil, inputError("address must be an https URL")
	}
	if parsed.User != nil {
		return nil, inputError("address must not contain a username or password")
	}
	if !sealed && urlguard.ContainsCredentials(raw) {
		return nil, inputError("address must not carry a token in its query; use the secret field")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" || host == "localhost" {
		return nil, inputError("address must be a public host")
	}
	for _, suffix := range localSuffixes {
		if strings.HasSuffix(host, suffix) {
			return nil, inputError("address must be a public host")
		}
	}
	if address, err := netip.ParseAddr(host); err == nil && !publicAddress(address) {
		return nil, inputError("address must be a public host")
	}
	parsed.Fragment = ""
	return parsed, nil
}

// guard checks every destination twice: its name must resolve only to
// public addresses before a request is sent (also before it is handed to a
// proxy), and a direct connection is refused at dial time when the address it
// actually reaches is not public, so DNS rebinding cannot get past the first
// check.
type guard struct {
	allow    func(netip.Addr) bool
	resolver interface {
		LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
	}
	// insecure lets tests reach plain-HTTP servers.
	insecure bool
}

func newGuard() guard {
	return guard{allow: publicAddress, resolver: net.DefaultResolver}
}

func (g guard) checkHost(ctx context.Context, host string) error {
	host = strings.TrimSuffix(host, ".")
	if address, err := netip.ParseAddr(host); err == nil {
		if !g.allow(address) {
			return errRejected
		}
		return nil
	}
	addresses, err := g.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return err
	}
	if len(addresses) == 0 {
		return errRejected
	}
	for _, address := range addresses {
		if !g.allow(address) {
			return errRejected
		}
	}
	return nil
}

// direct is the transport of the notify area's direct connections. It
// ignores HTTP_PROXY and HTTPS_PROXY: a proxy for alerts is configured in the
// outbound proxy pool.
func (g guard) direct() http.RoundTripper {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return errRejected
			}
			parsed, err := netip.ParseAddr(host)
			if err != nil || !g.allow(parsed) {
				return errRejected
			}
			return nil
		},
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = dialer.DialContext
	return transport
}

// guardedTransport checks a request's destination before next sends it.
type guardedTransport struct {
	guard guard
	next  http.RoundTripper
}

func (t guardedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme != "https" && !(t.guard.insecure && request.URL.Scheme == "http") {
		if request.Body != nil {
			_ = request.Body.Close()
		}
		return nil, errRejected
	}
	if err := t.guard.checkHost(request.Context(), request.URL.Hostname()); err != nil {
		if request.Body != nil {
			_ = request.Body.Close()
		}
		return nil, err
	}
	return t.next.RoundTrip(request)
}
