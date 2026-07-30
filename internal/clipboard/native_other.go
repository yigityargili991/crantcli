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
