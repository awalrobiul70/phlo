//go:build !windows

package vcruntime

import (
	"context"

	"phlo/internal/errs"
)

// Install is only needed on Windows.
func Install(context.Context, string) (string, error) {
	return "", errs.New("the Visual C++ runtime is only needed on Windows")
}
