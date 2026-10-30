package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"crantcli/internal/ask"
	"crantcli/internal/seatable"
)

// fakeDoctorCAVE is a doctorCAVEClient whose answer is fixed by the test.
type fakeDoctorCAVE struct {
	rootID uint64
	err    error
}

func (f fakeDoctorCAVE) GetRootID(uint64) (uint64, error) { return f.rootID, f.err }

// doctorTestState holds the values the doctor seams return in a test.
type doctorTestState struct {
	seaTableToken   string
	seaTableSource  string
	caveToken       string
	caveSource      string
	globalToken     string
	globalSource    string
	defaultState    []byte
	defaultStateErr error
	seatableErr     error
	caveClient      doctorCAVEClient
	caveClientErr   error
	cacheDir        string
	cacheDirErr     error
	model           ask.Model
	statErr         error
	statPath        string
}

// newDoctorTestState returns a fully configured state: every token and the
// ask model are present, and no custom default state is set.
func newDoctorTestState(t *testing.T) *doctorTestState {
	t.Helper()
	return &doctorTestState{
		seaTableToken:  "seatable-token-value",
		seaTableSource: "CRANTTABLE_TOKEN",
		caveToken:      "cave-token-value",
		caveSource:     "CAVE_TOKEN",
		globalToken:    "global-cave-token-value",
		globalSource:   "CAVE_GLOBAL_TOKEN",
		caveClient:     fakeDoctorCAVE{rootID: 720575940610453042},
		cacheDir:       t.TempDir(),
		model:          ask.Model{Version: "test-v1", File: "test-model.gguf"},
	}
}

// install replaces every doctor seam for the duration of the test.
func (s *doctorTestState) install(t *testing.T) {
	t.Helper()
	t.Setenv("CRANTCLI_ASK_MODEL", "")
	previousSeaTable := doctorLookupSeaTableToken
	previousCAVE := doctorLookupCAVEToken
	previousGlobal := doctorLookupCAVEGlobalToken
	previousDefaultState := doctorReadDefaultState
	previousNewSeaTable := doctorNewSeaTableClient
	previousNewCAVE := doctorNewCAVEClient
	previousCacheDir := doctorCacheDir
	previousStat := doctorStat
	previousModel := doctorCurrentModel
	t.Cleanup(func() {
		doctorLookupSeaTableToken = previousSeaTable
		doctorLookupCAVEToken = previousCAVE
		doctorLookupCAVEGlobalToken = previousGlobal
		doctorReadDefaultState = previousDefaultState
		doctorNewSeaTableClient = previousNewSeaTable
		doctorNewCAVEClient = previousNewCAVE
		doctorCacheDir = previousCacheDir
		doctorStat = previousStat
		doctorCurrentModel = previousModel
	})

	doctorLookupSeaTableToken = func() (string, string) { return s.seaTableToken, s.seaTableSource }
	doctorLookupCAVEToken = func() (string, string) { return s.caveToken, s.caveSource }
	doctorLookupCAVEGlobalToken = func() (string, string) { return s.globalToken, s.globalSource }
	doctorReadDefaultState = func() ([]byte, error) { return s.defaultState, s.defaultStateErr }
	doctorNewSeaTableClient = func() (*seatable.Client, error) {
		if s.seatableErr != nil {
			return nil, s.seatableErr
		}
		return &seatable.Client{}, nil
	}
	doctorNewCAVEClient = func() (doctorCAVEClient, error) {
		if s.caveClientErr != nil {
			return nil, s.caveClientErr
		}
		return s.caveClient, nil
	}
	doctorCacheDir = func() (string, error) { return s.cacheDir, s.cacheDirErr }
	doctorStat = func(path string) (os.FileInfo, error) {
		s.statPath = path
		return nil, s.statErr
	}
	doctorCurrentModel = func() ask.Model { return s.model }
}

// assertDoctorHidesSecrets fails when report contains any of the token values.
func assertDoctorHidesSecrets(t *testing.T, report string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		if strings.Contains(report, secret) {
			t.Errorf("report leaks token value %q:\n%s", secret, report)
		}
	}
}

