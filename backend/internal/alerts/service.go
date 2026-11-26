package alerts

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/yexca/kano-navi/backend/internal/outboundproxy"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const (
	defaultInterval = time.Minute
	pruneInterval   = 24 * time.Hour
	// dispatchBatch bounds the deliveries one pass sends.
	dispatchBatch = 20
	// dispatchBudget bounds the time one pass spends sending, below the
	// evaluation interval; the rest waits for the next pass.
	dispatchBudget = 45 * time.Second
	// maxAttempts bounds the tries of one delivery.
	maxAttempts = 5
	// deliveryLifetime drops a delivery still unsent this long after it was
	// queued: its news is stale.
	deliveryLifetime = 24 * time.Hour
)

// backoff is the wait after each failed attempt.
var backoff = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, time.Hour}

// State says whether alerts are evaluated and sent.
const (
	// StateOn evaluates rules and sends deliveries and the heartbeat.
	StateOn = "on"
	// StateDevelopment evaluates rules and records incidents but sends
	// nothing, apart from operator tests.
	StateDevelopment = "development"
	// StateOff (ALERTS_ENABLED=0 or a read-only database) does neither.
	StateOff = "off"
)

// Options configure a Service.
type Options struct {
	Store *sqlite.Store
	// Key is LLM_SECRETS_KEY, which seals channel secrets.
	Key string
	// Pool carries deliveries through the notify proxy area; nil connects
	// directly.
	Pool *outboundproxy.Pool
	// Enabled is false with ALERTS_ENABLED=0.
	Enabled bool
	// Production sends deliveries and the heartbeat. Development mode only
	// records incidents, so a local database never alerts.
	Production bool
	// Origin is the console's origin for message links ("" adds none).
	Origin  string
	Signals Signals
	// Interval is the evaluation period (a minute by default).
	Interval time.Duration
}

// Service evaluates the rule catalog, records incidents and delivers them.
// It works on the local database apart from its deliveries, so it is not a
// workflow run and does not use the queue.
type Service struct {
	store      *sqlite.Store
	key        string
	signals    Signals
	enabled    bool
	production bool
	origin     string
	interval   time.Duration
	now        func() time.Time
	sender     sender

	// writeMu serializes operator writes from their transaction through set.
	writeMu sync.Mutex

	mu       sync.Mutex
	loaded   bool
	settings Settings
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	status   status
}

type status struct {
	evaluatedAt      time.Time
	evaluationFailed bool
	prunedAt         time.Time
	heartbeat        heartbeatStatus
}

type heartbeatStatus struct {
	attemptAt, okAt, failureAt time.Time
	lastError                  string
}

// New builds the service. Deliveries go through the pool's notify area
// behind the public-address guard.
func New(options Options) *Service {
	return newService(options, newGuard())
}

func newService(options Options, g guard) *Service {
	next := g.direct()
	if options.Pool != nil {
		next = options.Pool.TransportWithDirect(outboundproxy.AreaNotify, g.direct())
	}
	client := &http.Client{
		Transport:     guardedTransport{guard: g, next: next},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	interval := options.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	return &Service{
		store: options.Store, key: options.Key, signals: options.Signals, enabled: options.Enabled,
		production: options.Production, origin: options.Origin, interval: interval, now: time.Now,
		sender: sender{client: client, telegram: telegramAPI},
	}
}

// State reports whether alerts are evaluated and sent.
func (s *Service) State() string {
	switch {
	case !s.enabled || !s.store.Writable():
		return StateOff
	case !s.production:
		return StateDevelopment
	}
	return StateOn
}

func (s *Service) delivering() bool { return s.State() == StateOn }

// Settings returns the current settings, loading them on first use. A
// failed load is retried by the next call.
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.loaded {
		settings, err := load(ctx, s.store.DB(), s.key)
		if err != nil {
			return Settings{}, ErrUnavailable
		}
		s.settings, s.loaded = settings, true
	}
	return s.settings, nil
}

func (s *Service) set(settings Settings) {
	s.mu.Lock()
	s.settings, s.loaded = settings, true
	s.mu.Unlock()
}

// Start begins the evaluation loop unless alerts are off.
func (s *Service) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil || s.State() == StateOff {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.Tick(ctx)
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.Tick(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop ends the loop and waits for a pass in progress.
func (s *Service) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		s.wg.Wait()
	}
}

// Tick runs one pass: evaluate the rules, send due deliveries, ping the
// heartbeat when due and prune once a day.
func (s *Service) Tick(ctx context.Context) {
	now := s.now()
	settings, err := s.Settings(ctx)
	if err == nil {
		err = s.evaluateAll(ctx, settings, now)
	}
	if ctx.Err() != nil {
		return
	}
	s.mu.Lock()
	s.status.evaluatedAt, s.status.evaluationFailed = now, err != nil
	pruneDue := now.Sub(s.status.prunedAt) >= pruneInterval
	s.mu.Unlock()
	if settings.Rules == nil {
		return
	}
	if s.delivering() {
		// The heartbeat goes first so a slow channel cannot delay it.
		s.heartbeat(ctx, settings.Heartbeat, false)
		s.dispatch(ctx, settings)
	}
	if pruneDue {
		if retention, err := s.store.RunRetention(ctx); err == nil && prune(ctx, s.store.DB(), retention, now) == nil {
			s.mu.Lock()
			s.status.prunedAt = now
			s.mu.Unlock()
		}
	}
}

