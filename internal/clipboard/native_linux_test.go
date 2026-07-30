//go:build linux

package clipboard

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	xclipboard "golang.design/x/clipboard"
)

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

type signalWriteCloser struct {
	bytes.Buffer
	closed chan struct{}
}

func newSignalWriteCloser() *signalWriteCloser {
	return &signalWriteCloser{closed: make(chan struct{})}
}

func (w *signalWriteCloser) Close() error {
	close(w.closed)
	return nil
}

func isolateNativeClipboard(t *testing.T) {
	t.Helper()
	previousInit := nativeClipboardInit
	previousRead := nativeClipboardRead
	previousWrite := nativeClipboardWrite
	previousExecutable := nativeExecutable
	t.Cleanup(func() {
		nativeClipboardInit = previousInit
		nativeClipboardRead = previousRead
		nativeClipboardWrite = previousWrite
		nativeExecutable = previousExecutable
	})
}

func TestRunNativeClipboardOwnerAcknowledgesThenServes(t *testing.T) {
	isolateNativeClipboard(t)
	nativeClipboardInit = func() error { return nil }
	changed := make(chan struct{})
	var received []byte
	nativeClipboardWrite = func(_ xclipboard.Format, data []byte) <-chan struct{} {
		received = append([]byte(nil), data...)
		return changed
	}

	ready := newSignalWriteCloser()
	done := make(chan struct{})
	go func() {
		runNativeClipboardOwner(strings.NewReader("state URL"), ready)
		close(done)
	}()

	select {
	case <-ready.closed:
	case <-time.After(time.Second):
		t.Fatal("owner did not acknowledge readiness")
	}
	if got := string(received); got != "state URL" {
		t.Fatalf("clipboard payload = %q, want state URL", got)
	}
	if !strings.Contains(ready.String(), nativeOwnerReadyPrefix) {
		t.Fatalf("owner did not acknowledge readiness: %q", ready.String())
	}
	select {
	case <-done:
		t.Fatal("owner exited before the selection was replaced")
	default:
	}
	close(changed)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("owner did not exit after the selection was replaced")
	}
}

func TestRunNativeClipboardOwnerReportsInitializationError(t *testing.T) {
	isolateNativeClipboard(t)
	t.Setenv("DISPLAY", ":test")
	nativeClipboardInit = func() error { return errors.New("display unavailable") }
	var ready bytes.Buffer
	RunNativeClipboardOwner(strings.NewReader("value"), nopWriteCloser{&ready})
	if got := ready.String(); !strings.Contains(got, nativeErrorPrefix) || !strings.Contains(got, "display unavailable") {
		t.Fatalf("owner response = %q", got)
	}
}

