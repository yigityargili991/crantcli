// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"crantcli/internal/segprops"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestResolveLabelsHook(t *testing.T) {
	t.Setenv("CRANT_LABELS_HOOK", "env-hook")

	if got := resolveLabelsHook("flag-hook"); got != "flag-hook" {
		t.Fatalf("flag value should win, got %q", got)
	}
	if got := resolveLabelsHook(""); got != "env-hook" {
		t.Fatalf("environment value should be the fallback, got %q", got)
	}
}

func TestResolveLabelOptions(t *testing.T) {
	tests := []struct {
		name      string
		labelBy   string
		labelTags string
		want      segprops.Options
		wantError string
	}{
		{
			name:      "flag defaults reproduce the built-in configuration",
			labelBy:   defaultLabelBy,
			labelTags: defaultLabelTags,
			want:      segprops.DefaultOptions(),
		},
		{
			name:      "label field alone takes no fallback",
			labelBy:   "cell_subtype",
			labelTags: defaultLabelTags,
			want: segprops.Options{
				LabelField:     "cell_subtype",
				LabelFallbacks: []string{},
				TagFields:      segprops.DefaultOptions().TagFields,
			},
		},
		{
			name:      "fields after the first are fallbacks in order",
			labelBy:   "cell_subtype, cell_type ,cell_class",
			labelTags: "region,side",
			want: segprops.Options{
				LabelField:     "cell_subtype",
				LabelFallbacks: []string{"cell_type", "cell_class"},
				TagFields:      []string{"region", "side"},
			},
		},
		{
			name:      "none publishes a label-only source",
			labelBy:   "cell_type",
			labelTags: noLabelTags,
			want:      segprops.Options{LabelField: "cell_type", LabelFallbacks: []string{}},
		},
		{
			name:      "empty tags publish a label-only source",
			labelBy:   "cell_type",
			labelTags: "",
			want:      segprops.Options{LabelField: "cell_type", LabelFallbacks: []string{}},
		},
		{
			name:      "unknown label field",
			labelBy:   "cell_typo",
			labelTags: defaultLabelTags,
			wantError: `invalid --label-by "cell_typo"`,
		},
		{
			name:      "unknown tag field",
			labelBy:   defaultLabelBy,
			labelTags: "cell_class,colour",
			wantError: `invalid --label-tags "colour"`,
		},
		{
			name:      "repeated label field",
			labelBy:   "cell_type,cell_type",
			labelTags: defaultLabelTags,
			wantError: `"cell_type" is repeated`,
		},
		{
			name:      "empty field between commas",
			labelBy:   "cell_type,,cell_class",
			labelTags: defaultLabelTags,
			wantError: "empty field",
		},
		{
			name:      "no label field at all",
			labelBy:   "  ",
			labelTags: defaultLabelTags,
			wantError: "name at least one field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveLabelOptions(tt.labelBy, tt.labelTags)
			if tt.wantError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantError)
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("error = %v, want it to contain %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveLabelOptions returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("options = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestLabelFlagDefaultsKeepBuiltInBehavior(t *testing.T) {
	// Omitting the flags has to publish exactly what the pre-flag builds did.
	opts, err := resolveLabelOptions(defaultLabelBy, defaultLabelTags)
	if err != nil {
		t.Fatal(err)
	}
	want := segprops.DefaultOptions()
	if !reflect.DeepEqual(opts, want) {
		t.Fatalf("default options = %#v, want %#v", opts, want)
	}

	for name, wantDefault := range map[string]string{
		"label-by":   defaultLabelBy,
		"label-tags": defaultLabelTags,
	} {
		flag := addCmd.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("add is missing --%s", name)
		}
		if flag.DefValue != wantDefault {
			t.Errorf("add --%s default = %q, want %q", name, flag.DefValue, wantDefault)
		}
	}
}

func TestWarnUnshapedLabelFlags(t *testing.T) {
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "test"}
		var (
			enabled bool
			ttl     time.Duration
			hook    string
			by      string
			tags    string
		)
		registerLabelFlags(cmd, &enabled, &ttl, &hook, &by, &tags)
		return cmd
	}

	t.Run("silent with --labels", func(t *testing.T) {
		cmd := newCmd()
		mustSetFlag(t, cmd, "label-by", "cell_subtype")
		var out bytes.Buffer
		warnUnshapedLabelFlags(&out, cmd, true)
		if out.Len() != 0 {
			t.Fatalf("output = %q, want none", out.String())
		}
	})

	t.Run("silent when unset", func(t *testing.T) {
		var out bytes.Buffer
		warnUnshapedLabelFlags(&out, newCmd(), false)
		if out.Len() != 0 {
			t.Fatalf("output = %q, want none", out.String())
		}
	})

	t.Run("names both flags", func(t *testing.T) {
		cmd := newCmd()
		mustSetFlag(t, cmd, "label-by", "cell_subtype")
		mustSetFlag(t, cmd, "label-tags", "side")
		var out bytes.Buffer
		warnUnshapedLabelFlags(&out, cmd, false)
		want := "Warning: --label-by and --label-tags have no effect without --labels\n"
		if out.String() != want {
			t.Fatalf("output = %q, want %q", out.String(), want)
		}
	})

	t.Run("names one flag", func(t *testing.T) {
		cmd := newCmd()
		mustSetFlag(t, cmd, "label-tags", "side")
		var out bytes.Buffer
		warnUnshapedLabelFlags(&out, cmd, false)
		want := "Warning: --label-tags has no effect without --labels\n"
		if out.String() != want {
			t.Fatalf("output = %q, want %q", out.String(), want)
		}
	})
}

