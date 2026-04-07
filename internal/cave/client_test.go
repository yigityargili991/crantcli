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
