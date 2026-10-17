package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"
)

type publisherLock struct {
	Version int `json:"version"`
	PID     int `json:"pid"`
}

func windowsPublisherAbsent(ctx context.Context, pid int) error {
	exited, err := windowsPublisherExited(ctx, pid)
	if err != nil {
		return err
	}
	if !exited {
		return fmt.Errorf("publisher still present; recovery blocked")
	}
	return nil
}

func windowsPublisherExited(ctx context.Context, pid int) (bool, error) {
	if runtime.GOOS != "windows" || pid < 1 {
		return false, fmt.Errorf("publisher exit verification unavailable")
	}
	exited, err := platformPublisherExited(ctx, pid)
	if err != nil {
		return false, fmt.Errorf("publisher exit unverified; recovery blocked: %w", err)
	}
	return exited, nil
}

// WatchControlledRecovery is an explicitly launched sidecar; it never launches
// or terminates a process. False means clean lock removal, not a recovery pass.
func WatchControlledRecovery(ctx context.Context, c Config, allowed bool, ownershipPath, statePath, secret string) (bool, error) {
	if !allowed || ownershipPath == "" || statePath == "" || secret == "" {
		return false, fmt.Errorf("owned recovery requires explicit permission, ownership, state-file and controller authentication")
	}
	if _, err := readOwnership(c, ownershipPath); err != nil {
		return false, err
	}
	return watchPublisher(ctx, ownershipPath+".lock", windowsPublisherExited, func(ctx context.Context) error {
		return RecoverControlled(ctx, c, allowed, ownershipPath, statePath, secret)
	})
}

func watchPublisher(ctx context.Context, path string, exited func(context.Context, int) (bool, error), recover func(context.Context) error) (bool, error) {
	body, err := readPrivate(path, 1024)
	var record publisherLock
	if err != nil || json.Unmarshal(body, &record) != nil || record.Version != 1 || record.PID < 1 {
		return false, fmt.Errorf("missing or legacy publisher lock")
	}
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		current, err := readPrivate(path, 1024)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil || !bytes.Equal(body, current) {
			return false, fmt.Errorf("publisher lock changed while watching")
		}
		gone, err := exited(ctx, record.PID)
		if err != nil {
			return false, err
		}
		if gone {
			return true, recover(ctx)
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-tick.C:
		}
	}
}

// RecoverControlled clears only the two owned providers and never observes,
// probes, judges, reloads a profile or restarts a publisher. Expiry does not
// prohibit empty cleanup; a changed config, core, lock or rule sequence does.
func RecoverControlled(ctx context.Context, c Config, allowed bool, ownershipPath, statePath, secret string) error {
	return recoverControlled(ctx, c, allowed, ownershipPath, statePath, secret, windowsDirectPath{}, windowsPublisherAbsent)
}

func recoverControlled(ctx context.Context, c Config, allowed bool, ownershipPath, statePath, secret string, checker directPathChecker, absent func(context.Context, int) error) error {
	if !allowed || ownershipPath == "" || statePath == "" || secret == "" || checker == nil || absent == nil {
		return fmt.Errorf("owned recovery requires explicit permission, ownership, state-file and controller authentication")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	lease, err := readOwnership(c, ownershipPath)
	if err != nil {
		return err
	}
	lockPath := ownershipPath + ".lock"
	lockBody, err := readPrivate(lockPath, 1024)
	var record publisherLock
	if err != nil || json.Unmarshal(lockBody, &record) != nil || record.Version != 1 || record.PID < 1 {
		return fmt.Errorf("missing or legacy publisher lock; process exit cannot be established")
	}
	if err := absent(ctx, record.PID); err != nil {
		return err
	}
	recoveryPath := ownershipPath + ".recovery.lock"
	lock, err := os.OpenFile(recoveryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("another or interrupted recovery owns this lease")
	}
	defer os.Remove(recoveryPath)
	defer lock.Close()
	verifyLock := func() error {
		current, err := readPrivate(lockPath, 1024)
		if err != nil || !bytes.Equal(current, lockBody) {
			return fmt.Errorf("publisher lock changed; recovery blocked")
		}
		return absent(ctx, record.PID)
	}
	if err := verifyLock(); err != nil {
		return err
	}
	o, err := newObserver(c, Stub{})
	if err != nil {
		return err
	}
	o.control = &publicationControl{path: ownershipPath, lease: lease, checker: checker}
	o.statePath = statePath
	o.Providers.secret = secret
	o.Providers.strictFetch = true
	if err := o.control.checkIdentity(); err != nil {
		return err
	}
	if err := checker.CheckOwner(ctx, lease); err != nil {
		return err
	}
	o.baseline, err = o.rules(ctx)
	if err != nil {
		return err
	}
	if rulesDigest(o.baseline) != lease.RulesSHA256 {
		return fmt.Errorf("recovery core rules do not match ownership")
	}
	if err := o.checkJournal(); err != nil {
		return err
	}
	o.Providers.beforeWrite = func(job context.Context, entries map[string]Entry) error {
		if len(entries) != 0 {
			return fmt.Errorf("recovery forbids nonempty publication")
		}
		if err := verifyLock(); err != nil {
			return err
		}
		return o.unchanged(job)
	}
	listener, err := net.Listen("tcp", c.HTTPListen)
	if err != nil {
		return fmt.Errorf("owned provider endpoint unavailable; recovery blocked: %w", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(o.Providers.Handler), ReadHeaderTimeout: time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 16384}
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(listener) }()
	defer func() { _ = server.Close(); <-done }()
	if err := o.reconcile(ctx); err != nil {
		return fmt.Errorf("owned empty recovery incomplete: %w", err)
	}
	if err := o.saveJournal("recovered_stopped"); err != nil {
		return err
	}
	if err := verifyLock(); err != nil {
		return err
	}
	// Keep the publisher lock as a consumed lease tombstone. A later start must
	// capture new ownership, even if the old publication lease has not expired.
	return nil
}
