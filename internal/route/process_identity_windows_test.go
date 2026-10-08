package route

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

func TestNativePublisherAbsencePermissionAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		absent bool
	}{
		{"missing", windows.ERROR_INVALID_PARAMETER, true},
		{"denied", windows.ERROR_ACCESS_DENIED, false},
		{"other_error", windows.ERROR_INVALID_HANDLE, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			absent, err := publisherExitedWith(context.Background(), 42, func(int) (identityProcess, error) { return nil, nativeFailure("process_open", tc.err) })
			if absent != tc.absent || (err == nil) != tc.absent {
				t.Fatal(absent, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	absent, err := publisherExitedWith(ctx, 42, func(int) (identityProcess, error) {
		cancel()
		return nil, nativeFailure("process_open", windows.ERROR_INVALID_PARAMETER)
	})
	if absent || !errors.Is(err, context.Canceled) {
		t.Fatal("absence accepted after cancellation", absent, err)
	}
	for _, alive := range []bool{true, false} {
		p := &fakeIdentityProcess{alive: alive}
		absent, err := publisherExitedWith(context.Background(), 42, func(int) (identityProcess, error) { return p, nil })
		if err != nil || absent == alive || p.closed != 1 {
			t.Fatal(absent, err, p.closed)
		}
	}
}

func TestNativeProcessWaitStateMapping(t *testing.T) {
	for _, tc := range []struct {
		status       uint32
		alive, valid bool
	}{
		{uint32(windows.WAIT_TIMEOUT), true, true}, {windows.WAIT_OBJECT_0, false, true},
		{windows.WAIT_ABANDONED, false, false}, {windows.WAIT_FAILED, false, false}, {123, false, false},
	} {
		alive, err := nativeProcessAlive(tc.status, nil)
		if alive != tc.alive || (err == nil) != tc.valid {
			t.Fatal(tc.status, alive, err)
		}
	}
	if alive, err := nativeProcessAlive(uint32(windows.WAIT_TIMEOUT), windows.ERROR_ACCESS_DENIED); alive || err == nil {
		t.Fatal("wait error accepted")
	}
}
