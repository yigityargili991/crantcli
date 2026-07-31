//go:build linux

package browser

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func isolateLinuxOpeners(t *testing.T) {
	t.Helper()
	previousPortal := openViaPortal
	previousPrepare := prepareCommandOpenURL
	previousCommand := runPlatformCommand
	previousConnect := connectPortalSession
	previousBus := connectPortalBus
	previousToken := generateHandoffToken
	previousTimeout := portalRequestTimeout
	t.Cleanup(func() {
		openViaPortal = previousPortal
		prepareCommandOpenURL = previousPrepare
		runPlatformCommand = previousCommand
		connectPortalSession = previousConnect
		connectPortalBus = previousBus
		generateHandoffToken = previousToken
		portalRequestTimeout = previousTimeout
	})
}

func TestPlatformOpenPrefersPortal(t *testing.T) {
	isolateLinuxOpeners(t)
	commands := 0
	openViaPortal = func(string) (OpenResult, error) {
		return OpenResult{Backend: BackendXDGPortal}, nil
	}
	runPlatformCommand = func(Backend, string, ...string) (OpenResult, error) {
		commands++
		return OpenResult{}, errors.New("unexpected command")
	}

	result, err := platformOpenURL("https://example.org/")
	if err != nil {
		t.Fatal(err)
	}
	if result.Backend != BackendXDGPortal || commands != 0 {
		t.Fatalf("result=%#v commands=%d", result, commands)
	}
}

func TestPlatformOpenFallsBackInOrder(t *testing.T) {
	isolateLinuxOpeners(t)
	openViaPortal = func(string) (OpenResult, error) {
		return OpenResult{}, errors.New("portal unavailable")
	}
	var calls []string
	runPlatformCommand = func(backend Backend, name string, args ...string) (OpenResult, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if name == "xdg-open" {
			return OpenResult{}, errors.New("xdg failed")
		}
		return OpenResult{Backend: backend}, nil
	}

	result, err := platformOpenURL("https://example.org/state")
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := []string{
		"xdg-open https://example.org/state",
		"gio open https://example.org/state",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", calls, wantCalls)
	}
	if result.Backend != BackendGIO {
		t.Fatalf("backend = %q, want %q", result.Backend, BackendGIO)
	}
}

