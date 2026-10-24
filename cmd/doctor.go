package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"crantcli/internal/ask"
	"crantcli/internal/cave"
	"crantcli/internal/config"
	"crantcli/internal/nglstate"
	"crantcli/internal/seatable"

	"github.com/spf13/cobra"
)

// Status labels printed at the start of each doctor report line.
const (
	doctorStatusOK    = "ok"
	doctorStatusWarn  = "warn"
	doctorStatusInfo  = "info"
	doctorStatusError = "error"
)

// doctorSmokeSupervoxelID is a known CRANT supervoxel; --online resolves it to
// prove that CAVE accepts the configured token.
const doctorSmokeSupervoxelID uint64 = 73466068970462772

// Test seams: every read outside the process (credentials, default state,
// clients, cache path, model file) is replaceable, so the report can be
// exercised without network access or user configuration.
var (
	doctorLookupSeaTableToken   = lookupSeaTableToken
	doctorLookupCAVEToken       = config.LookupCAVEToken
	doctorLookupCAVEGlobalToken = config.LookupCAVEGlobalToken
	doctorReadDefaultState      = nglstate.ReadDefaultState
	doctorNewSeaTableClient     = seatable.NewClient
	doctorNewCAVEClient         = func() (doctorCAVEClient, error) { return cave.NewClient() }
	doctorCacheDir              = os.UserCacheDir
	doctorStat                  = os.Stat
	doctorCurrentModel          = func() ask.Model { return ask.CurrentModel }
)

// doctorCAVEClient is the part of the CAVE client the online check uses.
type doctorCAVEClient interface {
	GetRootID(supervoxelID uint64) (uint64, error)
}

// lookupSeaTableToken reports whether a SeaTable token is configured and a
// human-readable source, never the token itself. It mirrors the precedence
// GetAPIToken uses: credential store, CRANTTABLE_TOKEN, CRANTTABLE_TOKEN_FILE.
func lookupSeaTableToken() (token, source string) {
	token = config.GetAPIToken()
	if token == "" {
		return "", ""
	}
	switch {
	case config.ReadStoredToken() != "":
		return token, "credential store"
	case strings.TrimSpace(os.Getenv("CRANTTABLE_TOKEN")) != "":
		return token, "environment (CRANTTABLE_TOKEN)"
	default:
		// GetAPIToken found the token here, so CRANTTABLE_TOKEN_FILE is set
		// to a readable file.
		return token, "file (CRANTTABLE_TOKEN_FILE)"
	}
}

// doctorCheck is one line of the doctor report.
type doctorCheck struct {
	status  string
	subject string
	detail  string
}

// doctorReport collects the checks to print and decides the exit status.
type doctorReport struct {
	checks []doctorCheck
}

func (r *doctorReport) add(status, subject, detail string) {
	r.checks = append(r.checks, doctorCheck{status: status, subject: subject, detail: detail})
}

func (r doctorReport) failed() int {
	failed := 0
	for _, check := range r.checks {
		if check.status == doctorStatusError {
			failed++
		}
	}
	return failed
}

func (r doctorReport) write(out io.Writer) error {
	for _, check := range r.checks {
		if _, err := fmt.Fprintf(out, "%-5s %s: %s\n", check.status, check.subject, check.detail); err != nil {
			return err
		}
	}
	return nil
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Report configuration health and test connectivity",
	Long: `Report whether crantcli can find the configuration it needs: the SeaTable
token, the CAVE token, the global CAVE token, the default Neuroglancer state,
and the optional crantcli ask model.

doctor is read-only. It never prints a token value and never changes your
configuration. Each check prints one line with a status of ok, warn, info, or
error.

By default doctor only inspects local configuration. With --online it also
checks that the configured SeaTable token exchanges for a session and that
CAVE answers a root lookup, which requires network access.

The exit status is nonzero when a required check fails, such as a missing
SeaTable token or a custom default state that is not a JSON object. Missing
optional credentials and the ask model only produce warnings or info lines.`,
	Example: `  # Check the local configuration
  crantcli doctor

  # Also test that the SeaTable token exchanges and CAVE answers queries
  crantcli doctor --online`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runDoctor(cmd.OutOrStdout(), doctorOnline)
	},
}

// doctorOnline is set by the --online flag.
var doctorOnline bool

func init() {
	doctorCmd.Flags().BoolVar(&doctorOnline, "online", false, "Also test SeaTable and CAVE connectivity")
	doctorCmd.ValidArgsFunction = noFileCompletion
	rootCmd.AddCommand(doctorCmd)
}

