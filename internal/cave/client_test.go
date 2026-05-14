package cave

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Client{
		token:     "test-token",
		baseURL:   srv.URL,
		tableName: "test_table",
		http:      srv.Client(),
	}
}

func TestGetRootID(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		want := "/segmentation/api/v1/table/test_table/node/123456789/root"
		if r.URL.Path != want {
			t.Errorf("unexpected path: got %s, want %s", r.URL.Path, want)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"root_id": 720575940610453042}`)
	}))

	rootID, err := c.GetRootID(123456789)
	if err != nil {
		t.Fatalf("GetRootID: %v", err)
	}
	if rootID != 720575940610453042 {
		t.Errorf("got root_id %d, want 720575940610453042", rootID)
	}
}

func TestGetRootID_LargeUint64(t *testing.T) {
	// Verify uint64 precision is preserved (exceeds float64 53-bit mantissa).
	const expected uint64 = 720575940610453042
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"root_id": %d}`, expected)
	}))

	rootID, err := c.GetRootID(1)
	if err != nil {
		t.Fatalf("GetRootID: %v", err)
	}
	if rootID != expected {
		t.Errorf("got %d, want %d (uint64 precision lost)", rootID, expected)
	}
}

func TestGetRootID_HTTPError(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "invalid token")
	}))

	_, err := c.GetRootID(1)
	if err == nil {
		t.Fatal("expected error for HTTP 401")
	}
}

func TestGetRootIDs(t *testing.T) {
	input := []uint64{100, 200, 300}
	expected := []uint64{1000, 2000, 3000}

	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Errorf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}

		buf := make([]byte, 8*len(expected))
		for i, id := range expected {
			binary.LittleEndian.PutUint64(buf[i*8:], id)
		}
		w.Write(buf)
	}))

	rootIDs, err := c.GetRootIDs(input)
	if err != nil {
		t.Fatalf("GetRootIDs: %v", err)
	}
	if len(rootIDs) != len(expected) {
		t.Fatalf("got %d root IDs, want %d", len(rootIDs), len(expected))
	}
	for i, got := range rootIDs {
		if got != expected[i] {
			t.Errorf("rootIDs[%d] = %d, want %d", i, got, expected[i])
		}
	}
}

func TestGetRootIDs_Empty(t *testing.T) {
	c := &Client{token: "t", baseURL: "http://unused", tableName: "t", http: http.DefaultClient}
	rootIDs, err := c.GetRootIDs(nil)
	if err != nil {
		t.Fatalf("GetRootIDs(nil): %v", err)
	}
	if rootIDs != nil {
		t.Errorf("expected nil, got %v", rootIDs)
	}
}

func TestGetRootIDs_BadResponseLength(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte{1, 2, 3}) // Not a multiple of 8.
	}))

	_, err := c.GetRootIDs([]uint64{1})
	if err == nil {
		t.Fatal("expected error for bad response length")
	}
}

