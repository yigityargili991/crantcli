// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package config

import (
	"path/filepath"
	"syscall"
	"testing"
)

// Opening a FIFO with no writer blocks, so reading one used to hang the
// lookup forever.
func TestTokenFileRejectsFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "token")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	requireTokenFileRejected(t, fifo, "")
}
