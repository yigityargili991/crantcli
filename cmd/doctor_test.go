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

	"github.com/spf13/pflag"
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
		seaTableSource: "environment (CRANTTABLE_TOKEN)",
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

// resetDoctorFlags restores the doctor command's flags after a command test.
func resetDoctorFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		doctorCmd.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}
	reset()
	t.Cleanup(reset)
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
		"ok    SeaTable token: configured (source: environment (CRANTTABLE_TOKEN))",
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
	wantPath := filepath.Join(state.cacheDir, "crantcli", "ask", state.model.File)
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

func TestDoctorInvalidDefaultStateFails(t *testing.T) {
	tests := []struct {
		name  string
		state []byte
	}{
		{"syntax error", []byte("{not json")},
		{"array", []byte(`[]`)},
		{"null", []byte("null")},
		{"string", []byte(`"state"`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := newDoctorTestState(t)
			state.defaultState = tt.state
			state.install(t)

			var out bytes.Buffer
			err := runDoctor(&out, false)
			if err == nil {
				t.Fatalf("expected an error for an invalid default state\nreport:\n%s", out.String())
			}
			report := out.String()
			if !strings.Contains(report, "error default Neuroglancer state") {
				t.Errorf("report missing default-state error line:\n%s", report)
			}
			if !strings.Contains(report, "not a JSON object") {
				t.Errorf("report does not explain the invalid state:\n%s", report)
			}
		})
	}
}

func TestDoctorDefaultStateReadErrorFails(t *testing.T) {
	state := newDoctorTestState(t)
	state.defaultStateErr = errors.New("permission denied")
	state.install(t)

	var out bytes.Buffer
	err := runDoctor(&out, false)
	if err == nil {
		t.Fatalf("expected an error for an unreadable default state\nreport:\n%s", out.String())
	}
	report := out.String()
	if !strings.Contains(report, "error default Neuroglancer state: could not read") {
		t.Errorf("report missing read-error line:\n%s", report)
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
		"ok    CAVE connection: reachable (supervoxel 73466068970462772 resolves to root 720575940610453042)",
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
	resetDoctorFlags(t)

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
	resetDoctorFlags(t)

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
