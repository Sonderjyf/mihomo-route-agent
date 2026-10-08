package route

import (
	"context"
	"fmt"
	"time"
)

// MaintenanceConfig has no implicit start time or end policy. Fixed-offset
// zones avoid ambiguous/skipped daylight-saving times on Windows.
type MaintenanceConfig struct {
	Start           string `json:"start"`
	Timezone        string `json:"timezone"`
	DurationSeconds int    `json:"duration_seconds"`
	Resume          string `json:"resume"`
}

func (m MaintenanceConfig) validate() error {
	t, err := time.Parse("15:04", m.Start)
	if err != nil || t.Format("15:04") != m.Start || m.DurationSeconds < 1 || m.DurationSeconds >= 86400 {
		return fmt.Errorf("maintenance requires start HH:MM and duration_seconds 1..86399")
	}
	if m.Timezone != "Asia/Shanghai" && m.Timezone != "UTC" {
		return fmt.Errorf("maintenance timezone must be explicit Asia/Shanghai (UTC+08:00) or UTC")
	}
	if m.Resume != "manual" && m.Resume != "unchanged" {
		return fmt.Errorf("maintenance resume must be manual or unchanged")
	}
	return nil
}

type maintenanceWindow struct {
	config     MaintenanceConfig
	start, end time.Time
	paused     bool // worker-owned
}

func newMaintenance(m MaintenanceConfig, now time.Time) (*maintenanceWindow, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	zone := time.UTC
	if m.Timezone == "Asia/Shanghai" {
		zone = time.FixedZone("Asia/Shanghai", 8*3600)
	}
	clock, _ := time.Parse("15:04", m.Start)
	local := now.In(zone)
	start := time.Date(local.Year(), local.Month(), local.Day(), clock.Hour(), clock.Minute(), 0, 0, zone)
	duration := time.Duration(m.DurationSeconds) * time.Second
	if now.Before(start) && now.Before(start.AddDate(0, 0, -1).Add(duration)) {
		start = start.AddDate(0, 0, -1)
	}
	if !now.Before(start.Add(duration)) {
		start = start.AddDate(0, 0, 1)
	}
	return &maintenanceWindow{config: m, start: start, end: start.Add(duration)}, nil
}

func (m *maintenanceWindow) due(now time.Time) bool { return m != nil && !now.Before(m.start) }

// The write worker calls this after its previous bounded operation completes.
// A clock boundary is not acknowledgement: clearing must finish first.
func (o *LabObserver) maintain(ctx context.Context, now time.Time) error {
	m := o.control.maintenance
	if m == nil || !m.due(now) {
		return nil
	}
	if !m.paused {
		if o.control.paused {
			return nil
		} // authenticated manual pause wins
		o.maintenanceState.Store("draining")
		if err := o.pauseControlled(ctx); err != nil {
			return err
		}
		m.paused = true
		o.maintenanceState.Store("paused_empty")
	}
	if now.Before(m.end) || m.config.Resume == "manual" {
		return nil
	}
	if err := o.control.check(); err != nil {
		return err
	}
	if err := o.unchanged(ctx); err != nil {
		return err
	}
	if err := o.control.checkPaths(ctx); err != nil {
		return err
	}
	if err := o.reconcile(ctx); err != nil {
		return err
	}
	if err := o.control.check(); err != nil {
		return err
	}
	if err := o.saveJournal("active"); err != nil {
		return err
	}
	next, err := newMaintenance(m.config, now)
	if err != nil {
		return err
	}
	o.control.maintenance = next
	o.control.paused = false
	o.started = now // do not learn connections created during the pause
	o.ready.Store(true)
	o.maintenanceState.Store("scheduled")
	return nil
}
