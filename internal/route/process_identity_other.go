//go:build !windows

package route

import (
	"context"
	"fmt"
)

func platformCoreIdentity(context.Context, int) (coreIdentity, error) {
	return coreIdentity{}, fmt.Errorf("native_windows_identity_unavailable")
}

func platformPublisherExited(context.Context, int) (bool, error) {
	return false, fmt.Errorf("native_windows_identity_unavailable")
}
