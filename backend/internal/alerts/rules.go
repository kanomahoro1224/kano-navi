package alerts

import (
	"context"
	"database/sql"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// Severity orders how urgent an incident is; ntfy maps it to a priority.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

func validSeverity(value Severity) bool {
	return value == SeverityInfo || value == SeverityWarning || value == SeverityCritical
}

// Rule codes.
const (
	RuleRunFailedStreak       = "run_failed_streak"
	RuleSourceStale           = "source_stale"
	RuleLoginExpired          = "login_expired"
	RuleLoginRefreshFailed    = "login_refresh_failed"
	RuleUnreadableCredentials = "secrets_unreadable"
)

// Rule is one catalog entry. Param names its integer parameter ("runs" or
// "hours"), or is empty for a rule without one.
type Rule struct {
	Code     string   `json:"code"`
	Param    string   `json:"param,omitempty"`
	Default  int      `json:"default,omitempty"`
	Min      int      `json:"min,omitempty"`
	Max      int      `json:"max,omitempty"`
	Severity Severity `json:"severity"`
	Enabled  bool     `json:"enabled"`
}

// Catalog lists the rules in display order. A rule's conditions are fixed in
// code; the operator chooses only its switch, parameter, severity and
// whether its resolution is delivered.
var Catalog = []Rule{
	{Code: RuleRunFailedStreak, Param: "runs", Default: 3, Min: 1, Max: 20, Severity: SeverityWarning, Enabled: true},
	{Code: RuleSourceStale, Param: "hours", Default: 24, Min: 1, Max: 168, Severity: SeverityWarning, Enabled: true},
	{Code: RuleLoginExpired, Severity: SeverityCritical, Enabled: true},
	{Code: RuleLoginRefreshFailed, Severity: SeverityWarning, Enabled: true},
	{Code: RuleUnreadableCredentials, Severity: SeverityCritical, Enabled: true},
}

func ruleByCode(code string) (Rule, bool) {
	for _, rule := range Catalog {
		if rule.Code == code {
			return rule, true
		}
	}
	return Rule{}, false
}

func (r Rule) defaults() RuleSetting {
	return RuleSetting{Enabled: r.Enabled, Severity: r.Severity, Threshold: r.Default, NotifyResolved: true}
}

func (r Rule) normalize(setting RuleSetting) (RuleSetting, error) {
	if setting.Severity == "" {
		setting.Severity = r.Severity
	}
	if !validSeverity(setting.Severity) {
		return RuleSetting{}, inputError("severity must be info, warning or critical")
	}
	if r.Param == "" {
		setting.Threshold = 0
		return setting, nil
	}
	if setting.Threshold == 0 {
		setting.Threshold = r.Default
	}
	if setting.Threshold < r.Min || setting.Threshold > r.Max {
		return RuleSetting{}, inputError(r.Param + " must be between " + strconv.Itoa(r.Min) + " and " + strconv.Itoa(r.Max))
	}
	return setting, nil
}

// Signals are the checks the server process wires in from other packages.
// Each one reads local state only.
type Signals struct {
	// SkipSources lists the sources the environment turned off (SKIP_*).
	SkipSources map[string]bool
	// BilibiliLogin reports the login cookie's state as the console shows
	// it; "expired" opens login_expired. Nil without a Bilibili account.
	BilibiliLogin func(ctx context.Context) (string, error)
	// BilibiliLoginRefresh reports the failure code of the stored login's
	// latest refresh check, "" when it succeeded or no login is stored; a
	// code other than bilibili_login_expired opens login_refresh_failed.
	BilibiliLoginRefresh func(ctx context.Context) (string, error)
	// Secrets reports, by area, whether a stored secret no longer decrypts.
	Secrets map[string]func(ctx context.Context) (bool, error)
	// LiveProbe is true when the live-status probe runs in this process;
	// its runs count toward run_failed_streak only then.
	LiveProbe bool
}

// Finding is one rule holding for one subject.
type Finding struct {
	Rule    string
	Subject string
	// Detail holds the message's values: counts, hours, times and fixed
	// codes only.
	Detail map[string]any
}

const (
	// streakWindow is how far back finished runs count toward a streak, so
	// a schedule that stopped running does not stay failing forever.
	streakWindow = 7 * 24 * time.Hour
	liveWorkflow = "@live"
)

// freshSources are the sources with a public freshness time, each read by
// the source schedule of the same name.
var freshSources = []string{"x", "youtube", "bilibili", sqlite.ShopSource}

var codePattern = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

// evaluate runs one rule. An error leaves the rule's incidents as they are.
func (s *Service) evaluate(ctx context.Context, rule Rule, setting RuleSetting, settings Settings, now time.Time) ([]Finding, error) {
	switch rule.Code {
	case RuleRunFailedStreak:
		return s.failedStreaks(ctx, setting.Threshold, now)
	case RuleSourceStale:
		return s.staleSources(ctx, setting.Threshold, now)
	case RuleLoginExpired:
		if s.signals.BilibiliLogin == nil || s.signals.SkipSources["bilibili"] {
			return nil, nil
		}
		status, err := s.signals.BilibiliLogin(ctx)
		if err != nil {
			return nil, err
		}
		if status == "expired" {
			return []Finding{{Rule: rule.Code, Subject: "bilibili", Detail: map[string]any{}}}, nil
		}
		return nil, nil
	case RuleLoginRefreshFailed:
		if s.signals.BilibiliLoginRefresh == nil || s.signals.SkipSources["bilibili"] {
			return nil, nil
		}
		code, err := s.signals.BilibiliLoginRefresh(ctx)
		if err != nil {
			return nil, err
		}
		if code != "" && code != "bilibili_login_expired" && codePattern.MatchString(code) {
			return []Finding{{Rule: rule.Code, Subject: "bilibili", Detail: map[string]any{"error": code}}}, nil
		}
		return nil, nil
	case RuleUnreadableCredentials:
		return s.unreadableSecrets(ctx, settings)
	}
	return nil, nil
}

// failedStreaks finds source schedules and the live-status probe whose
// latest finished runs all failed or were blocked. Cancelled runs do not
// count either way; a schedule the operator turned off, or the probe when it
// does not run, is left out.
func (s *Service) failedStreaks(ctx context.Context, threshold int, now time.Time) ([]Finding, error) {
	db := s.store.DB()
	enabled := map[string]bool{liveWorkflow: s.signals.LiveProbe}
	rows, err := db.QueryContext(ctx, `SELECT source FROM source_schedules WHERE enabled=1`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var source string
		if err = rows.Scan(&source); err != nil {
			rows.Close()
			return nil, err
		}
		enabled[sqlite.SourceScheduleWorkflow(source)] = true
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	rows, err = db.QueryContext(ctx, `SELECT workflow_id,status,COALESCE(error_code,''),finished_at FROM (
		SELECT workflow_id,status,error_code,finished_at,
			ROW_NUMBER() OVER (PARTITION BY workflow_id ORDER BY julianday(finished_at) DESC,id DESC) AS n
		FROM workflow_runs
		WHERE status IN ('succeeded','partial','failed','blocked') AND finished_at IS NOT NULL AND julianday(finished_at) >= julianday(?)
	) WHERE n <= ? ORDER BY workflow_id, n`, sqlite.ISOTime(now.Add(-streakWindow)), threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type streak struct {
		count  int
		broken bool
		code   string
		since  string
	}
	streaks := map[string]*streak{}
	order := []string{}
	for rows.Next() {
		var workflow, status, code, finished string
		if err = rows.Scan(&workflow, &status, &code, &finished); err != nil {
			return nil, err
		}
		if !enabled[workflow] {
			continue
		}
		current := streaks[workflow]
		if current == nil {
			current = &streak{}
			streaks[workflow] = current
			order = append(order, workflow)
			if codePattern.MatchString(code) {
				current.code = code
			}
		}
		if current.broken {
			continue
		}
		if status == "failed" || status == "blocked" {
			current.count++
			current.since = finished
		} else {
			current.broken = true
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := []Finding{}
	for _, workflow := range order {
		current := streaks[workflow]
		if current.count < threshold {
			continue
		}
		subject := "live"
		if source, ok := sqlite.SourceScheduleOf(workflow); ok {
			subject = source
		}
		detail := map[string]any{"runs": current.count, "since": current.since}
		if current.code != "" {
			detail["error"] = current.code
		}
		out = append(out, Finding{Rule: RuleRunFailedStreak, Subject: subject, Detail: detail})
	}
	return out, nil
}

// staleSources finds enabled source schedules without a successful read for
// threshold hours. A source never read counts from when its schedule last
// changed.
func (s *Service) staleSources(ctx context.Context, threshold int, now time.Time) ([]Finding, error) {
	fetched, err := s.store.SourceFetchedAt(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.DB().QueryContext(ctx, `SELECT source,updated_at FROM source_schedules WHERE enabled=1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	changed := map[string]string{}
	for rows.Next() {
		var source string
		var updated sql.NullString
		if err = rows.Scan(&source, &updated); err != nil {
			return nil, err
		}
		changed[source] = updated.String
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	limit := time.Duration(threshold) * time.Hour
	out := []Finding{}
	for _, source := range freshSources {
		since, scheduled := changed[source]
		if !scheduled || s.signals.SkipSources[source] {
			continue
		}
		detail := map[string]any{}
		if at := fetched[source]; at != nil {
			since = *at
			detail["lastReadAt"] = *at
		}
		at, parseErr := time.Parse(time.RFC3339Nano, since)
		if parseErr != nil || now.Sub(at) < limit {
			continue
		}
		detail["hours"] = int(now.Sub(at) / time.Hour)
		out = append(out, Finding{Rule: RuleSourceStale, Subject: source, Detail: detail})
	}
	return out, nil
}

// unreadableSecrets reports each area with a stored secret that no longer
// decrypts, including the alert channels and heartbeat themselves.
func (s *Service) unreadableSecrets(ctx context.Context, settings Settings) ([]Finding, error) {
	areas := make([]string, 0, len(s.signals.Secrets))
	for area := range s.signals.Secrets {
		areas = append(areas, area)
	}
	sort.Strings(areas)
	out := []Finding{}
	for _, area := range areas {
		unreadable, err := s.signals.Secrets[area](ctx)
		if err != nil {
			return nil, err
		}
		if unreadable {
			out = append(out, Finding{Rule: RuleUnreadableCredentials, Subject: area, Detail: map[string]any{}})
		}
	}
	own := settings.Heartbeat.Unreadable
	for _, channel := range settings.Channels {
		own = own || channel.Unreadable
	}
	if own {
		out = append(out, Finding{Rule: RuleUnreadableCredentials, Subject: "alerts", Detail: map[string]any{}})
	}
	return out, nil
}
