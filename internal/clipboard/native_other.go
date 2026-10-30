// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package clipboard

import (
	"fmt"
	"runtime"
)

func readBuiltInLinux() (string, error) {
	return "", fmt.Errorf("built-in Linux clipboard is unavailable on %s", runtime.GOOS)
}

func writeBuiltInLinux(string) error {
	return fmt.Errorf("built-in Linux clipboard is unavailable on %s", runtime.GOOS)
}
