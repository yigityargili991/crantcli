package skeleton

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestDecodeBridgeError(t *testing.T) {
	err := decodeBridgeError([]byte(`{"ok":false,"error":{"code":"missing_dependency","message":"Python dependency missing: pcg_skel"}}`))
	if err == nil {
		t.Fatal("expected bridge error")
	}
	var bridgeErr *BridgeError
	if !errors.As(err, &bridgeErr) {
		t.Fatalf("error type = %T, want BridgeError", err)
	}
	if bridgeErr.Code != "missing_dependency" || !strings.Contains(bridgeErr.Message, "pcg_skel") {
		t.Fatalf("BridgeError = %#v", bridgeErr)
	}
}

func TestDecodeBridgeErrorRedactsTokenValues(t *testing.T) {
	err := decodeBridgeError([]byte(`{"ok":false,"error":{"code":"failed","message":"Bearer secret-token","details":"api_token=abc123&x=1"}}`))
	if err == nil {
		t.Fatal("expected bridge error")
	}
	text := err.Error()
	for _, secret := range []string{"secret-token", "abc123"} {
		if strings.Contains(text, secret) {
			t.Fatalf("bridge error leaked %q in %q", secret, text)
		}
	}
	if !strings.Contains(text, "[REDACTED]") {
		t.Fatalf("bridge error = %q, want redaction marker", text)
	}
}

func TestBridgeCommandErrorRedactsStderr(t *testing.T) {
	err := bridgeCommandError("request failed: Authorization: Bearer stderr-secret", errors.New("exit status 1"))
	if strings.Contains(err.Error(), "stderr-secret") {
		t.Fatalf("bridge command error leaked token: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("bridge command error = %q, want redaction marker", err.Error())
	}
}

func TestBridgeEnvReplacesCAVEToken(t *testing.T) {
	t.Setenv("CAVE_TOKEN", "old-token")
	t.Setenv("CAVE_TOKEN_FILE", "/tmp/old-cave-token")
	t.Setenv("CRANTTABLE_TOKEN", "seatable-secret")
	t.Setenv("HTTPS_PROXY", "http://proxy.example")
	env := bridgeEnv("new-token")

	var caveEntries []string
	for _, entry := range env {
		if strings.HasPrefix(entry, "CAVE_TOKEN=") {
			caveEntries = append(caveEntries, entry)
		}
	}
	if len(caveEntries) != 1 {
		t.Fatalf("CAVE_TOKEN entries = %#v, want exactly one", caveEntries)
	}
	if caveEntries[0] != "CAVE_TOKEN=new-token" {
		t.Fatalf("CAVE_TOKEN entry = %q, want new token", caveEntries[0])
	}
	if os.Getenv("CAVE_TOKEN") != "old-token" {
		t.Fatalf("bridgeEnv modified process environment")
	}
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "HTTPS_PROXY=http://proxy.example") {
		t.Fatalf("bridgeEnv = %#v, want HTTPS_PROXY preserved", env)
	}
	for _, secretName := range []string{"CAVE_TOKEN_FILE=", "CRANTTABLE_TOKEN="} {
		if strings.Contains(joined, secretName) {
			t.Fatalf("bridgeEnv leaked %s in %#v", secretName, env)
		}
	}
}

func TestEnsureBridgeRuntime(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureBridgeRuntime(dir); err != nil {
		t.Fatalf("EnsureBridgeRuntime: %v", err)
	}
	if err := EnsureBridgeRuntime(dir); err != nil {
		t.Fatalf("EnsureBridgeRuntime second call: %v", err)
	}
}
