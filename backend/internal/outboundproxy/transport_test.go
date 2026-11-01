package outboundproxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func answer(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}

// fakeNetwork stands in for real proxies: each proxy host either answers
// with its own name or fails to connect, and every attempt is recorded.
type fakeNetwork struct {
	mu       sync.Mutex
	down     map[string]bool
	attempts []string
	// written makes a failing proxy fail after the request was written.
	written bool
}

func (n *fakeNetwork) transport(proxy Proxy) http.RoundTripper {
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n.mu.Lock()
		n.attempts = append(n.attempts, proxy.Host)
		down, written := n.down[proxy.Host], n.written
		n.mu.Unlock()
		if r.Body != nil {
			_, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		if down {
			if written {
				return nil, errors.New("connection reset after write")
			}
			return nil, errors.New("proxyconnect tcp: connection refused")
		}
		return answer(proxy.Host), nil
	})
}

func (n *fakeNetwork) reset() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	attempts := n.attempts
	n.attempts = nil
	return attempts
}

func testPool(t *testing.T, config Config, network *fakeNetwork) (*Pool, *time.Time) {
	t.Helper()
	normalized, err := Normalize(config)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	pool := NewPool(nil, "")
	pool.now = func() time.Time { return now }
	pool.base = network.transport
	pool.Direct = roundTripFunc(func(*http.Request) (*http.Response, error) { return answer("direct"), nil })
	pool.set(normalized)
	return pool, &now
}

func proxyAt(id, host string) Proxy {
	return Proxy{ID: id, Scheme: "http", Host: host, Port: 8080, Enabled: true}
}

