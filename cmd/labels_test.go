package cmd

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"crantcli/internal/segprops"

	"github.com/spf13/cobra"
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

func mustSetFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("setting --%s: %v", name, err)
	}
}