func TestLabelFieldName(t *testing.T) {
	if got := labelFieldName(segprops.Options{LabelField: "cell_subtype"}); got != "cell_subtype" {
		t.Errorf("labelFieldName = %q, want the field the options name", got)
	}
	// An unset field is one BuildSegmentProperties fills in, so what the run
	// reports has to name that same default rather than nothing at all.
	if got, want := labelFieldName(segprops.Options{}), segprops.DefaultOptions().LabelField; got != want {
		t.Errorf("labelFieldName = %q, want the built-in default %q", got, want)
	}
}

func mustSetFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("setting --%s: %v", name, err)
	}
}

// setFlagForTest sets a flag on a shared command and restores both its value
// and its changed marker afterwards, so one test cannot shape another's run.
func setFlagForTest(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		t.Fatalf("%s has no --%s", cmd.Name(), name)
	}

	previousValue, previouslyChanged := flag.Value.String(), flag.Changed
	mustSetFlag(t, cmd, name, value)
	t.Cleanup(func() {
		mustSetFlag(t, cmd, name, previousValue)
		flag.Changed = previouslyChanged
	})
}

// labelsCleanCall records one invocation of the package-level cleaner seam.
type labelsCleanCall struct {
	all       bool
	olderThan time.Duration
	hook      string
}

// stubLabelsClean replaces the package-level cleaner for a test, recording each
// call and returning a fixed result. The original seam is restored afterwards.
func stubLabelsClean(t *testing.T, deleted, kept int) *[]labelsCleanCall {
	t.Helper()
	calls := &[]labelsCleanCall{}
	original := labelsClean
	t.Cleanup(func() { labelsClean = original })
	labelsClean = func(all bool, olderThan time.Duration, hook string) (int, int, error) {
		*calls = append(*calls, labelsCleanCall{all: all, olderThan: olderThan, hook: hook})
		return deleted, kept, nil
	}
	return calls
}

// labelsCleanSubcommand finds the deprecated 'labels clean' child command.
func labelsCleanSubcommand(t *testing.T) *cobra.Command {
	t.Helper()
	for _, cmd := range labelsCmd.Commands() {
		if cmd.Name() == "clean" {
			return cmd
		}
	}
	t.Fatal("labels has no clean subcommand")
	return nil
}

// resetCommandFlags restores every flag on cmd to its declared default, undoing
// values and changed markers a test run left behind.
func resetCommandFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
}

// runLabelsCommandForTest runs the labels command tree and restores its flags,
// so one run cannot shape the tests that follow it.
func runLabelsCommandForTest(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cleanCmd := labelsCleanSubcommand(t)
	t.Cleanup(func() {
		resetCommandFlags(labelsCmd)
		resetCommandFlags(cleanCmd)
	})
	return executeRootForTest(t, append([]string{"labels"}, args...)...)
}

func TestLabelsCommandIsRunnableWithCleanFlags(t *testing.T) {
	if labelsCmd.RunE == nil {
		t.Fatal("labels must be runnable")
	}
	for _, name := range []string{"clean", "all", "older-than", "labels-hook"} {
		if labelsCmd.Flags().Lookup(name) == nil {
			t.Errorf("labels is missing --%s", name)
		}
	}
	for name, want := range map[string]string{
		"clean":       "false",
		"all":         "false",
		"older-than":  "168h0m0s",
		"labels-hook": "",
	} {
		if got := labelsCmd.Flags().Lookup(name).DefValue; got != want {
			t.Errorf("labels --%s default = %q, want %q", name, got, want)
		}
	}
}

func TestLabelsBareCommandPrintsHelp(t *testing.T) {
	stdout, _, err := runLabelsCommandForTest(t)
	if err != nil {
		t.Fatalf("bare labels returned error: %v", err)
	}
	for _, want := range []string{"Usage:", "--clean", "--all", "--older-than", "--labels-hook", "removes its labels from any saved/shared state", "state-transfer --labels", "v0.25.0"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("bare labels help lacks %q:\n%s", want, stdout)
		}
	}
}