func get(t *testing.T, pool *Pool, area Area, target string) (string, error) {
	t.Helper()
	request, _ := http.NewRequest(http.MethodGet, target, nil)
	response, err := pool.Transport(area).RoundTrip(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return string(body), nil
}

func TestAreaWithoutRouteConnectsDirectly(t *testing.T) {
	network := &fakeNetwork{}
	pool, _ := testPool(t, Config{Proxies: []Proxy{proxyAt("a", "proxy-a.test")}, Routes: map[Area]Route{AreaX: {Enabled: true}}}, network)
	body, err := get(t, pool, AreaYoutube, "https://video.example.test/feed")
	if err != nil || body != "direct" {
		t.Fatalf("body=%q err=%v", body, err)
	}
	if attempts := network.reset(); len(attempts) != 0 {
		t.Fatalf("attempts=%v", attempts)
	}
}

func TestQueueTriesProxiesInOrderUntilOneAnswers(t *testing.T) {
	network := &fakeNetwork{down: map[string]bool{"proxy-a.test": true}}
	pool, now := testPool(t, Config{
		Proxies: []Proxy{proxyAt("a", "proxy-a.test"), proxyAt("b", "proxy-b.test"), proxyAt("c", "proxy-c.test")},
		Routes:  map[Area]Route{AreaX: {Enabled: true}},
	}, network)
	body, err := get(t, pool, AreaX, "https://api.example.test/timeline")
	if err != nil || body != "proxy-b.test" {
		t.Fatalf("body=%q err=%v", body, err)
	}
	if got := strings.Join(network.reset(), ","); got != "proxy-a.test,proxy-b.test" {
		t.Fatalf("attempts=%s", got)
	}
	// The failed proxy cools down behind the others, then leads again.
	if body, _ = get(t, pool, AreaX, "https://api.example.test/timeline"); body != "proxy-b.test" {
		t.Fatalf("cooling body=%q", body)
	}
	if got := strings.Join(network.reset(), ","); got != "proxy-b.test" {
		t.Fatalf("cooling attempts=%s", got)
	}
	*now = now.Add(cooldown + time.Second)
	network.down = map[string]bool{}
	if body, _ = get(t, pool, AreaX, "https://api.example.test/timeline"); body != "proxy-a.test" {
		t.Fatalf("recovered body=%q", body)
	}
}

func TestRouteSelectionAndDisabledProxies(t *testing.T) {
	network := &fakeNetwork{}
	disabled := proxyAt("b", "proxy-b.test")
	disabled.Enabled = false
	pool, _ := testPool(t, Config{
		Proxies: []Proxy{proxyAt("a", "proxy-a.test"), disabled, proxyAt("c", "proxy-c.test")},
		Routes:  map[Area]Route{AreaBilibili: {Enabled: true, ProxyIDs: []string{"c", "b"}}},
	}, network)
	if body, _ := get(t, pool, AreaBilibili, "https://api.example.test/"); body != "proxy-c.test" {
		t.Fatalf("body=%q", body)
	}
}

func TestStaleRouteSelectionDoesNotBecomeAllProxies(t *testing.T) {
	normalized, err := Normalize(Config{
		Proxies: []Proxy{proxyAt("remaining", "proxy-remaining.test")},
		Routes:  map[Area]Route{AreaX: {Enabled: true, ProxyIDs: []string{"removed"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	routed, usable := normalized.Resolve(AreaX)
	if !routed || len(usable) != 0 {
		t.Fatalf("routed=%v usable=%v route=%+v", routed, usable, normalized.Routes[AreaX])
	}
}

func TestEveryProxyFailingFailsClosedUnlessDirectFallback(t *testing.T) {
	network := &fakeNetwork{down: map[string]bool{"proxy-a.test": true}}
	config := Config{Proxies: []Proxy{proxyAt("a", "proxy-a.test")}, Routes: map[Area]Route{AreaMedia: {Enabled: true}}}
	pool, _ := testPool(t, config, network)
	_, err := get(t, pool, AreaMedia, "https://media.example.test/a.jpg")
	if err == nil || strings.Contains(err.Error(), "proxy-a") {
		t.Fatalf("err=%v", err)
	}
	config.DirectFallback = true
	pool, _ = testPool(t, config, network)
	if body, err := get(t, pool, AreaMedia, "https://media.example.test/a.jpg"); err != nil || body != "direct" {
		t.Fatalf("fallback body=%q err=%v", body, err)
	}
}

func TestRoutedAreaWithoutUsableProxy(t *testing.T) {
	network := &fakeNetwork{}
	disabled := proxyAt("a", "proxy-a.test")
	disabled.Enabled = false
	pool, _ := testPool(t, Config{Proxies: []Proxy{disabled}, Routes: map[Area]Route{AreaX: {Enabled: true}}}, network)
	if _, err := get(t, pool, AreaX, "https://api.example.test/"); !errors.Is(err, ErrNoProxy) {
		t.Fatalf("err=%v", err)
	}
}

func TestNonIdempotentRequestMovesOnOnlyBeforeItIsWritten(t *testing.T) {
	network := &fakeNetwork{down: map[string]bool{"proxy-a.test": true}}
	pool, _ := testPool(t, Config{
		Proxies: []Proxy{proxyAt("a", "proxy-a.test"), proxyAt("b", "proxy-b.test")},
		Routes:  map[Area]Route{AreaLLM: {Enabled: true}},
	}, network)
	post := func() (*http.Response, error) {
		request, _ := http.NewRequest(http.MethodPost, "https://provider.example.test/v1/responses", strings.NewReader(`{"input":"synthetic"}`))
		return pool.Transport(AreaLLM).RoundTrip(request)
	}
	if response, err := post(); err != nil {
		t.Fatalf("unwritten failure should move on: %v", err)
	} else {
		response.Body.Close()
	}
	network.reset()

	// A proxy that fails after the request was written ends the attempt.
	written := &fakeNetwork{down: map[string]bool{"proxy-a.test": true}, written: true}
	pool.base = func(proxy Proxy) http.RoundTripper {
		inner := written.transport(proxy)
		return roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if trace := traceOf(r); trace != nil && trace.WroteHeaders != nil {
				trace.WroteHeaders()
			}
			return inner.RoundTrip(r)
		})
	}
	pool.transports = map[string]http.RoundTripper{}
	pool.health = map[string]*health{}
	if _, err := post(); err == nil {
		t.Fatal("a written POST was replayed through another proxy")
	}
	if got := strings.Join(written.reset(), ","); got != "proxy-a.test" {
		t.Fatalf("attempts=%s", got)
	}
}

func TestCancelledRequestDoesNotTryTheNextProxy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	network := &fakeNetwork{}
	pool, _ := testPool(t, Config{
		Proxies: []Proxy{proxyAt("a", "proxy-a.test"), proxyAt("b", "proxy-b.test")},
		Routes:  map[Area]Route{AreaX: {Enabled: true}},
	}, network)
	pool.base = func(proxy Proxy) http.RoundTripper {
		return roundTripFunc(func(*http.Request) (*http.Response, error) {
			network.mu.Lock()
			network.attempts = append(network.attempts, proxy.Host)
			network.mu.Unlock()
			cancel()
			return nil, context.Canceled
		})
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.test/", nil)
	if _, err := pool.Transport(AreaX).RoundTrip(request); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if got := network.reset(); len(got) != 1 {
		t.Fatalf("attempts=%v", got)
	}
}

func TestLocalDestinationsBypassTheProxy(t *testing.T) {
	network := &fakeNetwork{}
	pool, _ := testPool(t, Config{Proxies: []Proxy{proxyAt("a", "proxy-a.test")}, Routes: map[Area]Route{AreaLLM: {Enabled: true}}}, network)
	for _, target := range []string{"http://localhost:11434/v1", "http://127.0.0.1:8000/", "http://[::1]:9000/", "http://models.localhost/"} {
		if body, err := get(t, pool, AreaLLM, target); err != nil || body != "direct" {
			t.Fatalf("%s body=%q err=%v", target, body, err)
		}
	}
	for _, host := range []string{"models.lan", "gpu.home.arpa", "printer.local", "vault.internal", "fe80::1", "fd12:3456::20"} {
		if !bypass(host) {
			t.Fatalf("%s was not bypassed", host)
		}
	}
	if body, _ := get(t, pool, AreaLLM, "https://provider.example.test/"); body != "proxy-a.test" {
		t.Fatalf("public body=%q", body)
	}
}

func TestHealthTracksAttemptsAndResetsWhenAProxyChanges(t *testing.T) {
	network := &fakeNetwork{down: map[string]bool{"proxy-a.test": true}}
	config := Config{Proxies: []Proxy{proxyAt("a", "proxy-a.test"), proxyAt("b", "proxy-b.test")}, Routes: map[Area]Route{AreaX: {Enabled: true}}}
	pool, _ := testPool(t, config, network)
	_, _ = get(t, pool, AreaX, "https://api.example.test/")
	health := pool.Health(pool.config)
	if health["a"].LastFailureAt == nil || health["a"].CoolingUntil == nil || health["b"].LastSuccessAt == nil {
		t.Fatalf("health=%+v", health)
	}
	config.Proxies[0].Port = 3128
	normalized, _ := Normalize(config)
	pool.set(normalized)
	if health = pool.Health(pool.config); health["a"].LastFailureAt != nil {
		t.Fatalf("changed proxy kept its health: %+v", health["a"])
	}
}

func TestProxyURLCarriesCredentials(t *testing.T) {
	proxy := Proxy{Scheme: "socks5h", Host: "2001:db8::1", Port: 1080, Username: "user", Password: "synthetic-pass"}
	if got := proxy.URL().String(); got != "socks5h://user:synthetic-pass@[2001:db8::1]:1080" {
		t.Fatalf("url=%s", got)
	}
}

func traceOf(r *http.Request) *httptrace.ClientTrace {
	return httptrace.ContextClientTrace(r.Context())
}

func TestProcessAttemptsFollowTheRouteWithAtMostThreeProxies(t *testing.T) {
	network := &fakeNetwork{}
	proxies := []Proxy{proxyAt("a", "proxy-a.test"), proxyAt("b", "proxy-b.test"), proxyAt("c", "proxy-c.test"), proxyAt("d", "proxy-d.test")}
	proxies[0].Username = "user"
	proxies[0].Password = "synthetic-pass"
	pool, _ := testPool(t, Config{Proxies: proxies, Routes: map[Area]Route{AreaYoutube: {Enabled: true}}, DirectFallback: true}, network)

	if attempts, err := pool.ProcessAttempts(context.Background(), AreaX); err != nil || len(attempts) != 1 || attempts[0].Env != nil {
		t.Fatalf("unrouted attempts=%+v err=%v", attempts, err)
	}
	attempts, err := pool.ProcessAttempts(context.Background(), AreaYoutube)
	if err != nil || len(attempts) != 4 || attempts[3].Env != nil {
		t.Fatalf("attempts=%+v err=%v", attempts, err)
	}
	variables := map[string]string{}
	for _, entry := range attempts[0].Env {
		name, value, _ := strings.Cut(entry, "=")
		variables[name] = value
	}
	address, err := url.Parse(variables["HTTPS_PROXY"])
	if err != nil || address.Host != "proxy-a.test:8080" || address.User.Username() != "user" || variables["all_proxy"] != variables["HTTPS_PROXY"] {
		t.Fatalf("variables=%v err=%v", variables, err)
	}
	if password, _ := address.User.Password(); password != "synthetic-pass" {
		t.Fatal("proxy password missing from the environment")
	}
	// A failed run cools its proxy down, so the next run starts elsewhere.
	attempts[0].Done(false)
	attempts, _ = pool.ProcessAttempts(context.Background(), AreaYoutube)
	if !strings.Contains(attempts[0].Env[0], "proxy-b.test") {
		t.Fatalf("env=%v", attempts[0].Env)
	}

	disabled := proxyAt("a", "proxy-a.test")
	disabled.Enabled = false
	pool, _ = testPool(t, Config{Proxies: []Proxy{disabled}, Routes: map[Area]Route{AreaYoutube: {Enabled: true}}}, network)
	if _, err := pool.ProcessAttempts(context.Background(), AreaYoutube); !errors.Is(err, ErrNoProxy) {
		t.Fatalf("err=%v", err)
	}
}
