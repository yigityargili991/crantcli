package clipboard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"crantcli/internal/procenv"
)

// Backend identifies the mechanism that completed a clipboard operation.
type Backend string

const (
	BackendBuiltInLinux Backend = "built-in Linux clipboard"
	BackendWLPaste      Backend = "wl-paste"
	BackendWLCopy       Backend = "wl-copy"
	BackendXClip        Backend = "xclip"
	BackendXSel         Backend = "xsel"
	BackendPBPaste      Backend = "pbpaste"
	BackendPBCopy       Backend = "pbcopy"
	BackendPowerShell   Backend = "PowerShell"
	BackendWindowsClip  Backend = "clip"
)

// ReadResult is clipboard text together with the backend that supplied it.
type ReadResult struct {
	Text    string
	Backend Backend
}

// WriteResult identifies the backend that accepted a clipboard write.
type WriteResult struct {
	Backend Backend
}

// clipTool is one Linux clipboard helper. env names the session variable
// (WAYLAND_DISPLAY / DISPLAY) that must be non-empty for the tool to connect.
type clipTool struct {
	name    string
	backend Backend
	env     string
	args    []string
}

// External Linux helpers remain compatibility fallbacks for compositors that
// the built-in backend cannot reach. Native Wayland/X11 support is attempted
// first, so a normal desktop does not require any of these packages.
var linuxReadTools = []clipTool{
	{name: "wl-paste", backend: BackendWLPaste, env: "WAYLAND_DISPLAY", args: []string{"-n"}},
	{name: "xclip", backend: BackendXClip, env: "DISPLAY", args: []string{"-selection", "clipboard", "-o"}},
	{name: "xsel", backend: BackendXSel, env: "DISPLAY", args: []string{"-b"}},
}

var linuxWriteTools = []clipTool{
	{name: "wl-copy", backend: BackendWLCopy, env: "WAYLAND_DISPLAY"},
	{name: "xclip", backend: BackendXClip, env: "DISPLAY", args: []string{"-selection", "clipboard"}},
	{name: "xsel", backend: BackendXSel, env: "DISPLAY", args: []string{"-ib"}},
}

var errNoLinuxClipboardTool = errors.New("no external Linux clipboard helper found")

var (
	clipboardRuntimeGOOS = runtime.GOOS
	linuxNativeRead      = readBuiltInLinux
	linuxNativeWrite     = writeBuiltInLinux
)

// maxClipboardBytes bounds clipboard reads and the payload sent to the
// detached Linux selection owner.
const maxClipboardBytes = 64 << 20

// cmdTimeout bounds external clipboard helpers so a hung helper cannot block
// the CLI forever.
const cmdTimeout = 30 * time.Second

func commandCtx(name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = procenv.Sanitized()
	return cmd, cancel
}

// limitWriter caps how much a clipboard helper may write to stdout.
type limitWriter struct {
	buf strings.Builder
	max int
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.max {
		return 0, fmt.Errorf("clipboard content exceeds %d bytes", w.max)
	}
	return w.buf.Write(p)
}

// Read returns trimmed UTF-8 clipboard text. ReadText also reports which
// backend supplied the value.
func Read() (string, error) {
	result, err := ReadText()
	return result.Text, err
}

