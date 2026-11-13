package sqlite

import (
	"context"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// A rotated token replaces the device's previous row: Huawei reissues a token
// on reinstall, and two rows for one phone would deliver every update twice.
func TestPushDeviceRegistrationKeepsOneTokenPerDevice(t *testing.T) {
	path := testutil.NewSnapshot(t)
	store, err := OpenAdminWrites(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	first := PushDevice{Token: "token-a", DeviceID: "device-1", Locale: "zh-CN", AppVersion: "1.0.0",
		Enabled: true, NotifyLive: true, NotifyVideo: false, NotifyPost: true}
	if err = store.RegisterPushDevice(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Token = "example-b"
	if err = store.RegisterPushDevice(ctx, second); err != nil {
		t.Fatal(err)
	}
	devices, err := store.PushDevices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Token != "example-b" || devices[0].DeviceID != "device-1" || devices[0].NotifyVideo || !devices[0].NotifyPost {
		t.Fatalf("devices = %+v", devices)
	}
	// The master switch stops a device from being read at all.
	second.Enabled = false
	if err = store.RegisterPushDevice(ctx, second); err != nil {
		t.Fatal(err)
	}
	if devices, err = store.PushDevices(ctx); err != nil || len(devices) != 0 {
		t.Fatalf("disabled device read as %+v (%v)", devices, err)
	}
	if err = store.DeletePushDevices(ctx, []string{"example-b"}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM push_devices`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rows=%d err=%v", count, err)
	}
}
