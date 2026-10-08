package route

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const continuousLease = 2 * time.Minute

// SupervisorContext is canceled on EOF, invalid input or heartbeat expiry.
func SupervisorContext(parent context.Context, input io.Reader, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	frames := make(chan string)
	go func() {
		defer close(frames)
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 64), 256)
		for scanner.Scan() {
			select {
			case frames <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		defer cancel()
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				return
			case frame, ok := <-frames:
				if !ok || frame != "PING" {
					return
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(timeout)
			}
		}
	}()
	return ctx, cancel
}

func (p *publicationControl) now() time.Time {
	if p.clock != nil {
		return p.clock()
	}
	return time.Now()
}

// EnableContinuous requires the separate process supervisor heartbeat. Loss of
// its context must cancel Run; this does not relax identity or lease checks.
func (o *LabObserver) EnableContinuous(allowed bool) error {
	if !allowed || o.control == nil {
		return fmt.Errorf("continuous publication requires explicit supervised ownership")
	}
	o.control.continuous = true
	return nil
}

// Renewal is serialized with writes and pause by the observer worker. It never
// adopts a new generation, revives an expired lease, or extends more than 2 min.
func (o *LabObserver) renew(ctx context.Context) error {
	p := o.control
	if p == nil || !p.continuous {
		return nil
	}
	if err := p.check(); err != nil {
		return err
	}
	if err := o.unchanged(ctx); err != nil {
		return err
	}
	if err := p.checkPaths(ctx); err != nil {
		return err
	}
	if p.lease.Expires.Sub(p.now()) > time.Minute && p.lease.Expires.Sub(p.now()) <= continuousLease {
		return nil
	}
	next := p.lease
	next.Expires = p.now().Add(continuousLease)
	body, err := json.Marshal(next)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(p.path), ".ownership-renew-*")
	if err != nil {
		return fmt.Errorf("lease renewal temporary file failed")
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(body)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	// Recheck after disk I/O, before the only atomic replacement.
	if err == nil {
		err = p.check()
	}
	if err == nil {
		err = o.unchanged(ctx)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = os.Rename(name, p.path)
	}
	if err != nil {
		return fmt.Errorf("lease renewal failed; publication stopped: %w", err)
	}
	p.lease.Expires = next.Expires // token is immutable and concurrently read by HTTP
	return nil
}
