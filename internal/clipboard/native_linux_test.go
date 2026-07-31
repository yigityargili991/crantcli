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

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

type failingWriteCloser struct {
	bytes.Buffer
	writeErr error
	closeErr error
}

func (w *failingWriteCloser) Write(p []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return w.Buffer.Write(p)
}

func (w *failingWriteCloser) Close() error { return w.closeErr }

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
	previousTimeout := nativeTimeout
	previousRuntimeGOOS := clipboardRuntimeGOOS
	previousLinuxRead := linuxNativeRead
	previousLinuxWrite := linuxNativeWrite
	t.Cleanup(func() {
		nativeClipboardInit = previousInit
		nativeClipboardRead = previousRead
		nativeClipboardWrite = previousWrite
		nativeExecutable = previousExecutable
		nativeTimeout = previousTimeout
		clipboardRuntimeGOOS = previousRuntimeGOOS
		linuxNativeRead = previousLinuxRead
		linuxNativeWrite = previousLinuxWrite
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

func TestReadBuiltInLinuxRejectsInvalidProtocol(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "helper error", body: "printf '" + nativeErrorPrefix + "desktop unavailable\\n'", want: "desktop unavailable"},
		{name: "missing header", body: "exit 0", want: "before sending data"},
		{name: "unexpected header", body: "printf 'unexpected\\n'", want: "unexpected clipboard reader response"},
		{name: "invalid length", body: "printf '" + nativeDataPrefix + "nope\\n'", want: "invalid clipboard reader length"},
		{name: "truncated data", body: "printf '" + nativeDataPrefix + "5\\nhi'", want: "reading native clipboard data"},
		{name: "failed after data", body: "printf '" + nativeDataPrefix + "2\\nhi'\nexit 7", want: "clipboard reader failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			isolateNativeClipboard(t)
			script := filepath.Join(t.TempDir(), "reader")
			if err := os.WriteFile(script, []byte("#!/bin/sh\n"+test.body+"\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			nativeExecutable = func() (string, error) { return script, nil }
			_, err := readBuiltInLinux()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want text %q", err, test.want)
			}
		})
	}
}