func TestDoctorAllConfiguredOffline(t *testing.T) {
	state := newDoctorTestState(t)
	state.install(t)

	var out bytes.Buffer
	if err := runDoctor(&out, false); err != nil {
		t.Fatalf("runDoctor returned error: %v\nreport:\n%s", err, out.String())
	}
	report := out.String()
	for _, want := range []string{
		"ok    SeaTable token: configured (source: CRANTTABLE_TOKEN)",
		"ok    CAVE token: configured (source: CAVE_TOKEN)",
		"ok    global CAVE token: configured (source: CAVE_GLOBAL_TOKEN)",
		"ok    default Neuroglancer state: built-in template",
		"ok    ask model: installed",
		"info  online checks: skipped",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
	wantPath := filepath.Join(askModelDir(state.cacheDir), state.model.File)
	if state.statPath != wantPath {
		t.Errorf("checked model path = %q, want %q", state.statPath, wantPath)
	}
	assertDoctorHidesSecrets(t, report, state.seaTableToken, state.caveToken, state.globalToken)
}

func TestDoctorMissingSeaTableTokenFails(t *testing.T) {
	state := newDoctorTestState(t)
	state.seaTableToken = ""
	state.seaTableSource = ""
	state.install(t)

	var out bytes.Buffer
	err := runDoctor(&out, false)
	if err == nil {
		t.Fatalf("expected an error for a missing SeaTable token\nreport:\n%s", out.String())
	}
	report := out.String()
	if !strings.Contains(report, "error SeaTable token: not configured") {
		t.Errorf("report missing SeaTable error line:\n%s", report)
	}
	if !strings.Contains(report, "crantcli setup") {
		t.Errorf("report missing setup guidance:\n%s", report)
	}
	if !strings.Contains(err.Error(), "1 configuration error") {
		t.Errorf("error = %q, want one configuration error", err.Error())
	}
	assertDoctorHidesSecrets(t, report, state.seaTableToken, state.caveToken, state.globalToken)
}

func TestDoctorOptionalItemsMissingDoesNotFail(t *testing.T) {
	state := newDoctorTestState(t)
	state.caveToken = ""
	state.caveSource = ""
	state.globalToken = ""
	state.globalSource = ""
	state.statErr = fmt.Errorf("stat model: %w", os.ErrNotExist)
	state.install(t)

	var out bytes.Buffer
	if err := runDoctor(&out, false); err != nil {
		t.Fatalf("optional items missing should not fail: %v\nreport:\n%s", err, out.String())
	}
	report := out.String()
	for _, want := range []string{
		"warn  CAVE token: not configured",
		"info  global CAVE token: not configured",
		"info  ask model: not installed",
		"crantcli ask --download",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "error ") {
		t.Errorf("optional items missing produced an error line:\n%s", report)
	}
}

func TestDoctorCustomDefaultStateValid(t *testing.T) {
	state := newDoctorTestState(t)
	state.defaultState = []byte(`{"layers": [], "custom": true}`)
	state.install(t)

	var out bytes.Buffer
	if err := runDoctor(&out, false); err != nil {
		t.Fatalf("valid custom state should not fail: %v\nreport:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "ok    default Neuroglancer state: custom") {
		t.Errorf("report missing custom-state ok line:\n%s", out.String())
	}
}

// TestDoctorInvalidDefaultStateWarns covers the invalid custom states
// nglstate.LoadState silently ignores: they must not fail doctor.
func TestDoctorInvalidDefaultStateWarns(t *testing.T) {
	tests := []struct {
		name  string
		state []byte
	}{
		{"syntax error", []byte("{not json")},
		{"array", []byte(`[]`)},
		{"null", []byte("null")},
		{"string", []byte(`"state"`)},
		{"empty file", []byte{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := newDoctorTestState(t)
			state.defaultState = tt.state
			state.install(t)

			var out bytes.Buffer
			if err := runDoctor(&out, false); err != nil {
				t.Fatalf("an invalid custom default state should only warn: %v\nreport:\n%s", err, out.String())
			}
			report := out.String()
			if !strings.Contains(report, "warn  default Neuroglancer state") {
				t.Errorf("report missing default-state warning line:\n%s", report)
			}
			if !strings.Contains(report, "ignores it and uses the built-in template") {
				t.Errorf("report does not explain the fallback:\n%s", report)
			}
			if strings.Contains(report, "error ") {
				t.Errorf("invalid default state produced an error line:\n%s", report)
			}
		})
	}
}

func TestDoctorDefaultStateReadErrorWarns(t *testing.T) {
	state := newDoctorTestState(t)
	state.defaultStateErr = errors.New("permission denied")
	state.install(t)

	var out bytes.Buffer
	if err := runDoctor(&out, false); err != nil {
		t.Fatalf("an unreadable custom default state should only warn: %v\nreport:\n%s", err, out.String())
	}
	report := out.String()
	if !strings.Contains(report, "warn  default Neuroglancer state: could not read the custom default state") {
		t.Errorf("report missing read-warning line:\n%s", report)
	}
	if !strings.Contains(report, "ignores it and uses the built-in template") {
		t.Errorf("report does not explain the fallback:\n%s", report)
	}
	if strings.Contains(report, "error ") {
		t.Errorf("unreadable default state produced an error line:\n%s", report)
	}
}

func TestDoctorAskModelCacheDirErrorIsWarnOnly(t *testing.T) {
	state := newDoctorTestState(t)
	state.cacheDirErr = errors.New("no cache directory")
	state.install(t)

	var out bytes.Buffer
	if err := runDoctor(&out, false); err != nil {
		t.Fatalf("a cache-directory problem is optional and must not fail: %v", err)
	}
	if !strings.Contains(out.String(), "warn  ask model: could not find the user cache directory") {
		t.Errorf("report missing ask-model warning:\n%s", out.String())
	}
}

func TestDoctorAskModelFromEnv(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		state := newDoctorTestState(t)
		state.cacheDirErr = errors.New("no cache directory")
		state.install(t)
		doctorStat = os.Stat

		modelPath := filepath.Join(t.TempDir(), "custom-model.gguf")
		if err := os.WriteFile(modelPath, []byte("gguf"), 0o600); err != nil {
			t.Fatalf("write custom model: %v", err)
		}
		t.Setenv("CRANTCLI_ASK_MODEL", modelPath)

		var out bytes.Buffer
		if err := runDoctor(&out, false); err != nil {
			t.Fatalf("a configured custom model should not fail: %v\nreport:\n%s", err, out.String())
		}
		report := out.String()
		if want := "ok    ask model: custom model (CRANTCLI_ASK_MODEL): " + modelPath; !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
		if strings.Contains(report, "could not find the user cache directory") {
			t.Errorf("a custom model should not need the cache directory:\n%s", report)
		}
	})

	t.Run("missing", func(t *testing.T) {
		state := newDoctorTestState(t)
		state.install(t)
		doctorStat = os.Stat

		modelPath := filepath.Join(t.TempDir(), "missing-model.gguf")
		t.Setenv("CRANTCLI_ASK_MODEL", modelPath)

		var out bytes.Buffer
		if err := runDoctor(&out, false); err != nil {
			t.Fatalf("a missing custom model should only warn: %v\nreport:\n%s", err, out.String())
		}
		report := out.String()
		if want := "warn  ask model: CRANTCLI_ASK_MODEL points to a missing file: " + modelPath; !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
		if strings.Contains(report, "error ") {
			t.Errorf("missing custom model produced an error line:\n%s", report)
		}
	})
}

