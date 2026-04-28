package clipboard

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
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

func Read() (string, error) {
	var cmd *exec.Cmd
	var toolName string
	switch runtime.GOOS {
	case "darwin":
		toolName = "pbpaste"
		cmd = exec.Command(toolName)
	case "linux":
		tool := findAvailable(linuxReadTools)
		if tool == nil {
			return "", errNoLinuxClipboardTool
		}
		toolName = tool.name
		cmd = exec.Command(tool.name, tool.args...)
	case "windows":
		toolName = "powershell"
		cmd = exec.Command(toolName, "-command", "Get-Clipboard")
	default:
		return "", fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("reading clipboard via %s: %w", toolName, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func Write(content string) error {
	var cmd *exec.Cmd
	var toolName string
	switch runtime.GOOS {
	case "darwin":
		toolName = "pbcopy"
		cmd = exec.Command(toolName)
	case "linux":
		tool := findAvailable(linuxWriteTools)
		if tool == nil {
			return errNoLinuxClipboardTool
		}
		toolName = tool.name
		args := tool.args
		// wl-copy with empty stdin daemonizes to serve the (empty) selection and
		// never returns. --clear releases the selection without spawning a daemon.
		if tool.name == "wl-copy" && content == "" {
			args = []string{"--clear"}
		}
		cmd = exec.Command(tool.name, args...)
	case "windows":
		toolName = "clip"
		cmd = exec.Command(toolName)
	default:
		return fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}

	cmd.Stdin = strings.NewReader(content)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("writing clipboard via %s: %w", toolName, err)
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
