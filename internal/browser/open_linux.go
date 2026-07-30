//go:build linux

package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	portalDestination = "org.freedesktop.portal.Desktop"
	portalPath        = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	portalOpenURI     = "org.freedesktop.portal.OpenURI.OpenURI"
	portalResponse    = "org.freedesktop.portal.Request.Response"
	portalTimeout     = 15 * time.Second

	// MAX_ARG_STRLEN caps a single exec argument at 128 KiB on Linux. Leave
	// headroom and hand anything larger to the command openers through a
	// private local redirect file instead.
	maxSafeOpenArgument = 96 << 10
)

var openViaPortal = openURLViaPortal

func platformOpenURL(rawURL string) (OpenResult, error) {
	// The portal path stages no file, so it would never sweep otherwise; a
	// session that stages once during a portal outage must not keep that URL
	// on disk indefinitely.
	sweepStaleHandoffs()

	var failures []error
	if result, err := openViaPortal(rawURL); err == nil {
		return result, nil
	} else {
		failures = append(failures, fmt.Errorf("XDG desktop portal: %w", err))
	}

	commandURL, err := prepareCommandOpenURL(rawURL)
	if err != nil {
		failures = append(failures, fmt.Errorf("preparing browser handoff: %w", err))
		return OpenResult{}, fmt.Errorf("no Linux browser opener succeeded: %w", errors.Join(failures...))
	}
	for _, candidate := range []struct {
		backend Backend
		name    string
		args    []string
	}{
		{backend: BackendXDGOpen, name: "xdg-open", args: []string{commandURL}},
		{backend: BackendGIO, name: "gio", args: []string{"open", commandURL}},
	} {
		result, err := runPlatformCommand(candidate.backend, candidate.name, candidate.args...)
		if err == nil {
			return result, nil
		}
		failures = append(failures, err)
	}
	return OpenResult{}, fmt.Errorf("no Linux browser opener succeeded: %w", errors.Join(failures...))
}

func openURLViaPortal(rawURL string) (OpenResult, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return OpenResult{}, fmt.Errorf("connecting to session bus: %w", err)
	}
	defer conn.Close()

	token, err := handoffToken()
	if err != nil {
		return OpenResult{}, err
	}
	object := conn.Object(portalDestination, portalPath)
	options := map[string]dbus.Variant{"handle_token": dbus.MakeVariant(token)}
	versionCtx, cancelVersion := context.WithTimeout(context.Background(), time.Second)
	version := portalInterfaceVersion(versionCtx, object)
	cancelVersion()
	if activationToken := os.Getenv("XDG_ACTIVATION_TOKEN"); activationToken != "" && version >= 4 {
		options["activation_token"] = dbus.MakeVariant(activationToken)
	}

	ctx, cancel := context.WithTimeout(context.Background(), portalTimeout)
	defer cancel()

	signals := make(chan *dbus.Signal, 4)
	conn.Signal(signals)
	defer conn.RemoveSignal(signals)
	match := []dbus.MatchOption{
		dbus.WithMatchSender(portalDestination),
		dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response"),
	}
	if err := conn.AddMatchSignal(match...); err != nil {
		return OpenResult{}, fmt.Errorf("subscribing to portal response: %w", err)
	}
	defer conn.RemoveMatchSignal(match...)

	var requestPath dbus.ObjectPath
	if err := object.CallWithContext(ctx, portalOpenURI, 0, "", rawURL, options).Store(&requestPath); err != nil {
		return OpenResult{}, fmt.Errorf("requesting URL open: %w", err)
	}
	if !requestPath.IsValid() {
		return OpenResult{}, fmt.Errorf("portal returned invalid request path %q", requestPath)
	}

	for {
		select {
		case signal, ok := <-signals:
			if !ok {
				return OpenResult{}, fmt.Errorf("session bus closed before portal response")
			}
			if signal == nil || signal.Name != portalResponse || signal.Path != requestPath {
				continue
			}
			if len(signal.Body) == 0 {
				return OpenResult{}, fmt.Errorf("portal response had no status")
			}
			status, ok := signal.Body[0].(uint32)
			if !ok {
				return OpenResult{}, fmt.Errorf("portal returned invalid status %T", signal.Body[0])
			}
			switch status {
			case 0:
				return OpenResult{Backend: BackendXDGPortal}, nil
			case 1:
				return OpenResult{}, fmt.Errorf("request was cancelled")
			default:
				return OpenResult{}, fmt.Errorf("request failed with status %d", status)
			}
		case <-ctx.Done():
			return OpenResult{}, fmt.Errorf("request did not complete within %s", portalTimeout)
		}
	}
}

func portalInterfaceVersion(ctx context.Context, object dbus.BusObject) uint32 {
	var value dbus.Variant
	if err := object.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0,
		"org.freedesktop.portal.OpenURI", "version").Store(&value); err != nil {
		return 0
	}
	version, _ := value.Value().(uint32)
	return version
}
