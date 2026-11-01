package outboundproxy

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// cooldown is how long a proxy that failed to answer stays behind the
	// proxies that have not, so one unreachable proxy does not add its
	// connect timeout to every request.
	cooldown = time.Minute
	// connectTimeout bounds the TCP connection to a proxy.
	connectTimeout = 10 * time.Second
	// maxProcessAttempts bounds the proxies an external program run tries,
	// since each attempt may use the program's whole timeout.
	maxProcessAttempts = 3
)

// ErrNoProxy reports a routed area without a usable proxy and without the
// direct fallback.
var ErrNoProxy = errors.New("no usable outbound proxy")

// Pool owns the cached configuration, one transport per proxy and each
// proxy's health. Transports for every area share it, so a proxy that failed
// for one area is tried last by the others too.
type Pool struct {
	db  *sql.DB
	key string
	now func() time.Time
	// Direct carries requests of areas that are not routed, bypassed local
	// destinations and the direct fallback. It defaults to
	// http.DefaultTransport, which keeps the process proxy environment.
	Direct http.RoundTripper
	// base builds the transport of one proxy; tests replace it.
	base func(Proxy) http.RoundTripper

	// writeMu serializes operator writes from their transaction through
	// set, so the cache publishes configurations in commit order.
	writeMu sync.Mutex

	mu         sync.Mutex
	loaded     bool
	config     Config
	transports map[string]http.RoundTripper
	health     map[string]*health
}

// health is the process-local record of one proxy, keyed by proxy id and
// reset when its endpoint or credentials change.
type health struct {
	fingerprint   string
	lastSuccessAt time.Time
	lastFailureAt time.Time
	coolingUntil  time.Time
}

// Health is the operator projection of a proxy's recent attempts.
type Health struct {
	LastSuccessAt *time.Time `json:"lastSuccessAt"`
	LastFailureAt *time.Time `json:"lastFailureAt"`
	CoolingUntil  *time.Time `json:"coolingUntil"`
}

// NewPool reads its configuration from db on first use. key is
// LLM_SECRETS_KEY, which unseals proxy passwords.
func NewPool(db *sql.DB, key string) *Pool {
	return &Pool{db: db, key: key, now: time.Now, Direct: http.DefaultTransport, base: proxyTransport, transports: map[string]http.RoundTripper{}, health: map[string]*health{}}
}

// proxyTransport clones the default transport with a bounded connect and the
// proxy as every request's first hop.
func proxyTransport(proxy Proxy) http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxy.URL())
	transport.DialContext = (&net.Dialer{Timeout: connectTimeout, KeepAlive: 30 * time.Second}).DialContext
	return transport
}

// Config returns the current configuration, loading it on first use. A
// failed load is retried by the next call.
func (p *Pool) Config(ctx context.Context) (Config, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.loaded {
		config, err := load(ctx, p.db, p.key)
		if err != nil {
			return Config{}, ErrUnavailable
		}
		p.config, p.loaded = config, true
	}
	return p.config, nil
}

// set publishes a committed configuration and drops transports of proxies
// that changed or left the queue.
func (p *Pool) set(config Config) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config, p.loaded = config, true
	keep := map[string]bool{}
	for _, proxy := range config.Proxies {
		keep[proxy.fingerprint()] = true
	}
	for fingerprint, transport := range p.transports {
		if !keep[fingerprint] {
			if closer, ok := transport.(interface{ CloseIdleConnections() }); ok {
				closer.CloseIdleConnections()
			}
			delete(p.transports, fingerprint)
		}
	}
}

// Transport returns the round tripper of area. It resolves the route on every
// request, so saved settings apply to the next request without a restart.
func (p *Pool) Transport(area Area) http.RoundTripper {
	return &areaTransport{pool: p, area: area}
}

// Client returns a client for area. Callers replace its timeout and redirect
// policy with their own fixed bounds.
func (p *Pool) Client(area Area) *http.Client {
	return &http.Client{Transport: p.Transport(area)}
}

type attempt struct {
	proxy     Proxy
	transport http.RoundTripper
}

// ProcessAttempt is one way to start an external program, such as yt-dlp,
// for an area. Env holds the proxy variables to add to the program's
// environment; nil leaves the environment as it is. Done receives whether the
// attempt reached its destination, which updates the proxy's health.
type ProcessAttempt struct {
	Env  []string
	Done func(connected bool)
}

// ProcessAttempts lists the ways to run a program for area, in order: one
// unchanged attempt when the area is not routed, otherwise the first usable
// proxies in plan order (at most maxProcessAttempts), then an unchanged
// attempt with the direct fallback. A routed area with nothing to try is
// ErrNoProxy.
func (p *Pool) ProcessAttempts(ctx context.Context, area Area) ([]ProcessAttempt, error) {
	routed, attempts, fallback, err := p.plan(ctx, area)
	if err != nil {
		return nil, err
	}
	if !routed {
		return []ProcessAttempt{{}}, nil
	}
	out := []ProcessAttempt{}
	for _, next := range attempts[:min(len(attempts), maxProcessAttempts)] {
		proxy, address := next.proxy, next.proxy.URL().String()
		env := []string{}
		for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
			env = append(env, name+"="+address)
		}
		out = append(out, ProcessAttempt{Env: env, Done: func(connected bool) { p.record(proxy, connected) }})
	}
	if fallback {
		out = append(out, ProcessAttempt{})
	}
	if len(out) == 0 {
		return nil, ErrNoProxy
	}
	return out, nil
}

