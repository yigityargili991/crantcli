// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

//go:build !linux && !darwin && !windows

package browser

import (
	"fmt"
	"runtime"
)

// Unused on platforms without an opener, but handoff.go is built everywhere.
const maxSafeOpenArgument = 96 << 10

func platformOpenURL(string) (OpenResult, error) {
	return OpenResult{}, fmt.Errorf("opening a browser is not supported on %s", runtime.GOOS)
}
