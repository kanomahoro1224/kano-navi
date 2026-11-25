package alerts

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestAFailedLoginRefreshWarnsUntilTheLoginRefreshesAgain(t *testing.T) {
	store := openStore(t)
	code := "bilibili_login_refresh_rejected"
	service, now := newTestService(t, store, true, Signals{BilibiliLoginRefresh: func(context.Context) (string, error) { return code, nil }})
	configure(t, service)
	service.Tick(context.Background())
	found := incidents(t, store)
	if len(found) != 1 || found[0].Rule != RuleLoginRefreshFailed || found[0].Subject != "bilibili" || found[0].Severity != SeverityWarning || found[0].Detail["error"] != code {
		t.Fatalf("incidents=%+v", found)
	}
	if out := render(KindOpened, &found[0], "", now.at); !strings.Contains(out.Title, "could not be refreshed") || !strings.Contains(out.Body, code) {
		t.Fatalf("message=%+v", out)
	}
	// An expired login is login_expired's incident, not this one's.
	code = "bilibili_login_expired"
	now.at = now.at.Add(time.Minute)
	service.Tick(context.Background())
	if found = incidents(t, store); len(found) != 1 || found[0].Status != "resolved" {
		t.Fatalf("incidents=%+v", found)
	}
}