// cleanFlagNames is the single source for registration and for the
// without---clean error; every command that declares the clean flags must
// declare exactly these.
func TestCleanFlagNamesMatchRegisteredFlags(t *testing.T) {
	for _, cmd := range []*cobra.Command{labelsCmd, labelsCleanSubcommand(t)} {
		count := 0
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			// "clean" belongs to the parent command only, and cobra adds its
			// own --help once a command has run.
			if flag.Name != "clean" && flag.Name != "help" {
				count++
			}
		})
		if count != len(cleanFlagNames) {
			t.Fatalf("%s declares %d clean-only flags, want %d (%v)", cmd.Name(), count, len(cleanFlagNames), cleanFlagNames)
		}
		for _, name := range cleanFlagNames {
			if cmd.Flags().Lookup(name) == nil {
				t.Errorf("%s is missing --%s", cmd.Name(), name)
			}
		}
	}
}

// TestLabelsCleanFlagsWithoutCleanError also pins the order cleanFlagNames
// gives the rejection message.
func TestLabelsCleanFlagsWithoutCleanError(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"all", []string{"--all"}},
		{"older-than", []string{"--older-than", "24h"}},
		{"labels-hook", []string{"--labels-hook", "hook-cmd"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runLabelsCommandForTest(t, tt.args...)
			if err == nil {
				t.Fatalf("labels %s without --clean should error", strings.Join(tt.args, " "))
			}
			for _, want := range []string{"--" + tt.name, "--clean"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q should name %q", err, want)
				}
			}
		})
	}

	t.Run("multiple flags keep message order", func(t *testing.T) {
		_, _, err := runLabelsCommandForTest(t, "--all", "--labels-hook", "hook-cmd")
		if err == nil {
			t.Fatal("labels with two clean-only flags should error")
		}
		if want := "--all and --labels-hook can only be used with --clean"; err.Error() != want {
			t.Errorf("error = %q, want %q", err, want)
		}
	})
}

func TestLabelsCleanDeletesAndReports(t *testing.T) {
	calls := stubLabelsClean(t, 2, 1)

	stdout, stderr, err := runLabelsCommandForTest(t, "--clean", "--all", "--older-than", "48h", "--labels-hook", "hook-cmd")
	if err != nil {
		t.Fatalf("labels --clean returned error: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("cleaner called %d times, want 1", len(*calls))
	}
	wantCall := labelsCleanCall{all: true, olderThan: 48 * time.Hour, hook: "hook-cmd"}
	if (*calls)[0] != wantCall {
		t.Errorf("cleaner called with %+v, want %+v", (*calls)[0], wantCall)
	}
	if stdout != "" {
		t.Errorf("labels --clean stdout = %q, want none", stdout)
	}
	if want := "Deleted 2 label source(s); 1 still tracked\n"; stderr != want {
		t.Errorf("labels --clean stderr = %q, want %q", stderr, want)
	}
}

func TestLabelsCleanDefaultsToEnvironmentHook(t *testing.T) {
	t.Setenv("CRANT_LABELS_HOOK", "env-hook")
	calls := stubLabelsClean(t, 0, 0)

	if _, _, err := runLabelsCommandForTest(t, "--clean"); err != nil {
		t.Fatalf("labels --clean returned error: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("cleaner called %d times, want 1", len(*calls))
	}
	wantCall := labelsCleanCall{olderThan: 168 * time.Hour, hook: "env-hook"}
	if (*calls)[0] != wantCall {
		t.Errorf("cleaner called with %+v, want %+v", (*calls)[0], wantCall)
	}
}

func TestLabelsCleanSubcommandDeprecatedAndHidden(t *testing.T) {
	cleanCmd := labelsCleanSubcommand(t)
	if !cleanCmd.Hidden {
		t.Error("labels clean must be hidden from --help")
	}
	for _, want := range []string{"labels --clean", "v0.25.0"} {
		if !strings.Contains(cleanCmd.Deprecated, want) {
			t.Errorf("labels clean deprecation warning %q lacks %q", cleanCmd.Deprecated, want)
		}
	}

	// The hidden subcommand still deletes through the same seam and message.
	calls := stubLabelsClean(t, 3, 0)
	stdout, stderr, err := runLabelsCommandForTest(t, "clean", "--all")
	if err != nil {
		t.Fatalf("labels clean returned error: %v", err)
	}
	if len(*calls) != 1 || !(*calls)[0].all {
		t.Fatalf("cleaner calls = %+v, want one --all call", *calls)
	}
	if !strings.Contains(stdout, "deprecated") || !strings.Contains(stdout, "labels --clean") {
		t.Errorf("labels clean deprecation warning = %q, want it to name labels --clean", stdout)
	}
	if want := "Deleted 3 label source(s); 0 still tracked\n"; stderr != want {
		t.Errorf("labels clean stderr = %q, want %q", stderr, want)
	}

	help, _, err := runLabelsCommandForTest(t, "--help")
	if err != nil {
		t.Fatalf("labels --help returned error: %v", err)
	}
	if strings.Contains(help, "Available Commands:") {
		t.Errorf("labels --help lists the hidden clean subcommand:\n%s", help)
	}
}
