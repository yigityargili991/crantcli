package seatable

import (
	"testing"
)

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
