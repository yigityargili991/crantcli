package seatable

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type seatableRoundTripFunc func(*http.Request) (*http.Response, error)

func (f seatableRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestExecuteSQLPreservesLargeNumericIDs(t *testing.T) {
	oldHTTPClient := httpClient
	t.Cleanup(func() { httpClient = oldHTTPClient })

	httpClient = &http.Client{Transport: seatableRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}
		body := `{
			"metadata": [
				{"key":"0000","name":"root_id","type":"number"},
				{"key":"0001","name":"supervoxel_id","type":"number"}
			],
			"results": [
				{"0000":720575940610453042,"0001":720575940610453043}
			]
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}

	client := &Client{accessToken: "access-token", dtableUUID: "base-uuid"}
	resp, err := client.ExecuteSQL("SELECT `root_id`, `supervoxel_id` FROM `CRANTb_meta` LIMIT 1")
	if err != nil {
		t.Fatalf("ExecuteSQL returned error: %v", err)
	}
	if got, ok := resp.Results[0]["root_id"].(json.Number); !ok || got.String() != "720575940610453042" {
		t.Fatalf("root_id = %#v (%T), want json.Number 720575940610453042", resp.Results[0]["root_id"], resp.Results[0]["root_id"])
	}
	if got, ok := resp.Results[0]["supervoxel_id"].(json.Number); !ok || got.String() != "720575940610453043" {
		t.Fatalf("supervoxel_id = %#v (%T), want json.Number 720575940610453043", resp.Results[0]["supervoxel_id"], resp.Results[0]["supervoxel_id"])
	}
}

// TestNormalizeResultKeys_WithNilRow verifies that normalizeResultKeys handles nil rows gracefully.
func TestNormalizeResultKeys_WithNilRow(t *testing.T) {
	tests := []struct {
		name     string
		resp     *SQLResponse
		wantRows []map[string]interface{}
	}{
		{
			name: "nil row in results",
			resp: &SQLResponse{
				Metadata: []ColumnMeta{
					{Key: "0000", Name: "root_id", Type: "text"},
					{Key: "0001", Name: "name", Type: "text"},
				},
				Results: []map[string]interface{}{
					{"0000": "123", "0001": "test"},
					nil, // This should not cause a panic
					{"0000": "456", "0001": "test2"},
				},
			},
			wantRows: []map[string]interface{}{
				{"0000": "123", "0001": "test", "root_id": "123", "name": "test"},
				nil,
				{"0000": "456", "0001": "test2", "root_id": "456", "name": "test2"},
			},
		},
		{
			name: "all nil rows",
			resp: &SQLResponse{
				Metadata: []ColumnMeta{
					{Key: "0000", Name: "root_id", Type: "text"},
				},
				Results: []map[string]interface{}{
					nil,
					nil,
				},
			},
			wantRows: []map[string]interface{}{
				nil,
				nil,
			},
		},
		{
			name: "empty response",
			resp: &SQLResponse{
				Metadata: []ColumnMeta{},
				Results:  []map[string]interface{}{},
			},
			wantRows: []map[string]interface{}{},
		},
		{
			name:     "nil response",
			resp:     nil,
			wantRows: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should not panic
			normalizeResultKeys(tt.resp)

			if tt.resp == nil {
				return
			}

			// Verify the results match expectations
			if len(tt.resp.Results) != len(tt.wantRows) {
				t.Errorf("normalizeResultKeys() result count = %d, want %d", len(tt.resp.Results), len(tt.wantRows))
				return
			}

			for i, row := range tt.resp.Results {
				if row == nil && tt.wantRows[i] == nil {
					continue
				}
				if row == nil || tt.wantRows[i] == nil {
					t.Errorf("normalizeResultKeys() row[%d] = %v, want %v", i, row, tt.wantRows[i])
					continue
				}

				// Check that keys were normalized correctly for non-nil rows
				for k, v := range tt.wantRows[i] {
					if row[k] != v {
						t.Errorf("normalizeResultKeys() row[%d][%q] = %v, want %v", i, k, row[k], v)
					}
				}
			}
		})
	}
}

// TestNormalizeResultKeys_NormalBehavior tests the normal operation of normalizeResultKeys.
func TestNormalizeResultKeys_NormalBehavior(t *testing.T) {
	resp := &SQLResponse{
		Metadata: []ColumnMeta{
			{Key: "0000", Name: "root_id", Type: "text"},
			{Key: "0001", Name: "status", Type: "text"},
		},
		Results: []map[string]interface{}{
			{"0000": "abc123", "0001": "active"},
			{"0000": "def456", "0001": "inactive"},
		},
	}

	normalizeResultKeys(resp)

	// Check that normalized keys exist
	for i, row := range resp.Results {
		if _, exists := row["root_id"]; !exists {
			t.Errorf("row[%d] missing 'root_id' key", i)
		}
		if _, exists := row["status"]; !exists {
			t.Errorf("row[%d] missing 'status' key", i)
		}

		// Verify values are correct
		if row["root_id"] != row["0000"] {
			t.Errorf("row[%d] root_id = %v, want %v", i, row["root_id"], row["0000"])
		}
		if row["status"] != row["0001"] {
			t.Errorf("row[%d] status = %v, want %v", i, row["status"], row["0001"])
		}
	}
}
