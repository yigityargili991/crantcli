package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"crantcli/internal/cave"
)

type caveHistoryCall struct {
	RootID   uint64
	Filtered bool
}

type fakeCaveHistoryClient struct {
	rows      map[uint64][]cave.ChangeLogRow
	errByRoot map[uint64]error
	calls     []caveHistoryCall
}

func (f *fakeCaveHistoryClient) GetRootChangeLog(rootID uint64, filtered bool) ([]cave.ChangeLogRow, error) {
	f.calls = append(f.calls, caveHistoryCall{RootID: rootID, Filtered: filtered})
	if err := f.errByRoot[rootID]; err != nil {
		return nil, err
	}
	return f.rows[rootID], nil
}

func TestRunCaveHistoryInvalidRootID(t *testing.T) {
	client := &fakeCaveHistoryClient{}
	var out, errOut bytes.Buffer

	err := runCaveHistory(&out, &errOut, client, []string{"not-a-root"}, caveHistoryOptions{Filtered: true})
	if err == nil {
		t.Fatal("expected invalid root_id error")
	}
	if !strings.Contains(err.Error(), `invalid root_id "not-a-root"`) {
		t.Fatalf("error = %q", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("client called for invalid root ID: %#v", client.calls)
	}
}

func TestRunCaveHistoryTable(t *testing.T) {
	client := &fakeCaveHistoryClient{
		rows: map[uint64][]cave.ChangeLogRow{
			111: {
				{
					OperationID:     7,
					Timestamp:       1700000000000,
					UserID:          5,
					BeforeRootIDs:   []uint64{10, 11},
					AfterRootIDs:    []uint64{12},
					IsMerge:         true,
					UserName:        "Ada",
					UserAffiliation: "Lab",
				},
			},
		},
	}
	var out, errOut bytes.Buffer

	if err := runCaveHistory(&out, &errOut, client, []string{"111"}, caveHistoryOptions{Filtered: true}); err != nil {
		t.Fatalf("runCaveHistory: %v", err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d output lines, want 2: %q", len(lines), out.String())
	}
	headerFields := strings.Fields(lines[0])
	wantHeader := []string{"root_id", "operation_id", "timestamp_utc", "type", "before_root_ids", "after_root_ids", "user_id", "user_name", "user_affiliation"}
	if strings.Join(headerFields, "|") != strings.Join(wantHeader, "|") {
		t.Fatalf("header fields = %#v, want %#v", headerFields, wantHeader)
	}

	rowFields := strings.Fields(lines[1])
	wantRow := []string{"111", "7", "2023-11-14T22:13:20Z", "merge", "10,11", "12", "5", "Ada", "Lab"}
	if strings.Join(rowFields, "|") != strings.Join(wantRow, "|") {
		t.Fatalf("row fields = %#v, want %#v", rowFields, wantRow)
	}
}

func TestRunCaveHistoryJSON(t *testing.T) {
	client := &fakeCaveHistoryClient{
		rows: map[uint64][]cave.ChangeLogRow{
			222: {
				{
					OperationID:   8,
					Timestamp:     1700000001000,
					UserID:        6,
					BeforeRootIDs: []uint64{720575940610453042},
					AfterRootIDs:  []uint64{720575940610453043, 720575940610453044},
					IsMerge:       false,
					UserName:      "Grace",
				},
			},
		},
	}
	var out, errOut bytes.Buffer

	if err := runCaveHistory(&out, &errOut, client, []string{"222"}, caveHistoryOptions{JSON: true, Filtered: true}); err != nil {
		t.Fatalf("runCaveHistory: %v", err)
	}

	var results []caveHistoryResult
	if err := json.Unmarshal(out.Bytes(), &results); err != nil {
		t.Fatalf("JSON output did not decode: %v\n%s", err, out.String())
	}
	if len(results) != 1 || results[0].RootID != "222" {
		t.Fatalf("decoded results = %#v", results)
	}
	if len(results[0].Entries) != 1 {
		t.Fatalf("entries = %#v", results[0].Entries)
	}
	entry := results[0].Entries[0]
	if entry.Type != "split" || entry.TimestampUTC != "2023-11-14T22:13:21Z" {
		t.Fatalf("entry metadata = %#v", entry)
	}
	if strings.Join(entry.BeforeRootIDs, ",") != "720575940610453042" {
		t.Fatalf("before_root_ids = %#v", entry.BeforeRootIDs)
	}
	if strings.Join(entry.AfterRootIDs, ",") != "720575940610453043,720575940610453044" {
		t.Fatalf("after_root_ids = %#v", entry.AfterRootIDs)
	}
}

func TestRunCaveHistoryMultipleRootIDsUnfiltered(t *testing.T) {
	client := &fakeCaveHistoryClient{
		rows: map[uint64][]cave.ChangeLogRow{
			111: {},
			222: {},
		},
	}
	var out, errOut bytes.Buffer

	if err := runCaveHistory(&out, &errOut, client, []string{"111", "222"}, caveHistoryOptions{JSON: true, Filtered: false}); err != nil {
		t.Fatalf("runCaveHistory: %v", err)
	}
	wantCalls := []caveHistoryCall{{RootID: 111, Filtered: false}, {RootID: 222, Filtered: false}}
	if len(client.calls) != len(wantCalls) {
		t.Fatalf("calls = %#v, want %#v", client.calls, wantCalls)
	}
	for i := range wantCalls {
		if client.calls[i] != wantCalls[i] {
			t.Fatalf("calls[%d] = %#v, want %#v", i, client.calls[i], wantCalls[i])
		}
	}

	var results []caveHistoryResult
	if err := json.Unmarshal(out.Bytes(), &results); err != nil {
		t.Fatalf("JSON output did not decode: %v", err)
	}
	if len(results) != 2 || len(results[0].Entries) != 0 || len(results[1].Entries) != 0 {
		t.Fatalf("results = %#v", results)
	}
}

func TestRunCaveHistoryNoHistoryTable(t *testing.T) {
	client := &fakeCaveHistoryClient{rows: map[uint64][]cave.ChangeLogRow{111: {}}}
	var out, errOut bytes.Buffer

	if err := runCaveHistory(&out, &errOut, client, []string{"111"}, caveHistoryOptions{Filtered: true}); err != nil {
		t.Fatalf("runCaveHistory: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
	if got, want := errOut.String(), "no history found\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestRunCaveHistoryFailsFastWithRootID(t *testing.T) {
	client := &fakeCaveHistoryClient{
		rows:      map[uint64][]cave.ChangeLogRow{111: {}},
		errByRoot: map[uint64]error{222: errors.New("boom")},
	}
	var out, errOut bytes.Buffer

	err := runCaveHistory(&out, &errOut, client, []string{"111", "222", "333"}, caveHistoryOptions{Filtered: true})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "fetching history for root_id 222: boom") {
		t.Fatalf("error = %q", err)
	}
	if len(client.calls) != 2 {
		t.Fatalf("calls = %#v, want fail-fast after second root", client.calls)
	}
}
