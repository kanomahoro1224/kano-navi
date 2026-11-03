package sqlite

import (
	"context"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// A browser refreshes its keys and switches without becoming a second row, and
// a re-subscribe revives a row that repeated failures had disabled.
func TestWebPushSubscriptionUpsertKeepsOneRow(t *testing.T) {
	path := testutil.NewSnapshot(t)
	store, err := OpenAdminWrites(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	first := WebPushSubscription{
		Endpoint: "https://fcm.googleapis.com/wp/one", P256DH: "key-one", Auth: "auth-one",
		Locale: "zh-CN", UserAgent: "Chrome", Enabled: true,
		NotifyLive: true, NotifyVideo: false, NotifyPost: true,
	}
	if err = store.RegisterWebPushSubscription(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.P256DH, second.Auth = "key-two", "auth-two"
	second.Locale, second.NotifyVideo = "ja", true
	if err = store.RegisterWebPushSubscription(ctx, second); err != nil {
		t.Fatal(err)
	}
	subs, err := store.WebPushSubscriptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].P256DH != "key-two" || subs[0].Locale != "ja" || !subs[0].NotifyVideo {
		t.Fatalf("subs = %+v", subs)
	}
	if subs[0].CreatedAt == "" || subs[0].UpdatedAt == "" || subs[0].LastSeenAt == "" {
		t.Fatalf("timestamps missing: %+v", subs[0])
	}
	// The master switch hides the row from dispatch but not from the console.
	second.Enabled = false
	if err = store.RegisterWebPushSubscription(ctx, second); err != nil {
		t.Fatal(err)
	}
	if subs, err = store.WebPushSubscriptions(ctx); err != nil || len(subs) != 0 {
		t.Fatalf("disabled read as %+v (%v)", subs, err)
	}
	if subs, err = store.AdminWebPushSubscriptions(ctx, 10); err != nil || len(subs) != 1 || subs[0].Enabled {
		t.Fatalf("admin read as %+v (%v)", subs, err)
	}
	if err = store.DeleteWebPushSubscription(ctx, first.Endpoint); err != nil {
		t.Fatal(err)
	}
	if total, enabled, err := store.WebPushSubscriptionStats(ctx); err != nil || total != 0 || enabled != 0 {
		t.Fatalf("stats = %d/%d (%v)", total, enabled, err)
	}
}

// A push service that keeps failing retires the row, so it stops consuming
// every run, and a success clears the counter.
func TestWebPushFailuresDisableAndClear(t *testing.T) {
	path := testutil.NewSnapshot(t)
	store, err := OpenAdminWrites(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	sub := WebPushSubscription{
		Endpoint: "https://updates.push.services.mozilla.com/wpush/v2/two",
		P256DH:   "key", Auth: "auth", Enabled: true,
		NotifyLive: true, NotifyVideo: true, NotifyPost: true,
	}
	if err = store.RegisterWebPushSubscription(ctx, sub); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = store.RecordWebPushFailure(ctx, sub.Endpoint, 3); err != nil {
			t.Fatal(err)
		}
	}
	if subs, err := store.WebPushSubscriptions(ctx); err != nil || len(subs) != 1 || subs[0].FailureCount != 2 {
		t.Fatalf("subs = %+v (%v)", subs, err)
	}
	if err = store.RecordWebPushFailure(ctx, sub.Endpoint, 3); err != nil {
		t.Fatal(err)
	}
	if subs, err := store.WebPushSubscriptions(ctx); err != nil || len(subs) != 0 {
		t.Fatalf("disabled read as %+v (%v)", subs, err)
	}
	sub.P256DH, sub.Auth = "key-new", "auth-new"
	if err = store.RegisterWebPushSubscription(ctx, sub); err != nil {
		t.Fatal(err)
	}
	subs, err := store.WebPushSubscriptions(ctx)
	if err != nil || len(subs) != 1 || subs[0].FailureCount != 0 || !subs[0].Enabled {
		t.Fatalf("revived as %+v (%v)", subs, err)
	}
	if err = store.RecordWebPushFailure(ctx, sub.Endpoint, 3); err != nil {
		t.Fatal(err)
	}
	if err = store.ClearWebPushFailures(ctx, sub.Endpoint); err != nil {
		t.Fatal(err)
	}
	if subs, err = store.WebPushSubscriptions(ctx); err != nil || subs[0].FailureCount != 0 {
		t.Fatalf("after clear: %+v (%v)", subs, err)
	}
}