func TestPlatformOpenReportsEveryFailure(t *testing.T) {
	isolateLinuxOpeners(t)
	openViaPortal = func(string) (OpenResult, error) {
		return OpenResult{}, errors.New("portal failed")
	}
	runPlatformCommand = func(_ Backend, name string, _ ...string) (OpenResult, error) {
		return OpenResult{}, errors.New(name + " failed")
	}

	_, err := platformOpenURL("https://example.org/")
	if err == nil {
		t.Fatal("platformOpenURL unexpectedly succeeded")
	}
	for _, want := range []string{"portal failed", "xdg-open failed", "gio failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestPlatformOpenReportsPreparationFailure(t *testing.T) {
	isolateLinuxOpeners(t)
	openViaPortal = func(string) (OpenResult, error) {
		return OpenResult{}, errors.New("portal failed")
	}
	prepareCommandOpenURL = func(string) (string, error) {
		return "", errors.New("cache unavailable")
	}

	_, err := platformOpenURL("https://example.org/")
	if err == nil {
		t.Fatal("platformOpenURL unexpectedly succeeded")
	}
	for _, want := range []string{"portal failed", "preparing browser handoff", "cache unavailable"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestPlatformOpenStopsAfterTerminalPortalErrors(t *testing.T) {
	for _, portalErr := range []error{errPortalCancelled, errPortalFallbackUnsafe} {
		t.Run(portalErr.Error(), func(t *testing.T) {
			isolateLinuxOpeners(t)
			openViaPortal = func(string) (OpenResult, error) {
				return OpenResult{}, portalErr
			}
			prepareCommandOpenURL = func(string) (string, error) {
				t.Fatal("command fallback was prepared after terminal portal error")
				return "", nil
			}
			runPlatformCommand = func(Backend, string, ...string) (OpenResult, error) {
				t.Fatal("command fallback ran after terminal portal error")
				return OpenResult{}, nil
			}

			_, err := platformOpenURL("https://example.org/")
			if !errors.Is(err, portalErr) {
				t.Fatalf("error = %v, want %v", err, portalErr)
			}
		})
	}
}

func TestOversizedURLUsesPrivateFileForCommandFallback(t *testing.T) {
	isolateLinuxOpeners(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	openViaPortal = func(string) (OpenResult, error) {
		return OpenResult{}, errors.New("portal unavailable")
	}
	longURL := "https://example.org/#!" + strings.Repeat("a", maxSafeOpenArgument)
	var openedTarget string
	runPlatformCommand = func(backend Backend, _ string, args ...string) (OpenResult, error) {
		openedTarget = args[len(args)-1]
		return OpenResult{Backend: backend}, nil
	}

	if _, err := platformOpenURL(longURL); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(openedTarget)
	if err != nil || parsed.Scheme != "file" {
		t.Fatalf("command target = %q, want file URL", openedTarget)
	}
	data, err := os.ReadFile(parsed.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), strings.Repeat("a", 1024)) || strings.Contains(openedTarget, "#!") {
		t.Fatal("handoff did not keep oversized state out of the command argument")
	}
	if info, err := os.Stat(parsed.Path); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("handoff permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestPlatformOpenSweepsStaleHandoffsOnPortalSuccess(t *testing.T) {
	isolateLinuxOpeners(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	openViaPortal = func(string) (OpenResult, error) {
		return OpenResult{Backend: BackendXDGPortal}, nil
	}

	// A leftover from an earlier portal outage must not linger just because the
	// portal now works and stages nothing.
	dir, err := handoffDir()
	if err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "crantcli_stale.html")
	if err := os.WriteFile(stale, []byte("<!doctype html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-handoffLifetime - time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := platformOpenURL("https://example.org/"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale handoff survived a portal open: %v", err)
	}
}

func TestPortalHandleTokenIsValidAndUnique(t *testing.T) {
	first, err := handoffToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := handoffToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !strings.HasPrefix(first, "crantcli_") || strings.ContainsAny(first, "-./") {
		t.Fatalf("invalid portal tokens %q and %q", first, second)
	}
}

func TestWaitForPortalResponse(t *testing.T) {
	requestPath := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/test")
	tests := []struct {
		name       string
		signals    []*dbus.Signal
		closeAfter bool
		want       Backend
		wantError  string
	}{
		{
			name: "ignores unrelated signals before success",
			signals: []*dbus.Signal{
				nil,
				{Name: "other.signal", Path: requestPath},
				{Name: portalResponse, Path: "/other/path"},
				{Name: portalResponse, Path: requestPath, Body: []interface{}{uint32(0)}},
			},
			want: BackendXDGPortal,
		},
		{
			name:      "missing status",
			signals:   []*dbus.Signal{{Name: portalResponse, Path: requestPath}},
			wantError: "no status",
		},
		{
			name:      "invalid status",
			signals:   []*dbus.Signal{{Name: portalResponse, Path: requestPath, Body: []interface{}{"zero"}}},
			wantError: "invalid status",
		},
		{
			name:      "cancelled",
			signals:   []*dbus.Signal{{Name: portalResponse, Path: requestPath, Body: []interface{}{uint32(1)}}},
			wantError: "cancelled",
		},
		{
			name:      "failed",
			signals:   []*dbus.Signal{{Name: portalResponse, Path: requestPath, Body: []interface{}{uint32(2)}}},
			wantError: "status 2",
		},
		{
			name:       "session closes",
			closeAfter: true,
			wantError:  "session bus closed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			signals := make(chan *dbus.Signal, len(test.signals))
			for _, signal := range test.signals {
				signals <- signal
			}
			if test.closeAfter {
				close(signals)
			}
			result, err := waitForPortalResponse(context.Background(), signals, requestPath)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("error = %v, want text %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.Backend != test.want {
				t.Fatalf("backend = %q, want %q", result.Backend, test.want)
			}
		})
	}

	t.Run("context expires", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := waitForPortalResponse(ctx, make(chan *dbus.Signal), requestPath)
		if err == nil || !strings.Contains(err.Error(), "did not complete") {
			t.Fatalf("error = %v, want timeout", err)
		}
	})
}

type portalVersionObject struct {
	dbus.BusObject
	call *dbus.Call
}

func (o portalVersionObject) CallWithContext(context.Context, string, dbus.Flags, ...interface{}) *dbus.Call {
	return o.call
}

func TestPortalInterfaceVersion(t *testing.T) {
	for _, test := range []struct {
		name string
		call *dbus.Call
		want uint32
	}{
		{name: "version", call: &dbus.Call{Body: []interface{}{dbus.MakeVariant(uint32(4))}}, want: 4},
		{name: "call error", call: &dbus.Call{Err: errors.New("unavailable")}},
		{name: "unexpected type", call: &dbus.Call{Body: []interface{}{dbus.MakeVariant("four")}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := portalInterfaceVersion(context.Background(), portalVersionObject{call: test.call}); got != test.want {
				t.Fatalf("version = %d, want %d", got, test.want)
			}
		})
	}
}

type fakePortalConnection struct {
	signals          chan<- *dbus.Signal
	addMatchErr      error
	requestCloseCall *dbus.Call
	requestPath      dbus.ObjectPath
	requestMethod    string
	closed           bool
	signalRemoved    bool
	matchRemoved     bool
}

func (c *fakePortalConnection) Close() error {
	c.closed = true
	return nil
}

func (c *fakePortalConnection) Object(_ string, path dbus.ObjectPath) dbus.BusObject {
	c.requestPath = path
	return fakePortalRequestObject{connection: c}
}

func (c *fakePortalConnection) Signal(signals chan<- *dbus.Signal) {
	c.signals = signals
}

func (c *fakePortalConnection) RemoveSignal(chan<- *dbus.Signal) {
	c.signalRemoved = true
}

func (c *fakePortalConnection) AddMatchSignal(...dbus.MatchOption) error {
	return c.addMatchErr
}

func (c *fakePortalConnection) RemoveMatchSignal(...dbus.MatchOption) error {
	c.matchRemoved = true
	return nil
}

type fakePortalRequestObject struct {
	dbus.BusObject
	connection *fakePortalConnection
}

func (o fakePortalRequestObject) CallWithContext(_ context.Context, method string, _ dbus.Flags, _ ...interface{}) *dbus.Call {
	o.connection.requestMethod = method
	if o.connection.requestCloseCall != nil {
		return o.connection.requestCloseCall
	}
	return &dbus.Call{}
}

type fakePortalObject struct {
	dbus.BusObject
	connection  *fakePortalConnection
	versionCall *dbus.Call
	openCall    *dbus.Call
	status      interface{}
	options     map[string]dbus.Variant
}

func (o *fakePortalObject) CallWithContext(_ context.Context, method string, _ dbus.Flags, args ...interface{}) *dbus.Call {
	if method == "org.freedesktop.DBus.Properties.Get" {
		return o.versionCall
	}
	if len(args) == 3 {
		o.options, _ = args[2].(map[string]dbus.Variant)
	}
	if o.openCall.Err == nil && o.status != nil {
		requestPath, _ := o.openCall.Body[0].(dbus.ObjectPath)
		o.connection.signals <- &dbus.Signal{
			Name: portalResponse,
			Path: requestPath,
			Body: []interface{}{o.status},
		}
	}
	return o.openCall
}

func TestOpenURLViaPortal(t *testing.T) {
	t.Run("connection failure", func(t *testing.T) {
		isolateLinuxOpeners(t)
		connectPortalSession = func() (*portalSession, error) {
			return nil, errors.New("no session")
		}
		_, err := openURLViaPortal("https://example.org/")
		if err == nil || !strings.Contains(err.Error(), "no session") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("token failure", func(t *testing.T) {
		isolateLinuxOpeners(t)
		connection := &fakePortalConnection{}
		connectPortalSession = func() (*portalSession, error) {
			return &portalSession{connection: connection, object: &fakePortalObject{}}, nil
		}
		generateHandoffToken = func() (string, error) {
			return "", errors.New("random unavailable")
		}
		_, err := openURLViaPortal("https://example.org/")
		if err == nil || !strings.Contains(err.Error(), "random unavailable") {
			t.Fatalf("error = %v", err)
		}
		if !connection.closed {
			t.Fatal("connection was not closed")
		}
	})

	t.Run("match failure", func(t *testing.T) {
		isolateLinuxOpeners(t)
		connection := &fakePortalConnection{addMatchErr: errors.New("match denied")}
		object := &fakePortalObject{
			connection:  connection,
			versionCall: &dbus.Call{Body: []interface{}{dbus.MakeVariant(uint32(1))}},
			openCall:    &dbus.Call{},
		}
		connectPortalSession = func() (*portalSession, error) {
			return &portalSession{connection: connection, object: object}, nil
		}
		_, err := openURLViaPortal("https://example.org/")
		if err == nil || !strings.Contains(err.Error(), "match denied") {
			t.Fatalf("error = %v", err)
		}
		if !connection.closed || !connection.signalRemoved {
			t.Fatalf("connection cleanup = %#v", connection)
		}
	})

	t.Run("request failure", func(t *testing.T) {
		isolateLinuxOpeners(t)
		connection := &fakePortalConnection{}
		object := &fakePortalObject{
			connection:  connection,
			versionCall: &dbus.Call{Err: errors.New("version unavailable")},
			openCall:    &dbus.Call{Err: errors.New("request denied")},
		}
		connectPortalSession = func() (*portalSession, error) {
			return &portalSession{connection: connection, object: object}, nil
		}
		_, err := openURLViaPortal("https://example.org/")
		if err == nil || !strings.Contains(err.Error(), "request denied") {
			t.Fatalf("error = %v", err)
		}
		if !connection.matchRemoved {
			t.Fatal("match rule was not removed")
		}
	})

	t.Run("request timeout before handle is terminal", func(t *testing.T) {
		isolateLinuxOpeners(t)
		connection := &fakePortalConnection{}
		object := &fakePortalObject{
			connection:  connection,
			versionCall: &dbus.Call{Body: []interface{}{dbus.MakeVariant(uint32(1))}},
			openCall:    &dbus.Call{Err: context.DeadlineExceeded},
		}
		connectPortalSession = func() (*portalSession, error) {
			return &portalSession{connection: connection, object: object}, nil
		}

		_, err := openURLViaPortal("https://example.org/")
		if !errors.Is(err, errPortalFallbackUnsafe) {
			t.Fatalf("error = %v, want terminal timeout", err)
		}
	})

	t.Run("invalid request path", func(t *testing.T) {
		isolateLinuxOpeners(t)
		connection := &fakePortalConnection{}
		object := &fakePortalObject{
			connection:  connection,
			versionCall: &dbus.Call{Body: []interface{}{dbus.MakeVariant(uint32(1))}},
			openCall:    &dbus.Call{Body: []interface{}{dbus.ObjectPath("invalid")}},
		}
		connectPortalSession = func() (*portalSession, error) {
			return &portalSession{connection: connection, object: object}, nil
		}
		_, err := openURLViaPortal("https://example.org/")
		if err == nil || !strings.Contains(err.Error(), "invalid request path") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("success passes activation token", func(t *testing.T) {
		isolateLinuxOpeners(t)
		t.Setenv("XDG_ACTIVATION_TOKEN", "activation")
		connection := &fakePortalConnection{}
		object := &fakePortalObject{
			connection:  connection,
			versionCall: &dbus.Call{Body: []interface{}{dbus.MakeVariant(uint32(4))}},
			openCall:    &dbus.Call{Body: []interface{}{dbus.ObjectPath("/org/freedesktop/portal/desktop/request/test")}},
			status:      uint32(0),
		}
		connectPortalSession = func() (*portalSession, error) {
			return &portalSession{connection: connection, object: object}, nil
		}
		result, err := openURLViaPortal("https://example.org/")
		if err != nil {
			t.Fatal(err)
		}
		if result.Backend != BackendXDGPortal {
			t.Fatalf("backend = %q", result.Backend)
		}
		if token, ok := object.options["activation_token"]; !ok || token.Value() != "activation" {
			t.Fatalf("options = %#v", object.options)
		}
		if handle, ok := object.options["handle_token"]; !ok || !strings.HasPrefix(handle.Value().(string), "crantcli_") {
			t.Fatalf("options = %#v", object.options)
		}
	})

	t.Run("timeout closes request before fallback", func(t *testing.T) {
		isolateLinuxOpeners(t)
		portalRequestTimeout = time.Nanosecond
		requestPath := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/test")
		connection := &fakePortalConnection{}
		object := &fakePortalObject{
			connection:  connection,
			versionCall: &dbus.Call{Body: []interface{}{dbus.MakeVariant(uint32(1))}},
			openCall:    &dbus.Call{Body: []interface{}{requestPath}},
		}
		connectPortalSession = func() (*portalSession, error) {
			return &portalSession{connection: connection, object: object}, nil
		}

		_, err := openURLViaPortal("https://example.org/")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error = %v, want deadline exceeded", err)
		}
		if connection.requestPath != requestPath || connection.requestMethod != portalRequestClose {
			t.Fatalf("closed request = %q %q", connection.requestMethod, connection.requestPath)
		}
	})

	t.Run("failed timeout close is terminal", func(t *testing.T) {
		isolateLinuxOpeners(t)
		portalRequestTimeout = time.Nanosecond
		connection := &fakePortalConnection{requestCloseCall: &dbus.Call{Err: errors.New("close failed")}}
		object := &fakePortalObject{
			connection:  connection,
			versionCall: &dbus.Call{Body: []interface{}{dbus.MakeVariant(uint32(1))}},
			openCall:    &dbus.Call{Body: []interface{}{dbus.ObjectPath("/org/freedesktop/portal/desktop/request/test")}},
		}
		connectPortalSession = func() (*portalSession, error) {
			return &portalSession{connection: connection, object: object}, nil
		}

		_, err := openURLViaPortal("https://example.org/")
		if !errors.Is(err, errPortalFallbackUnsafe) || !strings.Contains(err.Error(), "close failed") {
			t.Fatalf("error = %v, want terminal close failure", err)
		}
	})
}

func TestNewPortalSessionReportsConnectionFailure(t *testing.T) {
	previousBus := connectPortalBus
	t.Cleanup(func() { connectPortalBus = previousBus })

	t.Run("default connector failure", func(t *testing.T) {
		t.Setenv("DBUS_SESSION_BUS_ADDRESS", "invalid:")
		connectPortalBus = previousBus
		connection, err := connectPortalBus()
		if err == nil {
			connection.Close()
			t.Fatal("default connector unexpectedly connected")
		}
	})

	t.Run("failure", func(t *testing.T) {
		connectPortalBus = func() (portalConnection, error) {
			return nil, errors.New("no bus")
		}
		if _, err := newPortalSession(); err == nil {
			t.Fatal("newPortalSession unexpectedly connected")
		}
	})

	t.Run("success", func(t *testing.T) {
		connection := &fakePortalConnection{}
		connectPortalBus = func() (portalConnection, error) {
			return connection, nil
		}
		session, err := newPortalSession()
		if err != nil {
			t.Fatal(err)
		}
		if session.connection != connection || session.object == nil {
			t.Fatalf("session = %#v", session)
		}
	})
}