func ReadText() (ReadResult, error) {
	switch clipboardRuntimeGOOS {
	case "darwin":
		return readCommand("pbpaste", BackendPBPaste, nil)
	case "linux":
		text, nativeErr := linuxNativeRead()
		if nativeErr == nil {
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				return ReadResult{Text: trimmed, Backend: BackendBuiltInLinux}, nil
			}
			// The native backend cannot distinguish an empty clipboard from a
			// selection that offers no text target, and both come back as "".
			// Give an external helper a chance before reporting nothing; a
			// genuinely empty clipboard reads empty there too, so the result is
			// unchanged in that case.
			if tool := findAvailable(linuxReadTools); tool != nil {
				if fallback, err := readCommand(tool.name, tool.backend, tool.args); err == nil && fallback.Text != "" {
					return fallback, nil
				}
			}
			return ReadResult{Text: trimmed, Backend: BackendBuiltInLinux}, nil
		}

		tool := findAvailable(linuxReadTools)
		if tool == nil {
			return ReadResult{}, errors.Join(
				fmt.Errorf("built-in Linux clipboard: %w", nativeErr),
				errNoLinuxClipboardTool,
			)
		}
		result, err := readCommand(tool.name, tool.backend, tool.args)
		if err != nil {
			return ReadResult{}, errors.Join(fmt.Errorf("built-in Linux clipboard: %w", nativeErr), err)
		}
		return result, nil
	case "windows":
		return readCommand("powershell", BackendPowerShell, []string{"-NoProfile", "-NonInteractive", "-Command", "Get-Clipboard"})
	default:
		return ReadResult{}, fmt.Errorf("clipboard not supported on %s", clipboardRuntimeGOOS)
	}
}

func readCommand(name string, backend Backend, args []string) (ReadResult, error) {
	cmd, cancel := commandCtx(name, args...)
	defer cancel()
	out := &limitWriter{max: maxClipboardBytes}
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		return ReadResult{}, fmt.Errorf("reading clipboard via %s: %w", name, err)
	}
	return ReadResult{Text: strings.TrimSpace(out.buf.String()), Backend: backend}, nil
}

// Write copies UTF-8 text. WriteText also reports which backend accepted the
// value. On Linux, the built-in backend starts a detached copy of crantcli to
// retain selection ownership after the foreground command exits.
func Write(content string) error {
	_, err := WriteText(content)
	return err
}

func WriteText(content string) (WriteResult, error) {
	if len(content) > maxClipboardBytes {
		return WriteResult{}, fmt.Errorf("clipboard content exceeds %d bytes", maxClipboardBytes)
	}

	switch clipboardRuntimeGOOS {
	case "darwin":
		return writeCommand("pbcopy", BackendPBCopy, nil, content)
	case "linux":
		nativeErr := linuxNativeWrite(content)
		if nativeErr == nil {
			return WriteResult{Backend: BackendBuiltInLinux}, nil
		}

		tool := findAvailable(linuxWriteTools)
		if tool == nil {
			return WriteResult{}, errors.Join(
				fmt.Errorf("built-in Linux clipboard: %w", nativeErr),
				errNoLinuxClipboardTool,
			)
		}
		args := tool.args
		// wl-copy with empty stdin daemonizes to serve an empty selection and
		// never returns. --clear releases it without spawning a daemon.
		if tool.name == "wl-copy" && content == "" {
			args = []string{"--clear"}
		}
		result, err := writeCommand(tool.name, tool.backend, args, content)
		if err != nil {
			return WriteResult{}, errors.Join(fmt.Errorf("built-in Linux clipboard: %w", nativeErr), err)
		}
		return result, nil
	case "windows":
		return writeCommand("clip", BackendWindowsClip, nil, content)
	default:
		return WriteResult{}, fmt.Errorf("clipboard not supported on %s", clipboardRuntimeGOOS)
	}
}

func writeCommand(name string, backend Backend, args []string, content string) (WriteResult, error) {
	cmd, cancel := commandCtx(name, args...)
	defer cancel()
	cmd.Stdin = strings.NewReader(content)
	if err := cmd.Run(); err != nil {
		return WriteResult{}, fmt.Errorf("writing clipboard via %s: %w", name, err)
	}
	return WriteResult{Backend: backend}, nil
}

// findAvailable returns the first tool whose environment gate is satisfied
// and whose executable is on PATH.
func findAvailable(tools []clipTool) *clipTool {
	for i := range tools {
		if tools[i].env != "" && os.Getenv(tools[i].env) == "" {
			continue
		}
		if _, err := exec.LookPath(tools[i].name); err == nil {
			return &tools[i]
		}
	}
	return nil
}
