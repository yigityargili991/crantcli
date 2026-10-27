package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

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
	doctorLookupSeaTableToken   = config.LookupAPIToken
	doctorLookupCAVEToken       = config.LookupCAVEToken
	doctorLookupCAVEGlobalToken = config.LookupCAVEGlobalToken
	doctorReadDefaultState      = nglstate.ReadDefaultState
	doctorNewSeaTableClient     = seatable.NewClient
	doctorNewCAVEClient         = func() (doctorCAVEClient, error) { return cave.NewClient() }
	doctorCacheDir              = askCacheDir
	doctorStat                  = os.Stat
	doctorCurrentModel          = func() ask.Model { return ask.CurrentModel }
)

// doctorCAVEClient is the part of the CAVE client the online check uses.
type doctorCAVEClient interface {
	GetRootID(supervoxelID uint64) (uint64, error)
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
CAVE answers a root lookup, which requires network access. Without a CAVE
token the CAVE online check reports an info line instead of failing.

The exit status is nonzero when a required check fails, such as a missing
SeaTable token. Missing optional credentials and the ask model only produce
warnings or info lines. A custom default state that cannot be read or is not a
JSON object only produces a warning: crantcli ignores it and uses the built-in
template.`,
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
	const ignored = "; crantcli ignores it and uses the built-in template"

	data, err := doctorReadDefaultState()
	if err != nil {
		return doctorStatusWarn, name, "could not read the custom default state" + ignored + ": " + err.Error()
	}
	if data == nil {
		return doctorStatusOK, name, "built-in template"
	}

	var state map[string]json.RawMessage
	if err := json.Unmarshal(data, &state); err != nil {
		return doctorStatusWarn, name, "custom default state is not a JSON object" + ignored + ": " + err.Error()
	}
	if state == nil {
		return doctorStatusWarn, name, "custom default state is not a JSON object" + ignored
	}
	return doctorStatusOK, name, "custom"
}

func doctorAskModelCheck() (status, subject, detail string) {
	const name = "ask model"

	if custom := os.Getenv("CRANTCLI_ASK_MODEL"); custom != "" {
		if _, err := doctorStat(custom); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return doctorStatusWarn, name, "CRANTCLI_ASK_MODEL points to a missing file: " + custom
			}
			return doctorStatusWarn, name, "could not check the custom model: " + err.Error()
		}
		return doctorStatusOK, name, "custom model (CRANTCLI_ASK_MODEL): " + custom
	}

	cacheDir, err := doctorCacheDir()
	if err != nil {
		return doctorStatusWarn, name, "could not find the user cache directory: " + err.Error()
	}
	model := doctorCurrentModel()
	modelPath := ask.ModelPath(askModelDir(cacheDir), model)
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

	// The CAVE token is optional, so --online must not fail without it; the
	// local CAVE token check already warns about the missing token.
	if token, _ := doctorLookupCAVEToken(); token == "" {
		return doctorStatusInfo, name, "skipped; no CAVE token configured"
	}
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