// evaluateAll runs every enabled rule and records the pass. A rule that
// could not be evaluated keeps its incidents; the pass reports it failed.
func (s *Service) evaluateAll(ctx context.Context, settings Settings, now time.Time) error {
	outcomes := map[string]outcome{}
	var failed error
	for _, rule := range Catalog {
		setting := settings.Rules[rule.Code]
		if !setting.Enabled {
			continue
		}
		findings, err := s.evaluate(ctx, rule, setting, settings, now)
		if err != nil {
			failed = err
			continue
		}
		outcomes[rule.Code] = outcome{evaluated: true, findings: findings}
	}
	var channels []string
	if s.delivering() {
		for _, channel := range settings.Channels {
			if channel.usable() {
				channels = append(channels, channel.ID)
			}
		}
	}
	if err := apply(ctx, s.store.DB(), settings, outcomes, channels, now); err != nil {
		return err
	}
	return failed
}

// link is the console address a message carries for rule.
func (s *Service) link(rule string) string {
	if s.origin == "" {
		return ""
	}
	view := "alerts"
	switch rule {
	case RuleRunFailedStreak:
		view = "runs"
	case RuleSourceStale, RuleLoginExpired, RuleLoginRefreshFailed:
		view = "sources"
	}
	return s.origin + "/admin#" + view
}

// dispatch sends due deliveries one at a time, for at most dispatchBudget. A
// delivery whose channel was removed or turned off is dropped; one that keeps
// failing stops after maxAttempts, and one still unsent after a day is
// dropped. A channel that timed out or could not be reached gets no further
// attempt in the same pass, so one dead receiver cannot stall the loop.
func (s *Service) dispatch(ctx context.Context, settings Settings) {
	now := s.now()
	started := now
	db := s.store.DB()
	if _, err := db.ExecContext(ctx, `UPDATE alert_deliveries SET status='dropped',last_error='expired',finished_at=? WHERE status='pending' AND julianday(created_at) < julianday(?)`,
		formatTime(now), formatTime(now.Add(-deliveryLifetime))); err != nil {
		return
	}
	due, err := dueDeliveries(ctx, db, now, dispatchBatch)
	if err != nil {
		return
	}
	channels := map[string]Channel{}
	for _, channel := range settings.Channels {
		channels[channel.ID] = channel
	}
	stalled := map[string]bool{}
	for _, item := range due {
		if ctx.Err() != nil || s.now().Sub(started) >= dispatchBudget {
			return
		}
		if stalled[item.ChannelID] {
			continue
		}
		channel, ok := channels[item.ChannelID]
		switch {
		case !ok:
			_ = settle(ctx, db, item.ID, item.Attempts, "dropped", "channel_removed", now, now)
			continue
		case !channel.Enabled:
			_ = settle(ctx, db, item.ID, item.Attempts, "dropped", "channel_off", now, now)
			continue
		}
		code := s.sender.send(ctx, channel, render(item.Kind, item.Incident, s.link(item.Incident.Rule), s.now()))
		if ctx.Err() != nil {
			return
		}
		if code == codeTimeout || code == codeUnreachable {
			stalled[item.ChannelID] = true
		}
		at := s.now()
		attempts := item.Attempts + 1
		switch {
		case code == "":
			_ = settle(ctx, db, item.ID, attempts, "sent", "", at, at)
		case retryable(code) && attempts < maxAttempts:
			_ = settle(ctx, db, item.ID, attempts, "pending", code, at.Add(backoff[min(attempts, len(backoff))-1]), at)
		default:
			_ = settle(ctx, db, item.ID, attempts, "failed", code, at, at)
		}
	}
}

// heartbeat pings the monitor when its interval has passed, or at once when
// force is set (an operator test). It returns the send error code.
func (s *Service) heartbeat(ctx context.Context, heartbeat Heartbeat, force bool) string {
	if heartbeat.URL == "" || heartbeat.Unreadable || (!force && !heartbeat.Enabled) {
		return codeFailed
	}
	now := s.now()
	// Passes run on a fixed tick but start a little early or late, so half a
	// tick of slack keeps a ping from slipping to the following pass.
	interval := time.Duration(heartbeat.IntervalMinutes)*time.Minute - s.interval/2
	s.mu.Lock()
	due := force || now.Sub(s.status.heartbeat.attemptAt) >= interval
	if due {
		s.status.heartbeat.attemptAt = now
	}
	s.mu.Unlock()
	if !due {
		return ""
	}
	code := s.sender.get(ctx, heartbeat.URL)
	s.mu.Lock()
	if code == "" {
		s.status.heartbeat.okAt, s.status.heartbeat.lastError = s.now(), ""
	} else {
		s.status.heartbeat.failureAt, s.status.heartbeat.lastError = s.now(), code
	}
	s.mu.Unlock()
	return code
}

// TestChannel sends a test message through one saved channel, in any state,
// and records it in the delivery history.
func (s *Service) TestChannel(ctx context.Context, channel Channel) (string, time.Duration) {
	started := s.now()
	code := s.sender.send(ctx, channel, render(KindTest, nil, s.link(""), started))
	latency := s.now().Sub(started)
	_ = recordTest(ctx, s.store.DB(), channel.ID, code, s.now())
	return code, latency
}
