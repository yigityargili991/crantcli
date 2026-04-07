package cmd

import (
	"bytes"
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
