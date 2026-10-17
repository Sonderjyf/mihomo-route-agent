package route

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var getExtendedTCPTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")

type windowsIdentitySource struct{}
type windowsIdentityProcess struct{ handle windows.Handle }

type nativeWindowsError struct {
	stage string
	code  syscall.Errno
}

func (e *nativeWindowsError) Error() string {
	return fmt.Sprintf("native_%s_failed (win32_code=%d)", e.stage, uint32(e.code))
}
func (e *nativeWindowsError) Unwrap() error { return e.code }

func nativeFailure(stage string, err error) error {
	var code syscall.Errno
	_ = errors.As(err, &code)
	return &nativeWindowsError{stage, code}
}

func (windowsIdentitySource) ListenerPID(ctx context.Context, port int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := getExtendedTCPTable.Find(); err != nil {
		return 0, nativeFailure("tcp_api", err)
	}
	const maxTableBytes = 8 << 20
	size := uint32(64 << 10)
	for attempt := 0; attempt < 4; attempt++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if size < 4 || size > maxTableBytes {
			return 0, fmt.Errorf("native_tcp_table_size_invalid")
		}
		buffer := make([]byte, int(size))
		// AF_INET=2; TCP_TABLE_OWNER_PID_LISTENER=3; reserved=0.
		status, _, _ := getExtendedTCPTable.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)), 0, 2, 3, 0)
		runtime.KeepAlive(buffer)
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if syscall.Errno(status) == windows.ERROR_INSUFFICIENT_BUFFER {
			continue
		}
		if status != 0 {
			return 0, nativeFailure("tcp_table", syscall.Errno(status))
		}
		if size < 4 || int(size) > len(buffer) {
			return 0, fmt.Errorf("native_tcp_table_size_invalid")
		}
		return loopbackListenerPID(buffer[:size], port)
	}
	return 0, fmt.Errorf("native_tcp_table_unstable")
}

func (windowsIdentitySource) Open(pid int) (identityProcess, error) {
	if pid < 1 || uint64(pid) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("native_pid_invalid")
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return nil, nativeFailure("process_open", err)
	}
	return windowsIdentityProcess{handle}, nil
}

func (p windowsIdentityProcess) StartedFiletime() (uint64, error) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(p.handle, &created, &exited, &kernel, &user); err != nil {
		return 0, nativeFailure("process_times", err)
	}
	return uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), nil
}

func (p windowsIdentityProcess) Alive() (bool, error) {
	status, err := windows.WaitForSingleObject(p.handle, 0)
	return nativeProcessAlive(status, err)
}

func nativeProcessAlive(status uint32, err error) (bool, error) {
	if err != nil {
		return false, nativeFailure("process_wait", err)
	}
	switch status {
	case uint32(windows.WAIT_TIMEOUT):
		return true, nil
	case windows.WAIT_OBJECT_0:
		return false, nil
	default:
		return false, fmt.Errorf("native_process_wait_invalid")
	}
}

func (p windowsIdentityProcess) Close() error { return windows.CloseHandle(p.handle) }

func platformCoreIdentity(ctx context.Context, port int) (coreIdentity, error) {
	return boundedNativeQuery(ctx, nativeIdentitySlots, func(ctx context.Context) (coreIdentity, error) {
		return collectCoreIdentity(ctx, port, windowsIdentitySource{})
	})
}

func platformPublisherExited(ctx context.Context, pid int) (bool, error) {
	if pid < 1 || uint64(pid) > uint64(^uint32(0)) {
		return false, fmt.Errorf("native_pid_invalid")
	}
	return boundedNativeQuery(ctx, nativeIdentitySlots, func(ctx context.Context) (bool, error) {
		return publisherExitedWith(ctx, pid, (windowsIdentitySource{}).Open)
	})
}

func publisherExitedWith(ctx context.Context, pid int, open func(int) (identityProcess, error)) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	process, err := open(pid)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		// Only a nonexistent nonzero PID is absence. Access denied is unknown.
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return true, nil
		}
		return false, nativeFailure("publisher_open", err)
	}
	defer process.Close()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	alive, err := process.Alive()
	if err != nil {
		return false, err
	}
	return !alive, ctx.Err()
}
