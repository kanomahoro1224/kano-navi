package alerts

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const testKey = "synthetic-llm-secrets-key"

func openStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.OpenStandalone(filepath.Join(t.TempDir(), "database", "kano.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// testGuard lets tests reach their plain-HTTP loopback servers.
func testGuard() guard {
	return guard{allow: func(netip.Addr) bool { return true }, resolver: net.DefaultResolver, insecure: true}
}

type clock struct{ at time.Time }

func (c *clock) now() time.Time { return c.at }

func newTestService(t *testing.T, store *sqlite.Store, production bool, signals Signals) (*Service, *clock) {
	t.Helper()
	service := newService(Options{Store: store, Key: testKey, Enabled: true, Production: production, Signals: signals}, testGuard())
	now := &clock{at: time.Date(2026, 11, 21, 3, 0, 0, 0, time.UTC)}
	service.now = now.now
	return service, now
}

// recorder is a synthetic channel endpoint that answers with status.
type recorder struct {
	mu       sync.Mutex
	status   int
	requests []recorded
}

type recorded struct {
	Path    string
	Headers http.Header
	Body    []byte
}

func (r *recorder) server(t *testing.T) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		r.mu.Lock()
		r.requests = append(r.requests, recorded{Path: request.URL.Path, Headers: request.Header.Clone(), Body: body})
		status := r.status
		r.mu.Unlock()
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return server
}

func (r *recorder) all() []recorded {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recorded(nil), r.requests...)
}

