package route

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"
)

func tcpIdentityTable(rows ...[6]uint32) []byte {
	b := make([]byte, 4+24*len(rows))
	binary.LittleEndian.PutUint32(b, uint32(len(rows)))
	for i, row := range rows {
		for j, v := range row {
			binary.LittleEndian.PutUint32(b[4+24*i+4*j:], v)
		}
	}
	return b
}

func TestLoopbackListenerPIDTable(t *testing.T) {
	// Port 0x1234 is stored as bytes 12 34 00 00, not a native uint16.
	good := [6]uint32{2, 0x0100007f, 0x3412, 0, 0, 42}
	other := good
	other[5] = 43
	wildcard := good
	wildcard[1] = 0
	connected := good
	connected[0] = 5
	zeroPID := good
	zeroPID[5] = 0
	wrongPort := good
	wrongPort[2] = 0x1234
	truncated := tcpIdentityTable(good)
	binary.LittleEndian.PutUint32(truncated, math.MaxUint32)
	for _, tc := range []struct {
		name  string
		table []byte
		want  int
	}{
		{"one", tcpIdentityTable(good), 42}, {"duplicate_same_pid", tcpIdentityTable(good, good), 42},
		{"ambiguous", tcpIdentityTable(good, other), 0}, {"wildcard", tcpIdentityTable(wildcard), 0},
		{"established", tcpIdentityTable(connected), 0}, {"zero_pid", tcpIdentityTable(zeroPID), 0},
		{"wrong_endian", tcpIdentityTable(wrongPort), 0}, {"truncated", truncated, 0},
		{"partial_header", []byte{1, 0}, 0}, {"empty", tcpIdentityTable(), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := loopbackListenerPID(tc.table, 0x1234)
			if got != tc.want || (err == nil) != (tc.want != 0) {
				t.Fatalf("got %d, %v", got, err)
			}
		})
	}
}

func TestNativeFiletimePreservesOwnershipTicks(t *testing.T) {
	// Unix epoch FILETIME -> .NET DateTime.UnixEpoch.Ticks.
	got, err := filetimeUTCTicks(116444736000000000)
	if err != nil || got != "621355968000000000" {
		t.Fatal(got, err)
	}
	for _, v := range []uint64{0, math.MaxUint64, 3155378975999999999 - 504911232000000000 + 1} {
		if _, err := filetimeUTCTicks(v); err == nil {
			t.Fatal("invalid FILETIME accepted")
		}
	}
}

type fakeIdentityProcess struct {
	started            uint64
	alive              bool
	startErr, aliveErr error
	closed             int
	onAlive            func()
}

func (p *fakeIdentityProcess) StartedFiletime() (uint64, error) { return p.started, p.startErr }
func (p *fakeIdentityProcess) Alive() (bool, error) {
	if p.onAlive != nil {
		p.onAlive()
	}
	return p.alive, p.aliveErr
}
func (p *fakeIdentityProcess) Close() error { p.closed++; return nil }

type fakeIdentitySource struct {
	p                  *fakeIdentityProcess
	pids               []int
	calls, opens       int
	openErr, lookupErr error
}

func (s *fakeIdentitySource) ListenerPID(context.Context, int) (int, error) {
	s.calls++
	if s.lookupErr != nil {
		return 0, s.lookupErr
	}
	return s.pids[s.calls-1], nil
}
func (s *fakeIdentitySource) Open(int) (identityProcess, error) { s.opens++; return s.p, s.openErr }

func TestNativeIdentityFailsClosed(t *testing.T) {
	denied := errors.New("access_denied")
	for _, kind := range []string{"success", "table_error", "permission", "times_error", "changed_pid", "exited", "wait_error", "canceled", "cancel_during"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := &fakeIdentityProcess{started: 116444736000000000, alive: true}
			s := &fakeIdentitySource{p: p, pids: []int{42, 42}}
			switch kind {
			case "table_error":
				s.lookupErr = denied
			case "permission":
				s.openErr = denied
			case "times_error":
				p.startErr = denied
			case "changed_pid":
				s.pids[1] = 43
			case "exited":
				p.alive = false
			case "wait_error":
				p.aliveErr = denied
			case "canceled":
				cancel()
			case "cancel_during":
				p.onAlive = cancel
			}
			identity, err := collectCoreIdentity(ctx, 443, s)
			if (err == nil) != (kind == "success") {
				t.Fatalf("unexpected result %+v, %v", identity, err)
			}
			if kind == "success" && (identity.PID != 42 || identity.Started != "621355968000000000") {
				t.Fatal(identity)
			}
			if kind == "canceled" && (s.calls != 0 || s.opens != 0) {
				t.Fatal("called backend after cancellation")
			}
			wantClosed := 0
			if s.opens > 0 && s.openErr == nil {
				wantClosed = 1
			}
			if p.closed != wantClosed {
				t.Fatalf("handle closed %d times", p.closed)
			}
		})
	}
}

func TestBoundedNativeQueryCancellationAndWorkerLimit(t *testing.T) {
	slots := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	entered, release, returned := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := boundedNativeQuery(ctx, slots, func(context.Context) (int, error) { close(entered); <-release; return 99, nil })
		returned <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker not started")
	}
	cancel()
	select {
	case err := <-returned:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("caller did not cancel")
	}
	second, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	_, err := boundedNativeQuery(second, slots, func(context.Context) (int, error) {
		t.Error("second worker started while first syscall blocked")
		return 0, nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	close(release)
	third, stopThird := context.WithTimeout(context.Background(), time.Second)
	defer stopThird()
	value, err := boundedNativeQuery(third, slots, func(context.Context) (int, error) { return 7, nil })
	if err != nil || value != 7 {
		t.Fatal(value, err)
	}
}