func TestReadBuiltInLinuxProcessFailures(t *testing.T) {
	t.Run("start failure", func(t *testing.T) {
		isolateNativeClipboard(t)
		nativeExecutable = func() (string, error) {
			return filepath.Join(t.TempDir(), "missing"), nil
		}
		if _, err := readBuiltInLinux(); err == nil || !strings.Contains(err.Error(), "starting clipboard reader") {
			t.Fatalf("error = %v", err)
		}
	})

	for _, test := range []struct {
		name string
		body string
	}{
		{name: "header timeout", body: "exec sleep 1"},
		{name: "data timeout", body: "printf '" + nativeDataPrefix + "5\\n'; exec sleep 1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			isolateNativeClipboard(t)
			nativeTimeout = 10 * time.Millisecond
			script := filepath.Join(t.TempDir(), "reader")
			if err := os.WriteFile(script, []byte("#!/bin/sh\n"+test.body+"\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			nativeExecutable = func() (string, error) { return script, nil }
			if _, err := readBuiltInLinux(); err == nil || !strings.Contains(err.Error(), "timed out") {
				t.Fatalf("error = %v", err)
			}
		})
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

func TestWriteBuiltInLinuxRejectsInvalidHandshake(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "owner error", body: "printf '" + nativeErrorPrefix + "selection denied\\n'", want: "selection denied"},
		{name: "missing response", body: "exit 0", want: "exited before becoming ready"},
		{name: "unexpected response", body: "printf 'unexpected\\n'", want: "unexpected clipboard owner response"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			isolateNativeClipboard(t)
			script := filepath.Join(t.TempDir(), "owner")
			if err := os.WriteFile(script, []byte("#!/bin/sh\n"+test.body+"\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			nativeExecutable = func() (string, error) { return script, nil }
			err := writeBuiltInLinux("state URL")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want text %q", err, test.want)
			}
		})
	}
}

func TestWriteBuiltInLinuxProcessFailures(t *testing.T) {
	t.Run("start failure", func(t *testing.T) {
		isolateNativeClipboard(t)
		nativeExecutable = func() (string, error) {
			return filepath.Join(t.TempDir(), "missing"), nil
		}
		if err := writeBuiltInLinux("state URL"); err == nil || !strings.Contains(err.Error(), "starting clipboard owner") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("handshake timeout", func(t *testing.T) {
		isolateNativeClipboard(t)
		nativeTimeout = 10 * time.Millisecond
		script := filepath.Join(t.TempDir(), "owner")
		if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 1\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		nativeExecutable = func() (string, error) { return script, nil }
		if err := writeBuiltInLinux("state URL"); err == nil || !strings.Contains(err.Error(), "did not become ready") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestNativeClipboardDirectFailures(t *testing.T) {
	t.Run("reader panic", func(t *testing.T) {
		isolateNativeClipboard(t)
		nativeClipboardInit = func() error { panic("display panic") }
		if _, err := readNativeClipboardDirect(); err == nil || !strings.Contains(err.Error(), "display panic") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("reader initialization", func(t *testing.T) {
		isolateNativeClipboard(t)
		t.Setenv("WAYLAND_DISPLAY", "")
		t.Setenv("DISPLAY", "")
		nativeClipboardInit = func() error { return errors.New("unavailable") }
		if _, err := readNativeClipboardDirect(); err == nil || !strings.Contains(err.Error(), "no graphical") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("reader size limit", func(t *testing.T) {
		isolateNativeClipboard(t)
		nativeClipboardInit = func() error { return nil }
		nativeClipboardRead = func(xclipboard.Format) []byte { return make([]byte, maxClipboardBytes+1) }
		if _, err := readNativeClipboardDirect(); err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("reader protocol error", func(t *testing.T) {
		isolateNativeClipboard(t)
		nativeClipboardInit = func() error { return errors.New("unavailable") }
		var output bytes.Buffer
		RunNativeClipboardReader(nopWriteCloser{&output})
		if !strings.Contains(output.String(), nativeErrorPrefix) {
			t.Fatalf("output = %q", output.String())
		}
	})
}

func TestNativeClipboardOwnerFailures(t *testing.T) {
	tests := []struct {
		name  string
		input io.Reader
		setup func(*testing.T)
		ready *failingWriteCloser
		want  string
	}{
		{
			name:  "input error",
			input: failingReader{err: errors.New("read failed")},
			want:  "reading clipboard owner input",
		},
		{
			name:  "initialization error",
			input: strings.NewReader("value"),
			setup: func(t *testing.T) {
				t.Setenv("DISPLAY", ":test")
				nativeClipboardInit = func() error { return errors.New("display failed") }
			},
			want: "display failed",
		},
		{
			name:  "write rejected",
			input: strings.NewReader("value"),
			setup: func(*testing.T) {
				nativeClipboardInit = func() error { return nil }
				nativeClipboardWrite = func(xclipboard.Format, []byte) <-chan struct{} { return nil }
			},
			want: "rejected",
		},
		{
			name:  "ownership lost",
			input: strings.NewReader("value"),
			setup: func(*testing.T) {
				nativeClipboardInit = func() error { return nil }
				nativeClipboardWrite = func(xclipboard.Format, []byte) <-chan struct{} {
					changed := make(chan struct{})
					close(changed)
					return changed
				}
			},
			want: "lost before startup",
		},
		{
			name:  "ready write fails",
			input: strings.NewReader("value"),
			setup: func(*testing.T) {
				nativeClipboardInit = func() error { return nil }
				nativeClipboardWrite = func(xclipboard.Format, []byte) <-chan struct{} { return make(chan struct{}) }
			},
			ready: &failingWriteCloser{writeErr: errors.New("broken handshake")},
			want:  "acknowledging clipboard ownership",
		},
		{
			name:  "ready close fails",
			input: strings.NewReader("value"),
			setup: func(*testing.T) {
				nativeClipboardInit = func() error { return nil }
				nativeClipboardWrite = func(xclipboard.Format, []byte) <-chan struct{} { return make(chan struct{}) }
			},
			ready: &failingWriteCloser{closeErr: errors.New("close failed")},
			want:  "closing clipboard owner handshake",
		},
		{
			name:  "native panic",
			input: strings.NewReader("value"),
			setup: func(*testing.T) {
				nativeClipboardInit = func() error { panic("owner panic") }
			},
			want: "owner panic",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			isolateNativeClipboard(t)
			nativeClipboardInit = func() error { return nil }
			if test.setup != nil {
				test.setup(t)
			}
			ready := test.ready
			if ready == nil {
				ready = &failingWriteCloser{}
			}
			err := runNativeClipboardOwner(test.input, ready)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want text %q", err, test.want)
			}
		})
	}

	t.Run("size limit", func(t *testing.T) {
		isolateNativeClipboard(t)
		nativeClipboardInit = func() error { return nil }
		input := io.LimitReader(strings.NewReader(strings.Repeat("x", maxClipboardBytes+1)), maxClipboardBytes+1)
		err := runNativeClipboardOwner(input, &failingWriteCloser{})
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestNativeInitializationErrorSanitizesMessage(t *testing.T) {
	t.Setenv("DISPLAY", ":test")
	message := strings.Repeat("x", 350) + "\nsecret"
	err := nativeInitializationError(errors.New(message))
	if strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "…") {
		t.Fatalf("error was not sanitized: %q", err)
	}
}

func TestLinuxClipboardHighLevelBranches(t *testing.T) {
	t.Run("read wrapper returns native text", func(t *testing.T) {
		isolateNativeClipboard(t)
		linuxNativeRead = func() (string, error) { return "  value  ", nil }
		got, err := Read()
		if err != nil || got != "value" {
			t.Fatalf("Read = (%q, %v)", got, err)
		}
	})

	t.Run("native read failure without helper", func(t *testing.T) {
		isolateNativeClipboard(t)
		linuxNativeRead = func() (string, error) { return "", errors.New("native failed") }
		t.Setenv("PATH", t.TempDir())
		_, err := ReadText()
		if err == nil || !strings.Contains(err.Error(), "native failed") || !strings.Contains(err.Error(), "no external") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("fallback read failure", func(t *testing.T) {
		isolateNativeClipboard(t)
		linuxNativeRead = func() (string, error) { return "", errors.New("native failed") }
		dir := t.TempDir()
		script := filepath.Join(dir, "wl-paste")
		if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 3\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		t.Setenv("WAYLAND_DISPLAY", "test")
		_, err := ReadText()
		if err == nil || !strings.Contains(err.Error(), "native failed") || !strings.Contains(err.Error(), "wl-paste") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("write wrapper uses native backend", func(t *testing.T) {
		isolateNativeClipboard(t)
		linuxNativeWrite = func(string) error { return nil }
		if err := Write("value"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("native write failure without helper", func(t *testing.T) {
		isolateNativeClipboard(t)
		linuxNativeWrite = func(string) error { return errors.New("native failed") }
		t.Setenv("PATH", t.TempDir())
		_, err := WriteText("value")
		if err == nil || !strings.Contains(err.Error(), "native failed") || !strings.Contains(err.Error(), "no external") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("fallback write failure", func(t *testing.T) {
		isolateNativeClipboard(t)
		linuxNativeWrite = func(string) error { return errors.New("native failed") }
		dir := t.TempDir()
		script := filepath.Join(dir, "wl-copy")
		if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 4\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		t.Setenv("WAYLAND_DISPLAY", "test")
		_, err := WriteText("value")
		if err == nil || !strings.Contains(err.Error(), "native failed") || !strings.Contains(err.Error(), "wl-copy") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("empty write clears wl-copy", func(t *testing.T) {
		isolateNativeClipboard(t)
		linuxNativeWrite = func(string) error { return errors.New("native failed") }
		dir := t.TempDir()
		argsPath := filepath.Join(dir, "args")
		script := filepath.Join(dir, "wl-copy")
		if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$*\" >\"$CRANTCLI_TEST_ARGS\"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		t.Setenv("WAYLAND_DISPLAY", "test")
		t.Setenv("CRANTCLI_TEST_ARGS", argsPath)
		result, err := WriteText("")
		if err != nil {
			t.Fatal(err)
		}
		if result.Backend != BackendWLCopy {
			t.Fatalf("backend = %q", result.Backend)
		}
		if args, err := os.ReadFile(argsPath); err != nil || string(args) != "--clear" {
			t.Fatalf("args = %q, err = %v", args, err)
		}
	})

	t.Run("oversized write", func(t *testing.T) {
		isolateNativeClipboard(t)
		_, err := WriteText(strings.Repeat("x", maxClipboardBytes+1))
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestClipboardPlatformCommandBranches(t *testing.T) {
	t.Run("darwin", func(t *testing.T) {
		isolateNativeClipboard(t)
		clipboardRuntimeGOOS = "darwin"
		dir := t.TempDir()
		copyPath := filepath.Join(dir, "copied")
		if err := os.WriteFile(filepath.Join(dir, "pbpaste"), []byte("#!/bin/sh\nprintf 'darwin value\\n'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "pbcopy"), []byte("#!/bin/sh\n/bin/cat >\"$CRANTCLI_TEST_COPY\"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		t.Setenv("CRANTCLI_TEST_COPY", copyPath)

		read, err := ReadText()
		if err != nil || read.Text != "darwin value" || read.Backend != BackendPBPaste {
			t.Fatalf("ReadText = (%#v, %v)", read, err)
		}
		write, err := WriteText("darwin copy")
		if err != nil || write.Backend != BackendPBCopy {
			t.Fatalf("WriteText = (%#v, %v)", write, err)
		}
		if copied, err := os.ReadFile(copyPath); err != nil || string(copied) != "darwin copy" {
			t.Fatalf("copied = %q, err = %v", copied, err)
		}
	})

	t.Run("windows", func(t *testing.T) {
		isolateNativeClipboard(t)
		clipboardRuntimeGOOS = "windows"
		dir := t.TempDir()
		copyPath := filepath.Join(dir, "copied")
		if err := os.WriteFile(filepath.Join(dir, "powershell"), []byte("#!/bin/sh\nprintf 'windows value\\n'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "clip"), []byte("#!/bin/sh\n/bin/cat >\"$CRANTCLI_TEST_COPY\"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		t.Setenv("CRANTCLI_TEST_COPY", copyPath)

		read, err := ReadText()
		if err != nil || read.Text != "windows value" || read.Backend != BackendPowerShell {
			t.Fatalf("ReadText = (%#v, %v)", read, err)
		}
		write, err := WriteText("windows copy")
		if err != nil || write.Backend != BackendWindowsClip {
			t.Fatalf("WriteText = (%#v, %v)", write, err)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		isolateNativeClipboard(t)
		clipboardRuntimeGOOS = "plan9"
		if _, err := ReadText(); err == nil || !strings.Contains(err.Error(), "plan9") {
			t.Fatalf("ReadText error = %v", err)
		}
		if _, err := WriteText("value"); err == nil || !strings.Contains(err.Error(), "plan9") {
			t.Fatalf("WriteText error = %v", err)
		}
	})
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
