package alerts

import (
	"context"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// A succeeded window of 0 keeps sent deliveries, and with them their
// incident, however old; failed and dropped ones follow the failed window.
func TestPruneKeepsSentDeliveriesWhenTheSucceededWindowKeepsThem(t *testing.T) {
	store := openStore(t)
	now := time.Date(2026, 11, 21, 3, 0, 0, 0, time.UTC)
	old := formatTime(now.AddDate(0, 0, -60))
	exec(t, store, `INSERT INTO alert_incidents(id,rule,subject,severity,status,detail_json,opened_at,last_seen_at,resolved_at) VALUES
		(1,'source_stale','x','warning','resolved','{}',?,?,?),
		(2,'source_stale','youtube','warning','resolved','{}',?,?,?)`, old, old, old, old, old, old)
	exec(t, store, `INSERT INTO alert_deliveries(incident_id,channel_id,kind,status,attempts,next_attempt_at,created_at,finished_at) VALUES
		(1,'synthetic','opened','sent',1,?,?,?),
		(2,'synthetic','opened','failed',5,?,?,?)`, old, old, old, old, old, old)
	count := func(query string) int {
		t.Helper()
		var n int
		if err := store.DB().QueryRow(query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err := prune(context.Background(), store.DB(), sqlite.RunRetention{SucceededDays: 0, FailedDays: 30}, now); err != nil {
		t.Fatal(err)
	}
	if got := count(`SELECT COUNT(*) FROM alert_deliveries WHERE status='sent'`); got != 1 {
		t.Fatalf("sent deliveries=%d, want the kept one", got)
	}
	if got := count(`SELECT COUNT(*) FROM alert_deliveries WHERE status='failed'`); got != 0 {
		t.Fatalf("failed deliveries=%d, want none past the failed window", got)
	}
	if got := count(`SELECT group_concat(id) FROM alert_incidents`); got != 1 {
		t.Fatalf("incidents left=%d, want the one a kept delivery names", got)
	}
}
