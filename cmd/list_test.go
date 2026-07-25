package cmd

import (
	"bytes"
	"strings"
	"testing"

	"crantcli/internal/seatable"
)

func TestWriteDistinctResults(t *testing.T) {
	resp := &seatable.SQLResponse{
		Results: []map[string]interface{}{
			{"region": "LX", "count": 2},
			{"region": "LW", "count": 1},
		},
	}

	var buf bytes.Buffer
	if err := writeDistinctResults(&buf, "region", resp, true); err != nil {
		t.Fatalf("writeDistinctResults returned error: %v", err)
	}

	want := "LX                                       2\nLW                                       1\n"
	if buf.String() != want {
		t.Fatalf("writeDistinctResults output = %q, want %q", buf.String(), want)
	}
}

func TestValidateListField(t *testing.T) {
	if err := validateListField("cell_instance"); err != nil {
		t.Fatalf("validateListField(cell_instance) returned error: %v", err)
	}

	err := validateListField("column")
	if err == nil {
		t.Fatal("expected column to be rejected as a raw list field")
	}
	if !strings.Contains(err.Error(), `invalid field "column"`) {
		t.Fatalf("error = %q, want invalid field message", err.Error())
	}
}

func TestListInvalidFieldValidatesBeforeTokenSetup(t *testing.T) {
	out, errOut, err := executeRootForTest(t, "list", "column")
	if err == nil {
		t.Fatal("expected invalid field error")
	}
	if !strings.Contains(err.Error(), `invalid field "column"`) {
		t.Fatalf("error = %q, want invalid field message", err.Error())
	}
	if !strings.Contains(out, "Usage:") {
		t.Fatalf("stdout = %q, want Cobra usage", out)
	}
	if errOut != "" {
		t.Fatalf("stderr = %q, want empty because root command silences Cobra duplicate errors", errOut)
	}
}

// SEC-001 regression: server-provided values must not reach the terminal with
// control characters intact.
func TestWriteDistinctResults_SanitizesValues(t *testing.T) {
	resp := &seatable.SQLResponse{
		Results: []map[string]interface{}{
			{"cell_type": "evil\x1b]52;c;SGFjZ2Vk\x07\x1b[31m", "count": 3},
		},
	}

	var buf bytes.Buffer
	if err := writeDistinctResults(&buf, "cell_type", resp, true); err != nil {
		t.Fatalf("writeDistinctResults returned error: %v", err)
	}
	if strings.ContainsRune(buf.String(), '\x1b') {
		t.Fatalf("output contains ESC: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "evil") {
		t.Fatalf("output lost printable content: %q", buf.String())
	}
}
