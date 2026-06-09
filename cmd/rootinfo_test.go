package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"crantcli/internal/cave"
	"crantcli/internal/seatable"
)

type fakeRootInfoDataSource struct {
	row        *seatable.NeuronInfoRow
	rowErr     error
	regionOpts map[string]string
	positions  []seatable.NeuronPositionRow
	posErr     error
}

func (f *fakeRootInfoDataSource) QueryNeuronInfo(rootID string) (*seatable.NeuronInfoRow, error) {
	if f.rowErr != nil {
		return nil, f.rowErr
	}
	return f.row, nil
}

func (f *fakeRootInfoDataSource) RegionOptions() (map[string]string, error) {
	return f.regionOpts, nil
}

func (f *fakeRootInfoDataSource) QueryEPGPEGPositions(regionOpts map[string]string) ([]seatable.NeuronPositionRow, error) {
	if f.posErr != nil {
		return nil, f.posErr
	}
	return f.positions, nil
}

type fakeRootInfoCaveClient struct {
	currentRoot uint64
	rootErr     error
	rootCalls   int
	requestedSV uint64
	historyRows []cave.ChangeLogRow
	historyErr  error
}

func (f *fakeRootInfoCaveClient) GetRootID(supervoxelID uint64) (uint64, error) {
	f.rootCalls++
	f.requestedSV = supervoxelID
	if f.rootErr != nil {
		return 0, f.rootErr
	}
	return f.currentRoot, nil
}

func (f *fakeRootInfoCaveClient) GetRootChangeLog(rootID uint64, filtered bool) ([]cave.ChangeLogRow, error) {
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	return f.historyRows, nil
}

func TestRunRootInfoJSON(t *testing.T) {
	data := &fakeRootInfoDataSource{
		row: &seatable.NeuronInfoRow{
			RootID:       "111",
			SuperClass:   "central",
			CellClass:    "cx",
			CellType:     "EPG/PEG",
			Side:         "left",
			Region:       "LX",
			SupervoxelID: "999",
			X:            1,
			Y:            2,
			Z:            3,
			PositionSet:  true,
			PositionRaw:  "1,2,3",
			ExtraFields:  map[string]string{"annotation_note": "keep me"},
		},
		positions: []seatable.NeuronPositionRow{
			{RootID: "far", Region: "CX", Side: "left", X: 100, Y: 2, Z: 3, PositionSet: true},
			{RootID: "222", Region: "LX", Side: "right", X: 4, Y: 2, Z: 3, PositionSet: true},
		},
	}
	caveClient := &fakeRootInfoCaveClient{
		currentRoot: 111,
		historyRows: []cave.ChangeLogRow{
			{
				OperationID:   10,
				Timestamp:     1700000000000,
				BeforeRootIDs: []uint64{111},
				AfterRootIDs:  []uint64{112},
				IsMerge:       true,
				UserName:      "Ada",
			},
			{
				OperationID:   11,
				Timestamp:     1700000100000,
				BeforeRootIDs: []uint64{112},
				AfterRootIDs:  []uint64{111},
				IsMerge:       false,
				UserName:      "Grace",
			},
		},
	}

	var out bytes.Buffer
	if err := runRootInfo(&out, data, caveClient, "111", rootInfoOptions{JSON: true, HistoryLimit: 1, Filtered: true}); err != nil {
		t.Fatalf("runRootInfo returned error: %v", err)
	}

	var got rootInfoResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decoding output JSON: %v\n%s", err, out.String())
	}
	if got.CAVE.Status != statusOK {
		t.Fatalf("CAVE.Status = %q, want %s", got.CAVE.Status, statusOK)
	}
	if got.NearestColumn == nil || got.NearestColumn.RootID != "222" {
		t.Fatalf("NearestColumn = %#v, want root 222", got.NearestColumn)
	}
	if got.NearestColumn.SideRelation != "different" {
		t.Fatalf("SideRelation = %q, want different", got.NearestColumn.SideRelation)
	}
	if got.History.Total != 2 || got.History.Merges != 1 || got.History.Splits != 1 {
		t.Fatalf("History summary = %#v, want 2 total, 1 merge, 1 split", got.History)
	}
	if got.History.Latest == nil || got.History.Latest.OperationID != 11 {
		t.Fatalf("Latest = %#v, want operation 11", got.History.Latest)
	}
	if len(got.History.Entries) != 1 || got.History.Entries[0].OperationID != 11 {
		t.Fatalf("History entries = %#v, want newest operation only", got.History.Entries)
	}
	if got.ExtraFields["annotation_note"] != "keep me" {
		t.Fatalf("ExtraFields = %#v, want annotation_note", got.ExtraFields)
	}
}

