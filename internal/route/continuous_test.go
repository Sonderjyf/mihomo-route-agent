package route

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestContinuousRenewalBeyondTenMinutesAndPausedWindow(t *testing.T) {
	o, _, _ := controlledFixture(t)
	if err := o.EnableContinuous(true); err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	started := clock
	o.control.clock = func() time.Time { return clock }
	o.ready.Store(true)
	for i := 0; i < 31; i++ {
		if i == 4 {
			o.control.maintenance = &maintenanceWindow{config: MaintenanceConfig{"00:00", "UTC", 3600, "manual"}, start: clock.Add(-time.Second), end: clock.Add(20 * time.Minute)}
			if err := o.maintain(context.Background(), clock); err != nil {
				t.Fatal(err)
			}
		}
		if err := o.renew(context.Background()); err != nil {
			t.Fatalf("minute %d: %v", i, err)
		}
		body, err := os.ReadFile(o.control.path)
		if err != nil {
			t.Fatal(err)
		}
		var disk Ownership
		if json.Unmarshal(body, &disk) != nil || !disk.Expires.Equal(o.control.lease.Expires) || disk.Expires.Sub(clock) > continuousLease {
			t.Fatal("invalid durable short renewal")
		}
		clock = clock.Add(time.Minute)
	}
	if clock.Sub(started) <= 10*time.Minute || !o.control.paused {
		t.Fatal("long paused session not exercised")
	}
	o.control.maintenance.config.Resume = "unchanged"
	if err := o.maintain(context.Background(), clock); err != nil {
		t.Fatal("resume after thirty minutes", err)
	}
	if !o.ready.Load() || o.control.paused {
		t.Fatal("not resumed")
	}
	clock = o.control.lease.Expires
	if err := o.renew(context.Background()); err == nil {
		t.Fatal("revived expired lease")
	}
	if err := o.control.check(); err == nil {
		t.Fatal("expired writes permitted")
	}
}

func TestContinuousRenewalRejectsChangedIdentityAndLostLease(t *testing.T) {
	for _, kind := range []string{"file", "core", "lease-file", "write-failure"} {
		t.Run(kind, func(t *testing.T) {
			o, guard, _ := controlledFixture(t)
			if err := o.EnableContinuous(true); err != nil {
				t.Fatal(err)
			}
			before := o.control.lease.Expires
			switch kind {
			case "file":
				os.WriteFile(o.control.lease.ConfigPath, []byte("changed"), 0600)
			case "core":
				guard.restarted.Store(true)
			case "lease-file":
				os.Remove(o.control.path)
			case "write-failure":
				os.Remove(o.control.path)
				os.Mkdir(o.control.path, 0700)
			}
			if err := o.renew(context.Background()); err == nil {
				t.Fatal("unsafe renewal")
			}
			if !o.control.lease.Expires.Equal(before) {
				t.Fatal("memory lease changed after failure")
			}
		})
	}
}

func TestFailedOwnedExitRetainsRecoveryLock(t *testing.T) {
	o, guard, _ := controlledFixture(t)
	if err := o.EnableContinuous(true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- o.Run(ctx, o.statePath) }()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for !o.ready.Load() {
		select {
		case err := <-done:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal("not ready")
		case <-tick.C:
		}
	}
	guard.restarted.Store(true)
	if err := <-done; err == nil {
		t.Fatal("changed core accepted")
	}
	if _, err := os.Stat(o.control.path + ".lock"); err != nil {
		t.Fatal("failed cleanup lost process ownership evidence", err)
	}
	data, err := os.ReadFile(o.statePath)
	if err != nil {
		t.Fatal(err)
	}
	var journal observerJournal
	if json.Unmarshal(data, &journal) != nil || journal.Phase != "recovery_required" {
		t.Fatal("missing recovery state")
	}
}
