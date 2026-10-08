package route

import (
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"time"
)

const nativeIdentityBudget = 3 * time.Second

// Native Windows calls cannot be interrupted. One shared slot bounds outstanding
// workers even if a syscall stalls; cancellation discards its eventual result.
var nativeIdentitySlots = make(chan struct{}, 1)

func boundedNativeQuery[T any](parent context.Context, slots chan struct{}, work func(context.Context) (T, error)) (T, error) {
	var zero T
	ctx, cancel := context.WithTimeout(parent, nativeIdentityBudget)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return zero, ctx.Err()
	}
	type result struct {
		value T
		err   error
	}
	done := make(chan result, 1)
	go func() {
		defer func() { <-slots }()
		if err := ctx.Err(); err != nil {
			done <- result{err: err}
			return
		}
		value, err := work(ctx)
		done <- result{value, err}
	}()
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case r := <-done:
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		return r.value, r.err
	}
}

type identityProcess interface {
	StartedFiletime() (uint64, error)
	Alive() (bool, error)
	Close() error
}

type identitySource interface {
	ListenerPID(context.Context, int) (int, error)
	Open(int) (identityProcess, error)
}

// Keep the opened process object alive across the second listener snapshot and
// final liveness check. A PID-only lookup never substitutes for this identity.
func collectCoreIdentity(ctx context.Context, port int, source identitySource) (coreIdentity, error) {
	var zero coreIdentity
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	pid, err := source.ListenerPID(ctx, port)
	if err != nil {
		return zero, err
	}
	if err = ctx.Err(); err != nil {
		return zero, err
	}
	process, err := source.Open(pid)
	if err != nil {
		return zero, err
	}
	defer process.Close()
	if err = ctx.Err(); err != nil {
		return zero, err
	}
	filetime, err := process.StartedFiletime()
	if err != nil {
		return zero, err
	}
	started, err := filetimeUTCTicks(filetime)
	if err != nil {
		return zero, err
	}
	if err = ctx.Err(); err != nil {
		return zero, err
	}
	again, err := source.ListenerPID(ctx, port)
	if err != nil {
		return zero, err
	}
	if again != pid {
		return zero, fmt.Errorf("listener_owner_changed")
	}
	if err = ctx.Err(); err != nil {
		return zero, err
	}
	alive, err := process.Alive()
	if err != nil {
		return zero, err
	}
	if !alive {
		return zero, fmt.Errorf("listener_owner_exited")
	}
	if err = ctx.Err(); err != nil {
		return zero, err
	}
	return coreIdentity{PID: pid, Started: started}, nil
}

// Existing ownership files use DateTime UTC ticks (year 1), not FILETIME (1601).
func filetimeUTCTicks(filetime uint64) (string, error) {
	const epochOffset uint64 = 504911232000000000
	const maxDateTimeTicks uint64 = 3155378975999999999
	if filetime == 0 || filetime > maxDateTimeTicks-epochOffset {
		return "", fmt.Errorf("process_start_time_invalid")
	}
	return strconv.FormatUint(filetime+epochOffset, 10), nil
}

// IPv4 MIB_TCPTABLE_OWNER_PID: DWORD count followed by six-DWORD rows.
// All fields are DWORDs (4-byte alignment) on supported Windows architectures.
// Addresses and ports are network-order bytes; count/state/PID are native DWORDs.
func loopbackListenerPID(table []byte, port int) (int, error) {
	if port < 1 || port > 65535 || len(table) < 4 {
		return 0, fmt.Errorf("tcp_table_invalid")
	}
	count := uint64(binary.LittleEndian.Uint32(table[:4]))
	if count > uint64((len(table)-4)/24) {
		return 0, fmt.Errorf("tcp_table_truncated")
	}
	pid := uint32(0)
	for i := uint64(0); i < count; i++ {
		row := table[4+i*24 : 4+(i+1)*24]
		if binary.LittleEndian.Uint32(row[:4]) != 2 || row[4] != 127 || row[5] != 0 || row[6] != 0 || row[7] != 1 || int(binary.BigEndian.Uint16(row[8:10])) != port {
			continue
		}
		owner := binary.LittleEndian.Uint32(row[20:24])
		if owner == 0 || uint64(owner) > uint64(^uint(0)>>1) || (pid != 0 && pid != owner) {
			return 0, fmt.Errorf("listener_owner_ambiguous")
		}
		pid = owner
	}
	if pid == 0 {
		return 0, fmt.Errorf("loopback_listener_missing")
	}
	return int(pid), nil
}