// plan lists the proxies to try for area: ready ones first, then cooling
// ones, each group in queue order, so every proxy is still attempted when all
// of them failed recently. routed is false for an area without a route.
func (p *Pool) plan(ctx context.Context, area Area) (routed bool, attempts []attempt, fallback bool, err error) {
	config, err := p.Config(ctx)
	if err != nil {
		return false, nil, false, err
	}
	routed, proxies := config.Resolve(area)
	if !routed {
		return false, nil, false, nil
	}
	now := p.now()
	p.mu.Lock()
	defer p.mu.Unlock()
	ready, cooling := []attempt{}, []attempt{}
	for _, proxy := range proxies {
		fingerprint := proxy.fingerprint()
		transport, ok := p.transports[fingerprint]
		if !ok {
			transport = p.base(proxy)
			p.transports[fingerprint] = transport
		}
		next := attempt{proxy: proxy, transport: transport}
		if record := p.health[proxy.ID]; record != nil && record.fingerprint == fingerprint && record.coolingUntil.After(now) {
			cooling = append(cooling, next)
		} else {
			ready = append(ready, next)
		}
	}
	return true, append(ready, cooling...), config.DirectFallback, nil
}

func (p *Pool) record(proxy Proxy, ok bool) {
	now := p.now()
	fingerprint := proxy.fingerprint()
	p.mu.Lock()
	defer p.mu.Unlock()
	record := p.health[proxy.ID]
	if record == nil || record.fingerprint != fingerprint {
		record = &health{fingerprint: fingerprint}
		p.health[proxy.ID] = record
	}
	if ok {
		record.lastSuccessAt, record.coolingUntil = now, time.Time{}
	} else {
		record.lastFailureAt, record.coolingUntil = now, now.Add(cooldown)
	}
}

// Health reports the recent attempts of each configured proxy by id.
func (p *Pool) Health(config Config) map[string]Health {
	now := p.now()
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]Health, len(config.Proxies))
	for _, proxy := range config.Proxies {
		var item Health
		if record := p.health[proxy.ID]; record != nil && record.fingerprint == proxy.fingerprint() {
			item.LastSuccessAt = optionalTime(record.lastSuccessAt)
			item.LastFailureAt = optionalTime(record.lastFailureAt)
			if record.coolingUntil.After(now) {
				item.CoolingUntil = optionalTime(record.coolingUntil)
			}
		}
		out[proxy.ID] = item
	}
	return out
}

func optionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}

type areaTransport struct {
	pool *Pool
	area Area
}

// RoundTrip sends the request through the first proxy of the area's queue
// that answers it. A request moves to the next proxy only when the previous
// one produced no response, the request context is still live and the body
// can be replayed; an HTTP error status is an answer and is returned as is. A
// request that is not idempotent, such as a model provider POST, moves on
// only when it was never written, so a proxy that drops the connection
// mid-request cannot make the destination receive it twice. With the direct
// fallback, a direct attempt follows the last proxy.
func (t *areaTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if bypass(request.URL.Hostname()) {
		return t.pool.Direct.RoundTrip(request)
	}
	routed, attempts, fallback, err := t.pool.plan(request.Context(), t.area)
	if err != nil {
		closeBody(request)
		return nil, err
	}
	if !routed {
		return t.pool.Direct.RoundTrip(request)
	}
	idempotent := isIdempotent(request)
	var lastErr error
	for index, next := range attempts {
		current := request
		if index > 0 {
			replay, ok := replayRequest(request)
			if !ok {
				return nil, failure(lastErr)
			}
			current = replay
		}
		var written atomic.Bool
		if !idempotent {
			current = current.WithContext(httptrace.WithClientTrace(current.Context(), &httptrace.ClientTrace{
				WroteHeaders: func() { written.Store(true) },
			}))
		}
		response, err := next.transport.RoundTrip(current)
		if err == nil {
			t.pool.record(next.proxy, true)
			return response, nil
		}
		lastErr = err
		if request.Context().Err() != nil {
			return nil, request.Context().Err()
		}
		t.pool.record(next.proxy, false)
		if written.Load() {
			return nil, failure(err)
		}
	}
	if fallback {
		if len(attempts) == 0 {
			return t.pool.Direct.RoundTrip(request)
		}
		if replay, ok := replayRequest(request); ok {
			return t.pool.Direct.RoundTrip(replay)
		}
	}
	if lastErr == nil {
		closeBody(request)
		return nil, ErrNoProxy
	}
	return nil, failure(lastErr)
}

// proxyError hides proxy addresses in transport errors behind a fixed
// message; Unwrap keeps timeouts and cancellations recognizable.
type proxyError struct{ cause error }

func (e *proxyError) Error() string { return "outbound proxy request failed" }
func (e *proxyError) Unwrap() error { return e.cause }

func failure(cause error) error { return &proxyError{cause: cause} }

func closeBody(request *http.Request) {
	if request.Body != nil {
		_ = request.Body.Close()
	}
}

// isIdempotent reports whether sending request twice has the effect of
// sending it once, as net/http judges it for its own retries.
func isIdempotent(request *http.Request) bool {
	switch request.Method {
	case "", http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

func replayRequest(request *http.Request) (*http.Request, bool) {
	replay := request.Clone(request.Context())
	if request.Body == nil || request.Body == http.NoBody {
		return replay, true
	}
	if request.GetBody == nil {
		return nil, false
	}
	body, err := request.GetBody()
	if err != nil {
		return nil, false
	}
	replay.Body = body
	return replay, true
}

// localSuffixes are names only this machine or its network resolves.
var localSuffixes = []string{".localhost", ".local", ".lan", ".internal", ".home.arpa"}

// bypass keeps this machine, private networks and local-only names off the
// proxy, since a proxy elsewhere cannot reach them. It matters for a model
// provider on the local network; every source host is public.
func bypass(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" {
		return true
	}
	for _, suffix := range localSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	address = address.Unmap()
	return address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsUnspecified()
}
