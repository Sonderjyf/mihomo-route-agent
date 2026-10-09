package route

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMaintenanceValidationAndCrossMidnight(t *testing.T) {
	valid := MaintenanceConfig{"23:55", "Asia/Shanghai", 600, "manual"}
	for _, bad := range []MaintenanceConfig{
		{}, {"3:00", "UTC", 60, "manual"}, {"24:00", "UTC", 60, "manual"},
		{"03:00", "Local", 60, "manual"}, {"03:00", "UTC", 0, "manual"},
		{"03:00", "UTC", 86400, "manual"}, {"03:00", "UTC", 60, ""},
	} {
		if _, err := newMaintenance(bad, time.Now()); err == nil {
			t.Fatalf("accepted %#v", bad)
		}
	}
	for _, test := range []struct {
		utc, start string
		due        bool
	}{
		{"2026-10-08T15:54:59Z", "2026-10-08T15:55:00Z", false},
		{"2026-10-08T15:55:00Z", "2026-10-08T15:55:00Z", true},
		{"2026-10-08T16:04:59Z", "2026-10-08T15:55:00Z", true},
		{"2026-10-08T16:05:00Z", "2026-10-09T15:55:00Z", false},
	} {
		now, _ := time.Parse(time.RFC3339, test.utc)
		m, err := newMaintenance(valid, now)
		if err != nil || m.start.UTC().Format(time.RFC3339) != test.start || m.due(now) != test.due {
			t.Fatalf("%s: %#v %v", test.utc, m, err)
		}
	}
}

func TestMaintenanceDrainsAndRevalidatesBeforeResume(t *testing.T) {
	for _, change := range []string{"none", "config", "core", "lease", "path", "manual"} {
		t.Run(change, func(t *testing.T) {
			o, guard, puts := controlledFixture(t)
			if err := o.poll(context.Background()); err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			m := &maintenanceWindow{config: MaintenanceConfig{"00:00", "UTC", 1, "unchanged"}, start: now.Add(-time.Second), end: now.Add(time.Second)}
			o.control.maintenance = m
			o.ready.Store(true)
			if err := o.maintain(context.Background(), now); err != nil {
				t.Fatal(err)
			}
			if _, ok := o.Providers.Lookup("example.com"); ok || puts.Load() != 4 || !o.control.paused || o.ready.Load() || o.maintenanceState.Load() != "paused_empty" {
				t.Fatal("not drained")
			}
			if err := o.Providers.beforeWrite(context.Background(), map[string]Entry{"example.com": {Decision: Direct}}); err == nil {
				t.Fatal("write admitted in window")
			}
			switch change {
			case "config":
				if err := os.WriteFile(o.control.lease.ConfigPath, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "core":
				guard.restarted.Store(true)
			case "lease":
				o.control.lease.Expires = now.Add(-time.Second)
			case "path":
				guard.blocked.Store(true)
			case "manual":
				m.config.Resume = "manual"
			}
			err := o.maintain(context.Background(), m.end)
			if change == "none" {
				if err != nil || o.control.paused || !o.ready.Load() || puts.Load() != 6 || o.attempts.Load() != 1 {
					t.Fatal("unchanged resume failed", err)
				}
			} else if change == "manual" {
				if err != nil || !o.control.paused || puts.Load() != 4 {
					t.Fatal("manual pause resumed", err)
				}
			} else if err == nil || !o.control.paused || o.ready.Load() || puts.Load() != 4 {
				t.Fatal("changed ownership resumed", err)
			}
		})
	}
}

func TestMaintenanceRunStartsInWindowWithoutLearning(t *testing.T) {
	o, _, _ := controlledFixture(t)
	now := time.Now()
	o.control.maintenance = &maintenanceWindow{config: MaintenanceConfig{"00:00", "UTC", 60, "manual"}, start: now.Add(-time.Second), end: now.Add(time.Minute)}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- o.Run(ctx, o.statePath) }()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for o.maintenanceState.Load() != "paused_empty" {
		select {
		case err := <-done:
			t.Fatal("exited before drain", err)
		case <-ctx.Done():
			t.Fatal("no maintenance drain")
		case <-tick.C:
		}
	}
	if o.attempts.Load() != 0 || o.commits.Load() != 0 {
		t.Fatal("learned during startup window")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceFailedDrainIsNotAcknowledged(t *testing.T) {
	o, guard, _ := controlledFixture(t)
	o.ready.Store(true)
	now := time.Now()
	o.control.maintenance = &maintenanceWindow{config: MaintenanceConfig{"00:00", "UTC", 60, "manual"}, start: now.Add(-time.Second), end: now.Add(time.Minute)}
	guard.restarted.Store(true)
	if err := o.maintain(context.Background(), now); err == nil {
		t.Fatal("drain accepted changed core")
	}
	if o.control.paused || o.control.maintenance.paused || o.maintenanceState.Load() == "paused_empty" {
		t.Fatal("failed drain acknowledged")
	}
}

type maintenanceBlockedJudge struct{ called chan struct{} }

func (j maintenanceBlockedJudge) Decide(ctx context.Context, _ State) (Answer, error) {
	close(j.called)
	<-ctx.Done()
	return Answer{}, ctx.Err()
}

func TestMaintenanceCancelsInFlightEvidenceAndDrains(t *testing.T) {
	o, _, _ := controlledFixture(t)
	called := make(chan struct{})
	o.judge = maintenanceBlockedJudge{called}
	now := time.Now()
	o.control.maintenance = &maintenanceWindow{config: MaintenanceConfig{"00:00", "UTC", 60, "manual"}, start: now.Add(time.Second), end: now.Add(time.Minute)}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- o.Run(ctx, o.statePath) }()
	select {
	case <-called:
	case err := <-done:
		t.Fatal("exited before evidence", err)
	case <-ctx.Done():
		t.Fatal("no evidence call")
	}
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for o.maintenanceState.Load() != "paused_empty" {
		select {
		case err := <-done:
			t.Fatal("exited before drain", err)
		case <-ctx.Done():
			t.Fatal("did not cancel evidence and drain")
		case <-tick.C:
		}
	}
	if o.commits.Load() != 0 {
		t.Fatal("in-flight evidence published across boundary")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
