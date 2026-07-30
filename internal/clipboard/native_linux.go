//go:build linux

package clipboard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"crantcli/internal/procenv"

	xclipboard "golang.design/x/clipboard"
)

// NativeOwnerCommandName and NativeReaderCommandName are private self-exec
// modes. The owner keeps a selection alive; the reader isolates an untrusted
// clipboard provider and enforces the foreground process's timeout and limit.
const (
	NativeOwnerCommandName  = "__clipboard-serve"
	NativeReaderCommandName = "__clipboard-read"
)

const (
	nativeOwnerReadyPrefix = "CRANTCLI_CLIPBOARD_READY"
	nativeDataPrefix       = "CRANTCLI_CLIPBOARD_DATA "
	nativeErrorPrefix      = "CRANTCLI_CLIPBOARD_ERROR "
	nativeOperationTimeout = 30 * time.Second
)

var (
	nativeClipboardInit  = xclipboard.Init
	nativeClipboardRead  = xclipboard.Read
	nativeClipboardWrite = xclipboard.Write
	nativeExecutable     = os.Executable
)

// openDevNull discards a helper's stderr without the pipe and copier goroutine
// that a non-file io.Writer would require. A nil result leaves stderr inherited,
// which is noisier but never wrong.
func openDevNull() *os.File {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return nil
	}
	return devNull
}

func readBuiltInLinux() (string, error) {
	executable, err := nativeExecutable()
	if err != nil {
		return "", fmt.Errorf("locating crantcli executable: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), nativeOperationTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, NativeReaderCommandName)
	cmd.Env = procenv.Sanitized()
	if devNull := openDevNull(); devNull != nil {
		defer devNull.Close()
		cmd.Stderr = devNull
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("creating clipboard reader output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("starting clipboard reader: %w", err)
	}

	reader := bufio.NewReader(stdout)
	header, readErr := reader.ReadString('\n')
	header = strings.TrimSpace(header)
	if strings.HasPrefix(header, nativeErrorPrefix) {
		_ = cmd.Wait()
		return "", errors.New(strings.TrimPrefix(header, nativeErrorPrefix))
	}
	if readErr != nil {
		_ = cmd.Wait()
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("clipboard read timed out after %s", nativeOperationTimeout)
		}
		return "", fmt.Errorf("clipboard reader exited before sending data: %w", readErr)
	}
	if !strings.HasPrefix(header, nativeDataPrefix) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", fmt.Errorf("unexpected clipboard reader response %q", header)
	}

	lengthText := strings.TrimPrefix(header, nativeDataPrefix)
	length, err := strconv.ParseInt(lengthText, 10, 64)
	if err != nil || length < 0 || length > maxClipboardBytes {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", fmt.Errorf("invalid clipboard reader length %q", lengthText)
	}
	data := make([]byte, int(length))
	if _, err := io.ReadFull(reader, data); err != nil {
		_ = cmd.Wait()
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("clipboard read timed out after %s", nativeOperationTimeout)
		}
		return "", fmt.Errorf("reading native clipboard data: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("clipboard read timed out after %s", nativeOperationTimeout)
		}
		return "", fmt.Errorf("clipboard reader failed: %w", err)
	}
	return string(data), nil
}

