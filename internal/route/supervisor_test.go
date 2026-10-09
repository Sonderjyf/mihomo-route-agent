package route

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeOwnedWorker struct {
	events   *[]string
	done     chan error
	drainErr error
}

type stuckOwnedWorker struct{ done chan error }

func (w stuckOwnedWorker) Done() <-chan error          { return w.done }
func (w stuckOwnedWorker) Drain(context.Context) error { return nil }
func (w stuckOwnedWorker) Stop()                       {}

func TestOwnedSupervisorUnconfirmedExitNeverRecovers(t *testing.T) {
	var events []string
	err := stopOwnedWithin(stuckOwnedWorker{make(chan error)}, fakeConfigurationOwner{events: &events}, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "exit unconfirmed") {
		t.Fatalf("got %v", err)
	}
	if len(events) != 0 {
		t.Fatal(events)
	}
}

func (w *fakeOwnedWorker) Done() <-chan error { return w.done }
func (w *fakeOwnedWorker) Drain(context.Context) error {
	*w.events = append(*w.events, "drain")
	return w.drainErr
}
func (w *fakeOwnedWorker) Stop() {
	*w.events = append(*w.events, "stop-exit")
	w.done <- nil
	close(w.done)
}

type fakeConfigurationOwner struct {
	events                  *[]string
	cancel                  context.CancelFunc
	refreshErr, recoveryErr error
}

func (o fakeConfigurationOwner) Refresh(context.Context) error {
	*o.events = append(*o.events, "refresh")
	return o.refreshErr
}
func (o fakeConfigurationOwner) StartFresh(context.Context) (OwnedWorker, error) {
	*o.events = append(*o.events, "new-ownership-start")
	o.cancel()
	return &fakeOwnedWorker{events: o.events, done: make(chan error, 1)}, nil
}
func (o fakeConfigurationOwner) RecoverEmpty(context.Context) error {
	*o.events = append(*o.events, "recover-empty")
	return o.recoveryErr
}

func TestOwnedSupervisorRefreshOrder(t *testing.T) {
	var events []string
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker := &fakeOwnedWorker{events: &events, done: make(chan error, 1)}
	requests := make(chan struct{}, 1)
	requests <- struct{}{}
	err := SuperviseOwned(ctx, worker, fakeConfigurationOwner{events: &events, cancel: cancel}, requests)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"drain", "stop-exit", "refresh", "new-ownership-start", "drain", "stop-exit"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events %v, want %v", events, want)
	}
}

func TestOwnedSupervisorCrashNeverRefreshesOrRestarts(t *testing.T) {
	var events []string
	worker := &fakeOwnedWorker{events: &events, done: make(chan error, 1)}
	worker.done <- fmt.Errorf("synthetic crash")
	close(worker.done)
	requests := make(chan struct{}, 1)
	requests <- struct{}{}
	err := SuperviseOwned(context.Background(), worker, fakeConfigurationOwner{events: &events, recoveryErr: fmt.Errorf("core replaced")}, requests)
	if err == nil || !strings.Contains(err.Error(), "core replaced") {
		t.Fatalf("got %v", err)
	}
	if !reflect.DeepEqual(events, []string{"recover-empty"}) {
		t.Fatal(events)
	}
}

func TestOwnedSupervisorDrainFailureBlocksRefresh(t *testing.T) {
	var events []string
	worker := &fakeOwnedWorker{events: &events, done: make(chan error, 1), drainErr: fmt.Errorf("provider unavailable")}
	requests := make(chan struct{}, 1)
	requests <- struct{}{}
	err := SuperviseOwned(context.Background(), worker, fakeConfigurationOwner{events: &events}, requests)
	if err == nil || !strings.Contains(err.Error(), "provider unavailable") {
		t.Fatalf("got %v", err)
	}
	if !reflect.DeepEqual(events, []string{"drain", "stop-exit", "recover-empty"}) {
		t.Fatal(events)
	}
}

func TestOwnedSupervisorRefreshFailureLeavesStopped(t *testing.T) {
	var events []string
	worker := &fakeOwnedWorker{events: &events, done: make(chan error, 1)}
	requests := make(chan struct{}, 1)
	requests <- struct{}{}
	err := SuperviseOwned(context.Background(), worker, fakeConfigurationOwner{events: &events, refreshErr: fmt.Errorf("UI unavailable")}, requests)
	if err == nil || !strings.Contains(err.Error(), "UI unavailable") {
		t.Fatalf("got %v", err)
	}
	if !reflect.DeepEqual(events, []string{"drain", "stop-exit", "refresh"}) {
		t.Fatal(events)
	}
}