func TestRunRootInfoTextWithMissingOptionalData(t *testing.T) {
	data := &fakeRootInfoDataSource{
		row: &seatable.NeuronInfoRow{
			RootID:        "111",
			Side:          "left",
			PositionError: "position value is nil",
			ExtraFields:   map[string]string{},
		},
	}
	caveClient := &fakeRootInfoCaveClient{}

	var out bytes.Buffer
	if err := runRootInfo(&out, data, caveClient, "111", rootInfoOptions{HistoryLimit: 5, Filtered: true}); err != nil {
		t.Fatalf("runRootInfo returned error: %v", err)
	}

	text := out.String()
	for _, want := range []string{
		"root_id: 111",
		"status: no_supervoxel",
		"unavailable: position value is nil",
		"edits: 0",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestFetchRootInfoReportsMissingRow(t *testing.T) {
	data := &fakeRootInfoDataSource{}
	caveClient := &fakeRootInfoCaveClient{}

	_, err := fetchRootInfo(data, caveClient, "111", rootInfoOptions{Filtered: true})
	if err == nil {
		t.Fatal("expected missing row error")
	}
	if !strings.Contains(err.Error(), `no neuron found with root_id "111"`) {
		t.Fatalf("error = %q, want missing row", err.Error())
	}
}

func TestFetchRootInfoPropagatesHistoryError(t *testing.T) {
	data := &fakeRootInfoDataSource{
		row: &seatable.NeuronInfoRow{RootID: "111", SupervoxelID: "999", ExtraFields: map[string]string{}},
	}
	caveClient := &fakeRootInfoCaveClient{currentRoot: 111, historyErr: errors.New("boom")}

	_, err := fetchRootInfo(data, caveClient, "111", rootInfoOptions{Filtered: true})
	if err == nil {
		t.Fatal("expected history error")
	}
	if !strings.Contains(err.Error(), "fetching history for root_id 111: boom") {
		t.Fatalf("error = %q, want history context", err.Error())
	}
}

func TestFetchRootInfoHistoryTimeoutIsUnavailable(t *testing.T) {
	data := &fakeRootInfoDataSource{
		row: &seatable.NeuronInfoRow{RootID: "111", ExtraFields: map[string]string{}},
	}
	caveClient := &fakeRootInfoCaveClient{historyErr: cave.ErrChangeLogTimeout}

	result, err := fetchRootInfo(data, caveClient, "111", rootInfoOptions{Filtered: true})
	if err != nil {
		t.Fatalf("fetchRootInfo returned error: %v", err)
	}
	if result.History.Error != cave.ErrChangeLogTimeout.Error() {
		t.Fatalf("History.Error = %q, want timeout", result.History.Error)
	}

	var out bytes.Buffer
	if err := writeRootInfoText(&out, result); err != nil {
		t.Fatalf("writeRootInfoText: %v", err)
	}
	if !strings.Contains(out.String(), "unavailable: CAVE changelog request timed out") {
		t.Fatalf("text output missing unavailable history:\n%s", out.String())
	}
}

func TestSummarizeRootInfoHistoryLimitZero(t *testing.T) {
	history := summarizeRootInfoHistory([]cave.ChangeLogRow{
		{OperationID: 1, Timestamp: 1, IsMerge: true},
		{OperationID: 2, Timestamp: 2, IsMerge: false},
	}, true, 0)

	if history.Total != 2 || history.Merges != 1 || history.Splits != 1 {
		t.Fatalf("history summary = %#v, want counts", history)
	}
	if len(history.Entries) != 0 {
		t.Fatalf("Entries = %#v, want none for limit 0", history.Entries)
	}
	if history.Latest == nil || history.Latest.OperationID != 2 {
		t.Fatalf("Latest = %#v, want operation 2", history.Latest)
	}
}

func TestBuildRootInfoCAVEStatus(t *testing.T) {
	tests := []struct {
		name            string
		row             seatable.NeuronInfoRow
		client          fakeRootInfoCaveClient
		wantStatus      string
		wantCurrentRoot string
		wantError       string
		wantRootCalls   int
		wantRequestedSV uint64
	}{
		{
			name: "stale root",
			row: seatable.NeuronInfoRow{
				RootID:       "111",
				SupervoxelID: "999",
			},
			client:          fakeRootInfoCaveClient{currentRoot: 222},
			wantStatus:      statusStale,
			wantCurrentRoot: "222",
			wantRootCalls:   1,
			wantRequestedSV: 999,
		},
		{
			name: "invalid supervoxel_id",
			row: seatable.NeuronInfoRow{
				RootID:       "111",
				SupervoxelID: "not-a-number",
			},
			client:        fakeRootInfoCaveClient{currentRoot: 222},
			wantStatus:    statusError,
			wantError:     `invalid supervoxel_id "not-a-number"`,
			wantRootCalls: 0,
		},
		{
			name: "CAVE lookup error",
			row: seatable.NeuronInfoRow{
				RootID:       "111",
				SupervoxelID: "999",
			},
			client:          fakeRootInfoCaveClient{rootErr: errors.New("cave unavailable")},
			wantStatus:      statusError,
			wantError:       "cave unavailable",
			wantRootCalls:   1,
			wantRequestedSV: 999,
		},
		{
			name: "invalid row root ID",
			row: seatable.NeuronInfoRow{
				RootID:       "bad-root",
				SupervoxelID: "999",
			},
			client:        fakeRootInfoCaveClient{currentRoot: 222},
			wantStatus:    statusError,
			wantError:     `invalid root_id "bad-root"`,
			wantRootCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.client
			got := buildRootInfoCAVEStatus(&client, &tt.row)

			if got.Status != tt.wantStatus {
				t.Fatalf("Status = %q, want %q", got.Status, tt.wantStatus)
			}
			if got.SupervoxelID != tt.row.SupervoxelID {
				t.Fatalf("SupervoxelID = %q, want %q", got.SupervoxelID, tt.row.SupervoxelID)
			}
			if got.CurrentRootID != tt.wantCurrentRoot {
				t.Fatalf("CurrentRootID = %q, want %q", got.CurrentRootID, tt.wantCurrentRoot)
			}
			if tt.wantError == "" {
				if got.Error != "" {
					t.Fatalf("Error = %q, want empty", got.Error)
				}
			} else if !strings.Contains(got.Error, tt.wantError) {
				t.Fatalf("Error = %q, want to contain %q", got.Error, tt.wantError)
			}
			if client.rootCalls != tt.wantRootCalls {
				t.Fatalf("GetRootID calls = %d, want %d", client.rootCalls, tt.wantRootCalls)
			}
			if client.requestedSV != tt.wantRequestedSV {
				t.Fatalf("GetRootID supervoxelID = %d, want %d", client.requestedSV, tt.wantRequestedSV)
			}
		})
	}
}