func writeBuiltInLinux(content string) error {
	if content == "" {
		// Serving a zero-length selection would leave the detached owner alive
		// forever with nothing to hand out. Defer to the external helpers,
		// where wl-copy has an explicit --clear mode for this.
		return errors.New("built-in Linux clipboard cannot serve an empty selection")
	}

	executable, err := nativeExecutable()
	if err != nil {
		return fmt.Errorf("locating crantcli executable: %w", err)
	}

	cmd := exec.Command(executable, NativeOwnerCommandName)
	cmd.Env = procenv.Sanitized()
	// An *os.File is handed to the child directly. A plain io.Writer would make
	// os/exec allocate a pipe and a copier goroutine whose parent-side end is
	// closed only by Wait, which the success path deliberately never calls.
	if devNull := openDevNull(); devNull != nil {
		defer devNull.Close()
		cmd.Stderr = devNull
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("creating clipboard owner input: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("creating clipboard owner handshake: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return fmt.Errorf("starting clipboard owner: %w", err)
	}

	writeDone := make(chan error, 1)
	go func() {
		_, copyErr := io.WriteString(stdin, content)
		closeErr := stdin.Close()
		if copyErr != nil {
			writeDone <- copyErr
			return
		}
		writeDone <- closeErr
	}()

	response := make(chan struct {
		line string
		err  error
	}, 1)
	go func() {
		line, readErr := bufio.NewReader(stdout).ReadString('\n')
		response <- struct {
			line string
			err  error
		}{strings.TrimSpace(line), readErr}
	}()

	fail := func(cause error) error {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return cause
	}

	timer := time.NewTimer(nativeOperationTimeout)
	defer timer.Stop()
	select {
	case result := <-response:
		if result.line == nativeOwnerReadyPrefix {
			if writeErr := <-writeDone; writeErr != nil {
				return fail(fmt.Errorf("sending clipboard contents to owner: %w", writeErr))
			}
			_ = stdout.Close()
			if err := cmd.Process.Release(); err != nil {
				return fmt.Errorf("detaching clipboard owner: %w", err)
			}
			return nil
		}
		if strings.HasPrefix(result.line, nativeErrorPrefix) {
			return fail(errors.New(strings.TrimPrefix(result.line, nativeErrorPrefix)))
		}
		if result.err != nil {
			return fail(fmt.Errorf("clipboard owner exited before becoming ready: %w", result.err))
		}
		return fail(fmt.Errorf("unexpected clipboard owner response %q", result.line))
	case <-timer.C:
		return fail(fmt.Errorf("clipboard owner did not become ready within %s", nativeOperationTimeout))
	}
}

// RunNativeClipboardReader reads one selection in an isolated process and
// writes a length-prefixed response for the foreground parent.
func RunNativeClipboardReader(output io.WriteCloser) {
	data, err := readNativeClipboardDirect()
	if err != nil {
		writeNativeProtocolError(output, err)
		return
	}
	if _, err := fmt.Fprintf(output, "%s%d\n", nativeDataPrefix, len(data)); err == nil {
		_, _ = output.Write(data)
	}
	_ = output.Close()
}

func readNativeClipboardDirect() (data []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("native clipboard panic: %v", recovered)
		}
	}()
	if err := nativeClipboardInit(); err != nil {
		return nil, nativeInitializationError(err)
	}
	data = nativeClipboardRead(xclipboard.FmtText)
	if len(data) > maxClipboardBytes {
		return nil, fmt.Errorf("clipboard content exceeds %d bytes", maxClipboardBytes)
	}
	return data, nil
}

// RunNativeClipboardOwner reads one bounded payload, claims the desktop text
// selection, acknowledges readiness, and then remains alive until another
// application replaces the selection or the desktop session closes.
func RunNativeClipboardOwner(input io.Reader, ready io.WriteCloser) {
	if err := runNativeClipboardOwner(input, ready); err != nil {
		writeNativeProtocolError(ready, err)
	}
}

func runNativeClipboardOwner(input io.Reader, ready io.WriteCloser) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("native clipboard panic: %v", recovered)
		}
	}()

	data, err := io.ReadAll(io.LimitReader(input, maxClipboardBytes+1))
	if err != nil {
		return fmt.Errorf("reading clipboard owner input: %w", err)
	}
	if len(data) > maxClipboardBytes {
		return fmt.Errorf("clipboard content exceeds %d bytes", maxClipboardBytes)
	}
	if err := nativeClipboardInit(); err != nil {
		return nativeInitializationError(err)
	}
	changed := nativeClipboardWrite(xclipboard.FmtText, data)
	if changed == nil {
		return fmt.Errorf("native clipboard rejected the write")
	}
	select {
	case <-changed:
		return fmt.Errorf("clipboard ownership was lost before startup completed")
	default:
	}

	if _, err := fmt.Fprintln(ready, nativeOwnerReadyPrefix); err != nil {
		return fmt.Errorf("acknowledging clipboard ownership: %w", err)
	}
	if err := ready.Close(); err != nil {
		return fmt.Errorf("closing clipboard owner handshake: %w", err)
	}

	<-changed
	return nil
}

func writeNativeProtocolError(output io.WriteCloser, err error) {
	message := strings.NewReplacer("\r", " ", "\n", " ").Replace(err.Error())
	_, _ = fmt.Fprintln(output, nativeErrorPrefix+message)
	_ = output.Close()
}

func nativeInitializationError(err error) error {
	if os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("DISPLAY") == "" {
		return fmt.Errorf("no graphical Wayland/X11 session (WAYLAND_DISPLAY and DISPLAY are unset)")
	}
	message := strings.TrimSpace(err.Error())
	if newline := strings.IndexByte(message, '\n'); newline >= 0 {
		message = message[:newline]
	}
	if len(message) > 300 {
		message = message[:300] + "…"
	}
	return fmt.Errorf("could not connect to the Wayland/X11 clipboard: %s", message)
}
