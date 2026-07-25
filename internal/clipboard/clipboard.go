package clipboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// clipTool is one Linux clipboard helper. env names the session var
// (WAYLAND_DISPLAY / DISPLAY) that must be non-empty for the tool to connect;
// "" means no such gate.
type clipTool struct {
	name string
	env  string
	args []string
}

// Linux candidates in preference order. Wayland first so XWayland sessions
// (which set both DISPLAY and WAYLAND_DISPLAY) use the native protocol.
var linuxReadTools = []clipTool{
	{name: "wl-paste", env: "WAYLAND_DISPLAY", args: []string{"-n"}}, // -n: don't append a trailing newline
	{name: "xclip", env: "DISPLAY", args: []string{"-selection", "clipboard", "-o"}},
	{name: "xsel", env: "DISPLAY", args: []string{"-b"}},
}

var linuxWriteTools = []clipTool{
	{name: "wl-copy", env: "WAYLAND_DISPLAY"},
	{name: "xclip", env: "DISPLAY", args: []string{"-selection", "clipboard"}},
	{name: "xsel", env: "DISPLAY", args: []string{"-ib"}},
}

var errNoLinuxClipboardTool = fmt.Errorf("clipboard: no usable tool found (install wl-clipboard for Wayland or xclip/xsel for X11; WAYLAND_DISPLAY or DISPLAY must be set)")

// maxClipboardBytes bounds clipboard reads so a hostile or runaway selection
// owner cannot exhaust memory.
const maxClipboardBytes = 64 << 20

// cmdTimeout bounds clipboard helper subprocesses so a hung helper cannot
// block the CLI forever.
const cmdTimeout = 30 * time.Second

func commandCtx(name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	return exec.CommandContext(ctx, name, args...), cancel
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

func Read() (string, error) {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "pbpaste"
	case "linux":
		tool := findAvailable(linuxReadTools)
		if tool == nil {
			return "", errNoLinuxClipboardTool
		}
		name = tool.name
		args = tool.args
	case "windows":
		name = "powershell"
		args = []string{"-command", "Get-Clipboard"}
	default:
		return "", fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}

	cmd, cancel := commandCtx(name, args...)
	defer cancel()
	out := &limitWriter{max: maxClipboardBytes}
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("reading clipboard via %s: %w", name, err)
	}
	return strings.TrimSpace(out.buf.String()), nil
}

func Write(content string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "pbcopy"
	case "linux":
		tool := findAvailable(linuxWriteTools)
		if tool == nil {
			return errNoLinuxClipboardTool
		}
		name = tool.name
		args = tool.args
		// wl-copy with empty stdin daemonizes to serve the (empty) selection and
		// never returns. --clear releases the selection without spawning a daemon.
		if tool.name == "wl-copy" && content == "" {
			args = []string{"--clear"}
		}
	case "windows":
		name = "clip"
	default:
		return fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}

	cmd, cancel := commandCtx(name, args...)
	defer cancel()
	cmd.Stdin = strings.NewReader(content)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("writing clipboard via %s: %w", name, err)
	}
	return nil
}

// findAvailable returns the first tool whose env gate is satisfied and whose
// executable is on PATH. Returns nil if none qualify.
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

func Clear() error {
	return Write("")
}