func TestLinuxExternalHelperFallback(t *testing.T) {
	t.Run("write", func(t *testing.T) {
		isolateNativeClipboard(t)
		nativeExecutable = func() (string, error) { return "", errors.New("self-exec unavailable") }
		dir := t.TempDir()
		payloadPath := filepath.Join(dir, "copied")
		script := filepath.Join(dir, "wl-copy")
		if err := os.WriteFile(script, []byte("#!/bin/sh\n/bin/cat >\"$CRANTCLI_TEST_COPY\"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		t.Setenv("WAYLAND_DISPLAY", "test")
		t.Setenv("CRANTCLI_TEST_COPY", payloadPath)

		result, err := WriteText("fallback payload")
		if err != nil {
			t.Fatalf("WriteText: %v", err)
		}
		if result.Backend != BackendWLCopy {
			t.Fatalf("backend = %q, want %q", result.Backend, BackendWLCopy)
		}
		data, err := os.ReadFile(payloadPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "fallback payload" {
			t.Fatalf("payload = %q", data)
		}
	})

	t.Run("read", func(t *testing.T) {
		isolateNativeClipboard(t)
		nativeExecutable = func() (string, error) { return "", errors.New("self-exec unavailable") }
		dir := t.TempDir()
		script := filepath.Join(dir, "wl-paste")
		if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'fallback value\\n'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		t.Setenv("WAYLAND_DISPLAY", "test")

		result, err := ReadText()
		if err != nil {
			t.Fatalf("ReadText: %v", err)
		}
		if result.Backend != BackendWLPaste || result.Text != "fallback value" {
			t.Fatalf("result = %#v", result)
		}
	})
}

func TestRunNativeClipboardReaderFramesBoundedData(t *testing.T) {
	isolateNativeClipboard(t)
	nativeClipboardInit = func() error { return nil }
	nativeClipboardRead = func(xclipboard.Format) []byte { return []byte("clipboard data\nwith newline") }
	var output bytes.Buffer
	RunNativeClipboardReader(nopWriteCloser{&output})
	want := nativeDataPrefix + "27\nclipboard data\nwith newline"
	if output.String() != want {
		t.Fatalf("reader output = %q, want %q", output.String(), want)
	}
}

func TestReadBuiltInLinuxSelfExecProtocol(t *testing.T) {
	isolateNativeClipboard(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "reader")
	body := "#!/bin/sh\nprintf '" + nativeDataPrefix + "5\\nhello'\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	nativeExecutable = func() (string, error) { return script, nil }
	got, err := readBuiltInLinux()
	if err != nil {
		t.Fatalf("readBuiltInLinux: %v", err)
	}
	if got != "hello" {
		t.Fatalf("readBuiltInLinux = %q, want hello", got)
	}
}

func TestWriteBuiltInLinuxSelfExecHandshake(t *testing.T) {
	isolateNativeClipboard(t)
	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "payload")
	script := filepath.Join(dir, "owner")
	body := "#!/bin/sh\ncat >\"$CRANTCLI_TEST_PAYLOAD\"\nprintf '" + nativeOwnerReadyPrefix + "\\n'\nsleep 1\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CRANTCLI_TEST_PAYLOAD", payloadPath)
	nativeExecutable = func() (string, error) { return script, nil }

	if err := writeBuiltInLinux("secret state URL"); err != nil {
		t.Fatalf("writeBuiltInLinux: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	var data []byte
	for time.Now().Before(deadline) {
		data, _ = os.ReadFile(payloadPath)
		if string(data) == "secret state URL" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if string(data) != "secret state URL" {
		t.Fatalf("detached owner payload = %q", data)
	}
}

func TestWriteBuiltInLinuxRefusesEmptySelection(t *testing.T) {
	isolateNativeClipboard(t)
	nativeExecutable = func() (string, error) {
		t.Fatal("empty content must not spawn a detached owner")
		return "", nil
	}
	if err := writeBuiltInLinux(""); err == nil {
		t.Fatal("writeBuiltInLinux accepted an empty selection")
	}
}

func TestEmptyNativeReadFallsBackToHelper(t *testing.T) {
	isolateNativeClipboard(t)
	dir := t.TempDir()
	// The native reader connects but reports no text, as it does for a
	// selection that offers no text target.
	reader := filepath.Join(dir, "reader")
	if err := os.WriteFile(reader, []byte("#!/bin/sh\nprintf '"+nativeDataPrefix+"0\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	nativeExecutable = func() (string, error) { return reader, nil }

	helperDir := t.TempDir()
	helper := filepath.Join(helperDir, "wl-paste")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nprintf 'text target only'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", helperDir)
	t.Setenv("WAYLAND_DISPLAY", "test")

	result, err := ReadText()
	if err != nil {
		t.Fatalf("ReadText: %v", err)
	}
	if result.Text != "text target only" || result.Backend != BackendWLPaste {
		t.Fatalf("result = %#v, want the helper value", result)
	}
}

func TestEmptyClipboardStaysEmptyWithoutHelper(t *testing.T) {
	isolateNativeClipboard(t)
	dir := t.TempDir()
	reader := filepath.Join(dir, "reader")
	if err := os.WriteFile(reader, []byte("#!/bin/sh\nprintf '"+nativeDataPrefix+"0\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	nativeExecutable = func() (string, error) { return reader, nil }
	t.Setenv("PATH", t.TempDir())

	result, err := ReadText()
	if err != nil {
		t.Fatalf("ReadText: %v", err)
	}
	if result.Text != "" || result.Backend != BackendBuiltInLinux {
		t.Fatalf("result = %#v, want an empty built-in read", result)
	}
}
