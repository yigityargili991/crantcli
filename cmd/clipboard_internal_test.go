//go:build linux

package cmd

import (
	"io"
	"testing"
)

func TestClipboardInternalCommandsDispatch(t *testing.T) {
	previousOwner := runClipboardOwner
	previousReader := runClipboardReader
	t.Cleanup(func() {
		runClipboardOwner = previousOwner
		runClipboardReader = previousReader
	})

	ownerCalled := false
	runClipboardOwner = func(io.Reader, io.WriteCloser) {
		ownerCalled = true
	}
	readerCalled := false
	runClipboardReader = func(io.WriteCloser) {
		readerCalled = true
	}

	clipboardOwnerCmd.Run(clipboardOwnerCmd, nil)
	clipboardReaderCmd.Run(clipboardReaderCmd, nil)
	if !ownerCalled || !readerCalled {
		t.Fatalf("owner called = %t, reader called = %t", ownerCalled, readerCalled)
	}
}
