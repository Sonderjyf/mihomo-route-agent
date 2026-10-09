package route

import (
	"context"
	"fmt"
	"time"
)

// OwnedWorker is one exclusively owned publisher generation. Done must deliver
// once and close only after process exit (including provider HTTP shutdown).
// Drain must verify empty providers before acknowledging. Stop is nonblocking.
type OwnedWorker interface {
	Done() <-chan error
	Drain(context.Context) error
	Stop()
}

// ConfigurationOwner is deliberately not implemented with shell commands or UI
// guesses. The application owner must supply these operations before automatic
// refresh is enabled. Refresh is called only with the old publisher stopped.
type ConfigurationOwner interface {
	Refresh(context.Context) error
	StartFresh(context.Context) (OwnedWorker, error)
	// RecoverEmpty may clear only the stopped worker's unchanged owned core
	// generation. It must fail on changed ownership and must never start learning.
	RecoverEmpty(context.Context) error
}

// SuperviseOwned coordinates requests serially. A request is consumed only
// after the previous refresh completes. An unexpected exit runs bounded cleanup
// once and returns; it never silently restarts a crashed publisher.
// The caller retains exclusive configuration ownership for the whole call.
func SuperviseOwned(ctx context.Context, worker OwnedWorker, owner ConfigurationOwner, refresh <-chan struct{}) error {
	if worker == nil || owner == nil {
		return fmt.Errorf("supervisor requires an owned worker and configuration owner")
	}
	for {
		select {
		case err := <-worker.Done():
			return recoverStopped(owner, fmt.Errorf("owned publisher exited unexpectedly: %v", err))
		case <-ctx.Done():
			return stopOwned(worker, owner)
		case _, open := <-refresh:
			if !open {
				refresh = nil
				continue
			}
			// Prioritize a detected crash even when a refresh is also queued.
			select {
			case err := <-worker.Done():
				return recoverStopped(owner, fmt.Errorf("owned publisher exited before refresh: %v", err))
			default:
			}
			if err := stopOwned(worker, owner); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := owner.Refresh(ctx); err != nil {
				return fmt.Errorf("profile refresh failed; publisher remains stopped: %w", err)
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			var err error
			worker, err = owner.StartFresh(ctx)
			if err != nil || worker == nil {
				// StartFresh must clean up any partial start on failure.
				return fmt.Errorf("fresh ownership/start failed: %v", err)
			}
		}
	}
}

func recoverStopped(owner ConfigurationOwner, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := owner.RecoverEmpty(ctx); err != nil {
		return fmt.Errorf("%v; owned recovery required: %w", cause, err)
	}
	return cause
}

func stopOwned(worker OwnedWorker, owner ConfigurationOwner) error {
	return stopOwnedWithin(worker, owner, 25*time.Second)
}

func stopOwnedWithin(worker OwnedWorker, owner ConfigurationOwner, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	drainErr := worker.Drain(ctx)
	worker.Stop()
	select {
	case exitErr := <-worker.Done():
		if drainErr != nil || exitErr != nil {
			return recoverStopped(owner, fmt.Errorf("publisher drain/exit failed: %v / %v", drainErr, exitErr))
		}
		return nil
	case <-ctx.Done():
		// No confirmed exit: do not race it with cleanup or a profile refresh.
		return fmt.Errorf("publisher exit unconfirmed; refresh and recovery blocked")
	}
}