func exec(t *testing.T, store *sqlite.Store, query string, args ...any) {
	t.Helper()
	if _, err := store.DB().Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func enableSchedule(t *testing.T, store *sqlite.Store, source string, updated time.Time) {
	exec(t, store, `INSERT INTO source_schedules(source,enabled,interval_minutes,updated_at) VALUES(?,1,30,?)
		ON CONFLICT(source) DO UPDATE SET enabled=1,updated_at=excluded.updated_at`, source, sqlite.ISOTime(updated))
}

func addRun(t *testing.T, store *sqlite.Store, id, workflow, status, code string, finished time.Time) {
	at := sqlite.ISOTime(finished)
	var errorCode any
	if code != "" {
		errorCode = code
	}
	exec(t, store, `INSERT INTO workflow_runs(id,workflow_id,status,trigger_source,execution_source,created_at,started_at,finished_at,error_code) VALUES(?,?,?,'scheduler','scheduler',?,?,?,?)`,
		id, workflow, status, at, at, at, errorCode)
}

func webhookChannel(url, secret string) Channel {
	return Channel{ID: "hook", Name: "Synthetic hook", Type: TypeWebhook, Enabled: true, URL: url, Secret: secret}
}

func configure(t *testing.T, service *Service, channels ...Channel) {
	t.Helper()
	settings, err := Normalize(Settings{})
	if err != nil {
		t.Fatal(err)
	}
	settings.Channels = channels
	service.set(settings)
}

type deliveryRow struct {
	Kind, Status, Error string
	Attempts            int
}

func deliveries(t *testing.T, store *sqlite.Store) []deliveryRow {
	t.Helper()
	rows, err := store.DB().Query(`SELECT kind,status,COALESCE(last_error,''),attempts FROM alert_deliveries ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []deliveryRow{}
	for rows.Next() {
		var row deliveryRow
		if err = rows.Scan(&row.Kind, &row.Status, &row.Error, &row.Attempts); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	return out
}

func incidents(t *testing.T, store *sqlite.Store) []incident {
	t.Helper()
	rows, err := store.DB().Query(`SELECT ` + incidentColumns + ` FROM alert_incidents ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []incident{}
	for rows.Next() {
		item, err := scanIncident(rows)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, item)
	}
	return out
}

func TestFailedStreakOpensDeliversAndResolves(t *testing.T) {
	store := openStore(t)
	service, now := newTestService(t, store, true, Signals{})
	endpoint := &recorder{}
	server := endpoint.server(t)
	configure(t, service, webhookChannel(server.URL+"/hook", "synthetic-signing-key"))
	enableSchedule(t, store, "x", now.at.Add(-time.Hour))
	for index := range 3 {
		addRun(t, store, "run-"+string(rune('a'+index)), sqlite.SourceScheduleWorkflow("x"), "failed", "x_source_failed", now.at.Add(-time.Duration(30*(3-index))*time.Minute))
	}

	service.Tick(context.Background())
	found := incidents(t, store)
	if len(found) != 1 || found[0].Rule != RuleRunFailedStreak || found[0].Subject != "x" || found[0].Status != "open" {
		t.Fatalf("incidents=%+v", found)
	}
	requests := endpoint.all()
	if len(requests) != 1 {
		t.Fatalf("requests=%d", len(requests))
	}
	var payload WebhookPayload
	if err := json.Unmarshal(requests[0].Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Kind != KindOpened || payload.Rule != RuleRunFailedStreak || payload.Subject != "x" || payload.Title != "X runs are failing" {
		t.Fatalf("payload=%+v", payload)
	}
	if !strings.Contains(payload.Message, "The last 3 runs failed (x_source_failed).") || !strings.Contains(payload.Message, "2026-11-21 12:00 JST") {
		t.Fatalf("message=%q", payload.Message)
	}
	if got := requests[0].Headers.Get(signatureName); got != Signature("synthetic-signing-key", requests[0].Body) {
		t.Fatalf("signature=%q", got)
	}

	// A further pass while the streak holds sends nothing new.
	now.at = now.at.Add(time.Minute)
	service.Tick(context.Background())
	if len(endpoint.all()) != 1 {
		t.Fatalf("an open incident was delivered again")
	}

	addRun(t, store, "run-ok", sqlite.SourceScheduleWorkflow("x"), "succeeded", "", now.at)
	now.at = now.at.Add(time.Minute)
	service.Tick(context.Background())
	found = incidents(t, store)
	if found[0].Status != "resolved" {
		t.Fatalf("incident=%+v", found[0])
	}
	requests = endpoint.all()
	if len(requests) != 2 {
		t.Fatalf("requests=%d", len(requests))
	}
	if err := json.Unmarshal(requests[1].Body, &payload); err != nil || payload.Kind != KindResolved || payload.Title != "X runs succeed again" {
		t.Fatalf("payload=%+v err=%v", payload, err)
	}
	if rows := deliveries(t, store); len(rows) != 2 || rows[0].Status != "sent" || rows[1].Status != "sent" {
		t.Fatalf("deliveries=%+v", rows)
	}
}

func TestStreakIgnoresCancelledRunsAndSchedulesTurnedOff(t *testing.T) {
	store := openStore(t)
	service, now := newTestService(t, store, true, Signals{})
	configure(t, service)
	workflow := sqlite.SourceScheduleWorkflow("youtube")
	addRun(t, store, "r1", workflow, "failed", "", now.at.Add(-3*time.Hour))
	addRun(t, store, "r2", workflow, "cancelled", "", now.at.Add(-2*time.Hour))
	addRun(t, store, "r3", workflow, "blocked", "", now.at.Add(-90*time.Minute))
	addRun(t, store, "r4", workflow, "failed", "", now.at.Add(-time.Hour))
	service.Tick(context.Background())
	if found := incidents(t, store); len(found) != 0 {
		t.Fatalf("a schedule that is not enabled opened %+v", found)
	}
	enableSchedule(t, store, "youtube", now.at.Add(-time.Hour))
	service.Tick(context.Background())
	found := incidents(t, store)
	if len(found) != 1 || detailInt(found[0].Detail, "runs") != 3 {
		t.Fatalf("incidents=%+v", found)
	}
}

func TestStaleSourceRecordsWithoutSendingInDevelopment(t *testing.T) {
	store := openStore(t)
	service, now := newTestService(t, store, false, Signals{SkipSources: map[string]bool{"shop": true}})
	endpoint := &recorder{}
	server := endpoint.server(t)
	configure(t, service, webhookChannel(server.URL+"/hook", ""))
	enableSchedule(t, store, "youtube", now.at.Add(-30*time.Hour))
	enableSchedule(t, store, "shop", now.at.Add(-30*time.Hour))
	enableSchedule(t, store, "bilibili", now.at.Add(-2*time.Hour))
	service.Tick(context.Background())
	found := incidents(t, store)
	if len(found) != 1 || found[0].Rule != RuleSourceStale || found[0].Subject != "youtube" || detailInt(found[0].Detail, "hours") != 30 {
		t.Fatalf("incidents=%+v", found)
	}
	if rows := deliveries(t, store); len(rows) != 0 {
		t.Fatalf("development mode queued %+v", rows)
	}
	if len(endpoint.all()) != 0 {
		t.Fatal("development mode sent a delivery")
	}
	if service.State() != StateDevelopment {
		t.Fatalf("state=%s", service.State())
	}
}

func TestFailedDeliveriesRetryOnlyWhenRetryable(t *testing.T) {
	store := openStore(t)
	service, now := newTestService(t, store, true, Signals{BilibiliLogin: func(context.Context) (string, error) { return "expired", nil }})
	busy := &recorder{status: http.StatusServiceUnavailable}
	refused := &recorder{status: http.StatusBadRequest}
	busyServer, refusedServer := busy.server(t), refused.server(t)
	configure(t, service,
		Channel{ID: "busy", Type: TypeWebhook, Enabled: true, URL: busyServer.URL},
		Channel{ID: "refused", Type: TypeNtfy, Enabled: true, Server: refusedServer.URL, Topic: "synthetic"})
	service.Tick(context.Background())
	rows := deliveries(t, store)
	if len(rows) != 2 || rows[0] != (deliveryRow{KindOpened, "pending", codeServer, 1}) || rows[1] != (deliveryRow{KindOpened, "failed", codeClient, 1}) {
		t.Fatalf("deliveries=%+v", rows)
	}
	// Not due before its backoff.
	now.at = now.at.Add(10 * time.Second)
	service.Tick(context.Background())
	if len(busy.all()) != 1 {
		t.Fatalf("retried before the backoff")
	}
	busy.mu.Lock()
	busy.status = http.StatusOK
	busy.mu.Unlock()
	now.at = now.at.Add(30 * time.Second)
	service.Tick(context.Background())
	if rows = deliveries(t, store); rows[0] != (deliveryRow{KindOpened, "sent", "", 2}) {
		t.Fatalf("deliveries=%+v", rows)
	}
	var published map[string]any
	if err := json.Unmarshal(refused.all()[0].Body, &published); err != nil || published["topic"] != "synthetic" || published["priority"] != float64(5) {
		t.Fatalf("ntfy body=%v err=%v", published, err)
	}
}

func TestTelegramTokenStaysInThePathOnly(t *testing.T) {
	store := openStore(t)
	service, _ := newTestService(t, store, true, Signals{})
	endpoint := &recorder{}
	server := endpoint.server(t)
	service.sender.telegram = server.URL
	channel := Channel{ID: "tg", Type: TypeTelegram, Enabled: true, ChatID: "-100123", Secret: "synthetic-bot:synthetic-secret"}
	code, _ := service.TestChannel(context.Background(), channel)
	if code != "" {
		t.Fatalf("code=%s", code)
	}
	request := endpoint.all()[0]
	if request.Path != "/bot"+channel.Secret+"/sendMessage" || strings.Contains(string(request.Body), channel.Secret) {
		t.Fatalf("path=%s body=%s", request.Path, request.Body)
	}
	var stored string
	if err := store.DB().QueryRow(`SELECT kind||':'||status FROM alert_deliveries`).Scan(&stored); err != nil || stored != "test:sent" {
		t.Fatalf("stored=%s err=%v", stored, err)
	}
}

func TestTurningARuleOffResolvesItsIncidentSilently(t *testing.T) {
	store := openStore(t)
	service, now := newTestService(t, store, true, Signals{Secrets: map[string]func(context.Context) (bool, error){
		"proxy": func(context.Context) (bool, error) { return true, nil },
	}})
	endpoint := &recorder{}
	server := endpoint.server(t)
	configure(t, service, webhookChannel(server.URL, ""))
	service.Tick(context.Background())
	if found := incidents(t, store); len(found) != 1 || found[0].Subject != "proxy" {
		t.Fatalf("incidents=%+v", found)
	}
	settings, _ := service.Settings(context.Background())
	rule := settings.Rules[RuleUnreadableCredentials]
	rule.Enabled = false
	settings.Rules[RuleUnreadableCredentials] = rule
	service.set(settings)
	now.at = now.at.Add(time.Minute)
	service.Tick(context.Background())
	if found := incidents(t, store); found[0].Status != "resolved" {
		t.Fatalf("incident=%+v", found[0])
	}
	if len(endpoint.all()) != 1 {
		t.Fatalf("requests=%d", len(endpoint.all()))
	}
}

func TestHeartbeatPingsOnItsInterval(t *testing.T) {
	store := openStore(t)
	service, now := newTestService(t, store, true, Signals{})
	endpoint := &recorder{}
	server := endpoint.server(t)
	settings, _ := Normalize(Settings{})
	settings.Heartbeat = Heartbeat{Enabled: true, IntervalMinutes: 5, URL: server.URL + "/push/synthetic"}
	service.set(settings)
	service.Tick(context.Background())
	now.at = now.at.Add(time.Minute)
	service.Tick(context.Background())
	now.at = now.at.Add(4 * time.Minute)
	service.Tick(context.Background())
	if got := len(endpoint.all()); got != 2 {
		t.Fatalf("pings=%d", got)
	}
}

func TestHeartbeatToleratesAPassThatStartsEarly(t *testing.T) {
	store := openStore(t)
	service, now := newTestService(t, store, true, Signals{})
	endpoint := &recorder{}
	server := endpoint.server(t)
	settings, _ := Normalize(Settings{})
	settings.Heartbeat = Heartbeat{Enabled: true, IntervalMinutes: 5, URL: server.URL + "/push/synthetic"}
	service.set(settings)
	service.Tick(context.Background())
	// The fifth pass starts a few milliseconds sooner after the last ping
	// than the interval; it still pings instead of waiting another minute.
	now.at = now.at.Add(5*time.Minute - 50*time.Millisecond)
	service.Tick(context.Background())
	if got := len(endpoint.all()); got != 2 {
		t.Fatalf("pings=%d", got)
	}
}

func TestAnUnreachableChannelGetsOneAttemptPerPass(t *testing.T) {
	store := openStore(t)
	service, _ := newTestService(t, store, true, Signals{
		BilibiliLogin: func(context.Context) (string, error) { return "expired", nil },
		Secrets: map[string]func(context.Context) (bool, error){
			"proxy": func(context.Context) (bool, error) { return true, nil },
		},
	})
	closed := httptest.NewServer(http.NotFoundHandler())
	address := closed.URL
	closed.Close()
	configure(t, service, webhookChannel(address+"/hook", ""))
	service.Tick(context.Background())
	rows := deliveries(t, store)
	if len(rows) != 2 || rows[0] != (deliveryRow{KindOpened, "pending", codeUnreachable, 1}) || rows[1] != (deliveryRow{KindOpened, "pending", "", 0}) {
		t.Fatalf("deliveries=%+v", rows)
	}
}

func TestLiveProbeStreaksCountOnlyWhileTheProbeRuns(t *testing.T) {
	for _, running := range []bool{false, true} {
		store := openStore(t)
		service, now := newTestService(t, store, true, Signals{LiveProbe: running})
		configure(t, service)
		for index := range 3 {
			addRun(t, store, "live-"+string(rune('a'+index)), liveWorkflow, "failed", "", now.at.Add(-time.Duration(index+1)*time.Hour))
		}
		service.Tick(context.Background())
		found := incidents(t, store)
		if running && (len(found) != 1 || found[0].Subject != "live") {
			t.Fatalf("probe running: incidents=%+v", found)
		}
		if !running && len(found) != 0 {
			t.Fatalf("probe off: incidents=%+v", found)
		}
	}
}

func TestTurningARuleOffDropsItsQueuedDeliveries(t *testing.T) {
	store := openStore(t)
	service, now := newTestService(t, store, true, Signals{Secrets: map[string]func(context.Context) (bool, error){
		"proxy": func(context.Context) (bool, error) { return true, nil },
	}})
	busy := &recorder{status: http.StatusServiceUnavailable}
	configure(t, service, webhookChannel(busy.server(t).URL, ""))
	service.Tick(context.Background())
	if rows := deliveries(t, store); len(rows) != 1 || rows[0].Status != "pending" {
		t.Fatalf("deliveries=%+v", rows)
	}
	settings, _ := service.Settings(context.Background())
	rule := settings.Rules[RuleUnreadableCredentials]
	rule.Enabled = false
	settings.Rules[RuleUnreadableCredentials] = rule
	service.set(settings)
	now.at = now.at.Add(time.Hour)
	service.Tick(context.Background())
	if rows := deliveries(t, store); len(rows) != 1 || rows[0] != (deliveryRow{KindOpened, "dropped", "rule_off", 1}) {
		t.Fatalf("deliveries=%+v", rows)
	}
	if len(busy.all()) != 1 {
		t.Fatalf("requests=%d", len(busy.all()))
	}
}

func TestAnOpenIncidentFollowsTheRuleSeverity(t *testing.T) {
	store := openStore(t)
	service, now := newTestService(t, store, true, Signals{BilibiliLogin: func(context.Context) (string, error) { return "expired", nil }})
	configure(t, service)
	service.Tick(context.Background())
	settings, _ := service.Settings(context.Background())
	rule := settings.Rules[RuleLoginExpired]
	rule.Severity = SeverityInfo
	settings.Rules[RuleLoginExpired] = rule
	service.set(settings)
	now.at = now.at.Add(time.Minute)
	service.Tick(context.Background())
	if found := incidents(t, store); len(found) != 1 || found[0].Severity != SeverityInfo {
		t.Fatalf("incidents=%+v", found)
	}
}