func TestAskModelDir(t *testing.T) {
	cacheRoot := t.TempDir()
	want := filepath.Join(cacheRoot, "crantcli", "ask")
	if got := askModelDir(cacheRoot); got != want {
		t.Fatalf("askModelDir(%q) = %q, want %q", cacheRoot, got, want)
	}
}

func TestDoctorOnlineSuccess(t *testing.T) {
	state := newDoctorTestState(t)
	state.caveClient = fakeDoctorCAVE{rootID: 720575940610453042}
	state.install(t)

	var out bytes.Buffer
	if err := runDoctor(&out, true); err != nil {
		t.Fatalf("online checks should succeed: %v\nreport:\n%s", err, out.String())
	}
	report := out.String()
	for _, want := range []string{
		"ok    SeaTable connection: token exchange succeeded",
		fmt.Sprintf("ok    CAVE connection: reachable (supervoxel %d resolves to root 720575940610453042)", doctorSmokeSupervoxelID),
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "online checks: skipped") {
		t.Errorf("online checks were reported as skipped:\n%s", report)
	}
	assertDoctorHidesSecrets(t, report, state.seaTableToken, state.caveToken, state.globalToken)
}

func TestDoctorOnlineFailure(t *testing.T) {
	state := newDoctorTestState(t)
	state.seatableErr = errors.New("token rejected")
	state.caveClientErr = errors.New("CAVE unreachable")
	state.install(t)

	var out bytes.Buffer
	err := runDoctor(&out, true)
	if err == nil {
		t.Fatalf("expected online failures to fail doctor\nreport:\n%s", out.String())
	}
	report := out.String()
	for _, want := range []string{
		"error SeaTable connection: token exchange failed: token rejected",
		"error CAVE connection: could not create the CAVE client: CAVE unreachable",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
	if !strings.Contains(err.Error(), "2 configuration error") {
		t.Errorf("error = %q, want two configuration errors", err.Error())
	}
}

func TestDoctorOnlineCAVERootLookupFailure(t *testing.T) {
	state := newDoctorTestState(t)
	state.caveClient = fakeDoctorCAVE{err: errors.New("lookup failed")}
	state.install(t)

	var out bytes.Buffer
	err := runDoctor(&out, true)
	if err == nil {
		t.Fatalf("expected a CAVE root-lookup failure to fail doctor\nreport:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "error CAVE connection: root lookup failed: lookup failed") {
		t.Errorf("report missing CAVE lookup error line:\n%s", out.String())
	}
}

func TestDoctorOnlineWithoutCAVETokenSkipsCheck(t *testing.T) {
	state := newDoctorTestState(t)
	state.caveToken = ""
	state.caveSource = ""
	state.install(t)
	doctorNewCAVEClient = func() (doctorCAVEClient, error) {
		t.Error("CAVE client must not be created without a CAVE token")
		return nil, errors.New("unexpected CAVE client")
	}

	var out bytes.Buffer
	if err := runDoctor(&out, true); err != nil {
		t.Fatalf("a missing optional CAVE token must not fail --online: %v\nreport:\n%s", err, out.String())
	}
	report := out.String()
	for _, want := range []string{
		"ok    SeaTable connection: token exchange succeeded",
		"info  CAVE connection: skipped; no CAVE token configured",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "error ") {
		t.Errorf("missing optional CAVE token produced an error line:\n%s", report)
	}
}

func TestDoctorOnlineWithoutSeaTableTokenSkipsCheck(t *testing.T) {
	state := newDoctorTestState(t)
	state.seaTableToken = ""
	state.seaTableSource = ""
	state.install(t)
	doctorNewSeaTableClient = func() (*seatable.Client, error) {
		t.Error("SeaTable client must not be created without a SeaTable token")
		return nil, errors.New("unexpected SeaTable client")
	}

	var out bytes.Buffer
	err := runDoctor(&out, true)
	if err == nil {
		t.Fatalf("a missing required SeaTable token must still fail doctor\nreport:\n%s", out.String())
	}
	report := out.String()
	for _, want := range []string{
		"error SeaTable token: not configured",
		"info  SeaTable connection: skipped; no SeaTable token configured",
		"ok    CAVE connection: reachable",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "token exchange failed") {
		t.Errorf("online check duplicates the missing-token error:\n%s", report)
	}
	if !strings.Contains(err.Error(), "1 configuration error") {
		t.Errorf("error = %q, want exactly one configuration error", err.Error())
	}
	assertDoctorHidesSecrets(t, report, state.seaTableToken, state.caveToken, state.globalToken)
}

func TestDoctorNeverPrintsTokenValues(t *testing.T) {
	state := newDoctorTestState(t)
	state.seaTableToken = "SECRET-SEATABLE-TOKEN"
	state.seaTableSource = "credential store"
	state.caveToken = "SECRET-CAVE-TOKEN"
	state.caveSource = "crantcli's credential store"
	state.globalToken = "SECRET-GLOBAL-TOKEN"
	state.globalSource = "crantcli's credential store"
	state.install(t)

	for _, online := range []bool{false, true} {
		var out bytes.Buffer
		if err := runDoctor(&out, online); err != nil {
			t.Fatalf("runDoctor(online=%v): %v\nreport:\n%s", online, err, out.String())
		}
		assertDoctorHidesSecrets(t, out.String(), state.seaTableToken, state.caveToken, state.globalToken)
	}
}

func TestDoctorCommandWritesReportToCommandOutput(t *testing.T) {
	state := newDoctorTestState(t)
	state.install(t)
	resetCommandFlags(doctorCmd)
	t.Cleanup(func() { resetCommandFlags(doctorCmd) })

	if doctorCmd.Hidden {
		t.Fatal("doctor must be a visible command")
	}
	out, errOut, err := executeRootForTest(t, "doctor")
	if err != nil {
		t.Fatalf("doctor via root command: %v\nstderr:\n%s", err, errOut)
	}
	for _, want := range []string{
		"ok    SeaTable token: configured",
		"ok    default Neuroglancer state: built-in template",
		"info  online checks: skipped",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("command output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorOnlineFlagViaCommand(t *testing.T) {
	state := newDoctorTestState(t)
	state.install(t)
	resetCommandFlags(doctorCmd)
	t.Cleanup(func() { resetCommandFlags(doctorCmd) })

	out, errOut, err := executeRootForTest(t, "doctor", "--online")
	if err != nil {
		t.Fatalf("doctor --online: %v\nstderr:\n%s", err, errOut)
	}
	for _, want := range []string{
		"ok    SeaTable connection: token exchange succeeded",
		"ok    CAVE connection: reachable",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("command output missing %q:\n%s", want, out)
		}
	}
}
