package outboundproxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/secretstore"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const (
	testTimeout   = 10 * time.Second
	testReadLimit = 64 << 10
)

// testTargets are fixed public pages of hosts each area already reads. A
// test reaches one of them through one proxy; any HTTP answer means the
// proxy carried the request. Model providers have no fixed host.
var testTargets = map[Area]string{
	AreaX:        "https://api.fxtwitter.com/",
	AreaYoutube:  "https://www.youtube.com/generate_204",
	AreaBilibili: "https://www.bilibili.com/",
	AreaMedia:    "https://pbs.twimg.com/",
}

// Admin owns /api/admin/proxy: the proxy queue, the area routes and proxy
// tests. Saved settings reach the pool's transports immediately.
type Admin struct {
	store *sqlite.Store
	pool  *Pool
	now   func() time.Time
}

var _ httpx.AdminModule = (*Admin)(nil)

// NewAdmin binds the operator routes to pool.
func NewAdmin(store *sqlite.Store, pool *Pool) *Admin {
	return &Admin{store: store, pool: pool, now: time.Now}
}

// AdminRoutes lists the proxy endpoints. The router has already
// authenticated the operator and applied the origin guard.
func (a *Admin) AdminRoutes() []httpx.Route {
	return []httpx.Route{
		{Method: http.MethodGet, Path: "/proxy", Handler: a.get},
		{Method: http.MethodPut, Path: "/proxy", Handler: a.put},
		{Method: http.MethodPost, Path: "/proxy/{id}/test", Handler: a.test},
	}
}

type proxyResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Scheme   string `json:"scheme"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	// HasPassword and PasswordUnreadable replace the password, which is
	// write-only.
	HasPassword        bool   `json:"hasPassword"`
	PasswordUnreadable bool   `json:"passwordUnreadable"`
	Enabled            bool   `json:"enabled"`
	Health             Health `json:"health"`
}

type settingsResponse struct {
	Proxies        []proxyResponse `json:"proxies"`
	Routes         map[Area]Route  `json:"routes"`
	DirectFallback bool            `json:"directFallback"`
	Areas          []Area          `json:"areas"`
	TestAreas      []Area          `json:"testAreas"`
}

func (a *Admin) response(config Config) settingsResponse {
	health := a.pool.Health(config)
	proxies := make([]proxyResponse, 0, len(config.Proxies))
	for _, proxy := range config.Proxies {
		proxies = append(proxies, proxyResponse{
			ID: proxy.ID, Name: proxy.Name, Scheme: proxy.Scheme, Host: proxy.Host, Port: proxy.Port,
			Username: proxy.Username, HasPassword: proxy.Password != "" || proxy.Unreadable,
			PasswordUnreadable: proxy.Unreadable, Enabled: proxy.Enabled, Health: health[proxy.ID],
		})
	}
	routes := make(map[Area]Route, len(config.Routes))
	for area, route := range config.Routes {
		if route.ProxyIDs == nil {
			route.ProxyIDs = []string{}
		}
		routes[area] = route
	}
	testAreas := []Area{}
	for _, area := range Areas {
		if testTargets[area] != "" {
			testAreas = append(testAreas, area)
		}
	}
	return settingsResponse{Proxies: proxies, Routes: routes, DirectFallback: config.DirectFallback, Areas: Areas, TestAreas: testAreas}
}

func (a *Admin) get(w http.ResponseWriter, r *http.Request) {
	config, err := a.pool.Config(r.Context())
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "proxy_settings_unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a.response(config))
}

// proxyPayload omits Password to keep the stored password of the proxy with
// the same id; an empty string clears it.
type proxyPayload struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Scheme   string  `json:"scheme"`
	Host     string  `json:"host"`
	Port     int     `json:"port"`
	Username string  `json:"username"`
	Password *string `json:"password"`
	Enabled  bool    `json:"enabled"`
}

// put replaces the fields it carries: the whole queue, the routes and the
// direct fallback.
func (a *Admin) put(w http.ResponseWriter, r *http.Request) {
	if !a.store.Writable() {
		httpx.ReadOnly(w)
		return
	}
	body, err := httpx.Body(w, r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	a.pool.writeMu.Lock()
	defer a.pool.writeMu.Unlock()
	config, err := a.apply(r.Context(), body)
	switch {
	case errors.Is(err, secretstore.ErrKeyRequired):
		httpx.WriteCode(w, http.StatusServiceUnavailable, "secrets_key_unavailable")
	case errors.Is(err, ErrUnavailable):
		httpx.WriteCode(w, http.StatusServiceUnavailable, "proxy_settings_unavailable")
	case err != nil:
		httpx.WriteError(w, err)
	default:
		httpx.WriteJSON(w, http.StatusOK, a.response(config))
	}
}

func (a *Admin) apply(ctx context.Context, body map[string]json.RawMessage) (Config, error) {
	tx, err := a.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return Config{}, err
	}
	defer tx.Rollback()
	previous, err := load(ctx, tx, a.pool.key)
	if err != nil && !errors.Is(err, ErrUnavailable) {
		return Config{}, err
	}
	sealed, err := sealedPasswords(ctx, tx)
	if err != nil {
		return Config{}, err
	}
	next := Config{Proxies: previous.Proxies, Routes: previous.Routes, DirectFallback: previous.DirectFallback}
	if raw, ok := body["proxies"]; ok {
		var items []proxyPayload
		if json.Unmarshal(raw, &items) != nil || items == nil {
			return Config{}, httpx.Invalid("proxies must be a list of proxies")
		}
		kept := make(map[string]Proxy, len(previous.Proxies))
		for _, proxy := range previous.Proxies {
			kept[proxy.ID] = proxy
		}
		next.Proxies = make([]Proxy, 0, len(items))
		for _, item := range items {
			proxy := Proxy{ID: item.ID, Name: item.Name, Scheme: item.Scheme, Host: item.Host, Port: item.Port, Username: item.Username, Enabled: item.Enabled}
			if old, ok := kept[item.ID]; ok && item.ID != "" && item.Password == nil {
				proxy.Password, proxy.Unreadable = old.Password, old.Unreadable
			}
			if item.Password != nil {
				proxy.Password = *item.Password
			}
			next.Proxies = append(next.Proxies, proxy)
		}
	}
	if raw, ok := body["routes"]; ok {
		var routes map[Area]Route
		if json.Unmarshal(raw, &routes) != nil || routes == nil {
			return Config{}, httpx.Invalid("routes must map areas to routes")
		}
		next.Routes = routes
	}
	if raw, ok := body["directFallback"]; ok {
		if json.Unmarshal(raw, &next.DirectFallback) != nil {
			return Config{}, httpx.Invalid("directFallback must be a boolean")
		}
	}
	config, err := Normalize(next)
	if err != nil {
		return Config{}, err
	}
	if err = save(ctx, tx, config, a.pool.key, sealed, a.now()); err != nil {
		return Config{}, err
	}
	if err = tx.Commit(); err != nil {
		return Config{}, err
	}
	a.pool.set(config)
	return config, nil
}

type testResult struct {
	OK        bool          `json:"ok"`
	Status    int           `json:"status,omitempty"`
	Error     string        `json:"error,omitempty"`
	LatencyMs int64         `json:"latencyMs"`
	Proxy     proxyResponse `json:"proxy"`
}

// test reaches the fixed page of an area through one saved proxy, whether or
// not the proxy is enabled, and records the outcome in its health. It never
// falls back to another proxy or a direct connection.
func (a *Admin) test(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Body(w, r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	area := AreaYoutube
	if raw, ok := body["area"]; ok && json.Unmarshal(raw, &area) != nil {
		httpx.WriteError(w, httpx.Invalid("area must be a string"))
		return
	}
	target := testTargets[area]
	if target == "" {
		httpx.WriteError(w, httpx.Invalid("area must be x, youtube, bilibili or media"))
		return
	}
	config, err := a.pool.Config(r.Context())
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "proxy_settings_unavailable")
		return
	}
	var proxy *Proxy
	for index := range config.Proxies {
		if config.Proxies[index].ID == r.PathValue("id") {
			proxy = &config.Proxies[index]
		}
	}
	if proxy == nil {
		httpx.WriteCode(w, http.StatusNotFound, "proxy_not_found")
		return
	}
	if proxy.Unreadable {
		httpx.WriteError(w, httpx.Invalid("the stored password cannot be read; enter it again"))
		return
	}
	result := a.probe(r.Context(), *proxy, target)
	a.pool.record(*proxy, result.OK)
	result.Proxy = a.response(Config{Proxies: []Proxy{*proxy}}).Proxies[0]
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *Admin) probe(ctx context.Context, proxy Proxy, target string) testResult {
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	client := &http.Client{
		Transport:     a.pool.base(proxy),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return testResult{Error: "failed"}
	}
	started := a.now()
	response, err := client.Do(request)
	latency := a.now().Sub(started).Milliseconds()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return testResult{Error: "timeout", LatencyMs: latency}
		}
		return testResult{Error: "failed", LatencyMs: latency}
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, testReadLimit))
	_ = response.Body.Close()
	return testResult{OK: true, Status: response.StatusCode, LatencyMs: latency}
}
