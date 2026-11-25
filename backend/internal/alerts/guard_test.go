package alerts

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"
)

// inside is the first address after a block's network address.
func inside(block string) netip.Addr {
	return netip.MustParsePrefix(block).Addr().Next()
}

func TestPublicAddressRefusesEveryNonPublicRange(t *testing.T) {
	refused := []netip.Addr{
		inside("10.0.0.0/8"), inside("172.16.0.0/12"), inside("192.168.0.0/16"), inside("100.64.0.0/10"),
		inside("169.254.0.0/16"), inside("198.18.0.0/15"), inside("224.0.0.0/4"), inside("240.0.0.0/4"),
		netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("0.0.0.0"), netip.MustParseAddr("192.0.2.1"),
	}
	for _, raw := range []string{"::1", "fd00::1", "fd7a:115c:a1e0::1", "fe80::1", "::ffff:127.0.0.1", "2001:db8::1", "ff02::1"} {
		refused = append(refused, netip.MustParseAddr(raw))
	}
	for _, address := range refused {
		if publicAddress(address) {
			t.Errorf("%s counted as public", address)
		}
	}
	if !publicAddress(netip.MustParseAddr("2a00:1450::1")) {
		t.Error("a global unicast address counted as non-public")
	}
}

func TestValidateURLAcceptsOnlyPublicHTTPSAddresses(t *testing.T) {
	if _, err := validateURL("https://example.com/alerts", false); err != nil {
		t.Fatalf("public https address refused: %v", err)
	}
	withUser := (&url.URL{Scheme: "https", Host: "example.com", Path: "/alerts", User: url.UserPassword("synthetic", "synthetic")}).String()
	for _, raw := range []string{
		"http://example.com/alerts",
		withUser,
		"https://localhost/alerts",
		"https://ntfy.localhost/alerts",
		"https://127.0.0.1/alerts",
		"https://" + inside("192.168.0.0/16").String() + "/alerts",
		(&url.URL{Scheme: "https", Host: "[fd00::1]", Path: "/alerts"}).String(),
		"https://example.com/alerts?token=synthetic",
	} {
		if _, err := validateURL(raw, false); err == nil {
			t.Errorf("%s accepted", raw)
		}
	}
	// A sealed address may carry its token in the query.
	if _, err := validateURL("https://example.com/ping?token=synthetic", true); err != nil {
		t.Fatalf("sealed address refused: %v", err)
	}
}

type fixedResolver []netip.Addr

func (r fixedResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r, nil
}

func TestCheckHostRefusesANameWithAnyPrivateAddress(t *testing.T) {
	g := guard{allow: publicAddress, resolver: fixedResolver{netip.MustParseAddr("2a00:1450::1"), inside("10.0.0.0/8")}}
	if err := g.checkHost(context.Background(), "example.com"); !errors.Is(err, errRejected) {
		t.Fatalf("err=%v", err)
	}
	g.resolver = fixedResolver{netip.MustParseAddr("2a00:1450::1")}
	if err := g.checkHost(context.Background(), "example.com"); err != nil {
		t.Fatalf("public name refused: %v", err)
	}
}

// A name that passed the resolver check but connects to a private address
// (DNS rebinding) is refused when the connection is dialled.
func TestDirectTransportRefusesAPrivateAddressAtDialTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("the private server was reached")
	}))
	defer server.Close()
	client := &http.Client{Transport: newGuard().direct()}
	response, err := client.Get(server.URL)
	if err == nil {
		response.Body.Close()
		t.Fatal("request reached a loopback address")
	}
	if !errors.Is(err, errRejected) {
		t.Fatalf("err=%v", err)
	}
}

func TestGuardedTransportRefusesPlainHTTP(t *testing.T) {
	client := &http.Client{Transport: guardedTransport{guard: newGuard(), next: http.DefaultTransport}}
	_, err := client.Get("http://example.com/alerts")
	if !errors.Is(err, errRejected) {
		t.Fatalf("err=%v", err)
	}
}