// runDoctor prints the configuration report and returns an error when any
// required check failed.
func runDoctor(out io.Writer, online bool) error {
	var report doctorReport
	report.add(doctorSeaTableCheck())
	report.add(doctorCAVETokenCheck())
	report.add(doctorGlobalCAVETokenCheck())
	report.add(doctorDefaultStateCheck())
	report.add(doctorAskModelCheck())
	if online {
		report.add(doctorSeaTableOnlineCheck())
		report.add(doctorCAVEOnlineCheck())
	} else {
		report.add(doctorStatusInfo, "online checks", "skipped; pass --online to test SeaTable and CAVE connectivity")
	}

	if err := report.write(out); err != nil {
		return fmt.Errorf("writing doctor report: %w", err)
	}
	if failed := report.failed(); failed > 0 {
		return fmt.Errorf("doctor found %d configuration error(s)", failed)
	}
	return nil
}

func doctorSeaTableCheck() (status, subject, detail string) {
	token, source := doctorLookupSeaTableToken()
	if token == "" {
		return doctorStatusError, "SeaTable token",
			"not configured; run 'crantcli setup' or set CRANTTABLE_TOKEN / CRANTTABLE_TOKEN_FILE"
	}
	return doctorStatusOK, "SeaTable token", "configured (source: " + source + ")"
}

func doctorCAVETokenCheck() (status, subject, detail string) {
	token, source := doctorLookupCAVEToken()
	if token == "" {
		return doctorStatusWarn, "CAVE token",
			"not configured; CAVE commands need it (run 'crantcli setup' or set CAVE_TOKEN)"
	}
	return doctorStatusOK, "CAVE token", "configured (source: " + source + ")"
}

func doctorGlobalCAVETokenCheck() (status, subject, detail string) {
	token, source := doctorLookupCAVEGlobalToken()
	if token == "" {
		return doctorStatusInfo, "global CAVE token",
			"not configured; optional, used for Share links stored on global.daf-apis.com"
	}
	return doctorStatusOK, "global CAVE token", "configured (source: " + source + ")"
}

func doctorDefaultStateCheck() (status, subject, detail string) {
	const name = "default Neuroglancer state"

	data, err := doctorReadDefaultState()
	if err != nil {
		return doctorStatusError, name, "could not read the custom default state: " + err.Error()
	}
	if data == nil {
		return doctorStatusOK, name, "built-in template"
	}

	var state map[string]json.RawMessage
	if err := json.Unmarshal(data, &state); err != nil {
		return doctorStatusError, name, "custom default state is not a JSON object: " + err.Error()
	}
	if state == nil {
		return doctorStatusError, name, "custom default state is not a JSON object"
	}
	return doctorStatusOK, name, "custom"
}

func doctorAskModelCheck() (status, subject, detail string) {
	const name = "ask model"

	cacheDir, err := doctorCacheDir()
	if err != nil {
		return doctorStatusWarn, name, "could not find the user cache directory: " + err.Error()
	}
	model := doctorCurrentModel()
	modelPath := ask.ModelPath(filepath.Join(cacheDir, "crantcli", "ask"), model)
	if _, err := doctorStat(modelPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return doctorStatusInfo, name,
				"not installed; optional, run 'crantcli ask --download' to install it"
		}
		return doctorStatusWarn, name, "could not check the installed model: " + err.Error()
	}
	return doctorStatusOK, name, "installed"
}

func doctorSeaTableOnlineCheck() (status, subject, detail string) {
	const name = "SeaTable connection"

	if _, err := doctorNewSeaTableClient(); err != nil {
		return doctorStatusError, name, "token exchange failed: " + err.Error()
	}
	return doctorStatusOK, name, "token exchange succeeded"
}

func doctorCAVEOnlineCheck() (status, subject, detail string) {
	const name = "CAVE connection"

	client, err := doctorNewCAVEClient()
	if err != nil {
		return doctorStatusError, name, "could not create the CAVE client: " + err.Error()
	}
	rootID, err := client.GetRootID(doctorSmokeSupervoxelID)
	if err != nil {
		return doctorStatusError, name, "root lookup failed: " + err.Error()
	}
	return doctorStatusOK, name,
		fmt.Sprintf("reachable (supervoxel %d resolves to root %d)", doctorSmokeSupervoxelID, rootID)
}