func TestGetRootChangeLog(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		want := "/segmentation/api/v1/table/test_table/root/720575940610453042/tabular_change_log"
		if r.URL.Path != want {
			t.Errorf("unexpected path: got %s, want %s", r.URL.Path, want)
		}
		if got := r.URL.Query().Get("filtered"); got != "true" {
			t.Errorf("filtered query = %q, want true", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{
			"operation_id": 123,
			"timestamp": 1700000000000,
			"user_id": 42,
			"before_root_ids": [720575940610453042, 720575940610453043],
			"after_root_ids": [720575940610453044],
			"is_merge": true,
			"user_name": "Ada",
			"user_affiliation": "CRANT"
		}]`)
	}))

	rows, err := c.GetRootChangeLog(720575940610453042, true)
	if err != nil {
		t.Fatalf("GetRootChangeLog: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	row := rows[0]
	if row.OperationID != 123 || row.Timestamp != 1700000000000 || row.UserID != 42 {
		t.Fatalf("decoded scalar fields incorrectly: %#v", row)
	}
	if len(row.BeforeRootIDs) != 2 || row.BeforeRootIDs[0] != 720575940610453042 || row.BeforeRootIDs[1] != 720575940610453043 {
		t.Fatalf("before_root_ids = %v", row.BeforeRootIDs)
	}
	if len(row.AfterRootIDs) != 1 || row.AfterRootIDs[0] != 720575940610453044 {
		t.Fatalf("after_root_ids = %v", row.AfterRootIDs)
	}
	if !row.IsMerge || row.UserName != "Ada" || row.UserAffiliation != "CRANT" {
		t.Fatalf("decoded metadata incorrectly: %#v", row)
	}
}

func TestGetRootChangeLog_ColumnarResponse(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"operation_id": {"12": 460496, "11": 460495},
			"timestamp": {"12": 1772639709709, "11": 1772639424192},
			"user_id": {"12": "110", "11": "110"},
			"before_root_ids": {
				"12": [576460752692696295, 576460752696292404],
				"11": [576460752684881005]
			},
			"after_root_ids": {
				"12": [576460752756408394],
				"11": [576460752696292404]
			},
			"is_merge": {"12": true, "11": false},
			"user_name": {"12": "Marcel Sayre", "11": "Marcel Sayre"},
			"user_affiliation": {"12": "", "11": ""}
		}`)
	}))

	rows, err := c.GetRootChangeLog(576460752688642351, true)
	if err != nil {
		t.Fatalf("GetRootChangeLog: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	if got, want := rows[0].OperationID, uint64(460495); got != want {
		t.Fatalf("rows[0].OperationID = %d, want %d", got, want)
	}
	if got, want := rows[0].Timestamp, int64(1772639424192); got != want {
		t.Fatalf("rows[0].Timestamp = %d, want %d", got, want)
	}
	if got, want := rows[0].UserID, uint64(110); got != want {
		t.Fatalf("rows[0].UserID = %d, want %d", got, want)
	}
	if rows[0].IsMerge {
		t.Fatal("rows[0].IsMerge = true, want false")
	}
	if len(rows[0].BeforeRootIDs) != 1 || rows[0].BeforeRootIDs[0] != 576460752684881005 {
		t.Fatalf("rows[0].BeforeRootIDs = %v", rows[0].BeforeRootIDs)
	}
	if len(rows[0].AfterRootIDs) != 1 || rows[0].AfterRootIDs[0] != 576460752696292404 {
		t.Fatalf("rows[0].AfterRootIDs = %v", rows[0].AfterRootIDs)
	}
	if got, want := rows[0].UserName, "Marcel Sayre"; got != want {
		t.Fatalf("rows[0].UserName = %q, want %q", got, want)
	}

	if got, want := rows[1].OperationID, uint64(460496); got != want {
		t.Fatalf("rows[1].OperationID = %d, want %d", got, want)
	}
	if !rows[1].IsMerge {
		t.Fatal("rows[1].IsMerge = false, want true")
	}
}

func TestGetRootChangeLog_Unfiltered(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("filtered"); got != "false" {
			t.Errorf("filtered query = %q, want false", got)
		}
		fmt.Fprint(w, `[]`)
	}))

	rows, err := c.GetRootChangeLog(1, false)
	if err != nil {
		t.Fatalf("GetRootChangeLog: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("got %d rows, want 0", len(rows))
	}
}

func TestGetRootChangeLog_HTTPError(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, "upstream failure")
	}))

	_, err := c.GetRootChangeLog(1, true)
	if err == nil {
		t.Fatal("expected error for HTTP 502")
	}
}

func TestGetRootChangeLog_ReadTimeoutMeansNoHistory(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message": "Read timed out"}`)
	}))

	rows, err := c.GetRootChangeLog(1, true)
	if err != nil {
		t.Fatalf("GetRootChangeLog: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("got %d rows, want 0", len(rows))
	}
}
