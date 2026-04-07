package cmd

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"crantinject/internal/cave"
	"crantinject/internal/seatable"
)

func TestCheckNeurons_SingleOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"root_id": 999}`)
	}))
	defer srv.Close()

	c := cave.NewTestClient(srv.URL, srv.Client())

	neurons := []seatable.NeuronCaveCheckRow{
		{RootID: "999", SupervoxelID: "100"},
	}

	results, err := checkNeurons(c, neurons)
	if err != nil {
		t.Fatalf("checkNeurons: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Status != "ok" {
		t.Errorf("status = %q, want ok", results[0].Status)
	}
}

func TestCheckNeurons_Stale(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"root_id": 888}`)
	}))
	defer srv.Close()

	c := cave.NewTestClient(srv.URL, srv.Client())

	neurons := []seatable.NeuronCaveCheckRow{
		{RootID: "999", SupervoxelID: "100"},
	}

	results, err := checkNeurons(c, neurons)
	if err != nil {
		t.Fatalf("checkNeurons: %v", err)
	}
	if results[0].Status != "STALE" {
		t.Errorf("status = %q, want STALE", results[0].Status)
	}
	if results[0].CaveRootID != "888" {
		t.Errorf("cave_root_id = %q, want 888", results[0].CaveRootID)
	}
}

func TestCheckNeurons_NoSupervoxel(t *testing.T) {
	c := cave.NewTestClient("http://unused", http.DefaultClient)

	neurons := []seatable.NeuronCaveCheckRow{
		{RootID: "999", SupervoxelID: ""},
	}

	results, err := checkNeurons(c, neurons)
	if err != nil {
		t.Fatalf("checkNeurons: %v", err)
	}
	if results[0].Status != "no_supervoxel" {
		t.Errorf("status = %q, want no_supervoxel", results[0].Status)
	}
}

func TestCheckNeurons_BatchMode(t *testing.T) {
	// Create >10 neurons to trigger batch mode.
	neurons := make([]seatable.NeuronCaveCheckRow, 15)
	for i := range neurons {
		neurons[i] = seatable.NeuronCaveCheckRow{
			RootID:       fmt.Sprintf("%d", 1000+i),
			SupervoxelID: fmt.Sprintf("%d", 100+i),
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return same root IDs as stored (all "ok").
		buf := make([]byte, 8*15)
		for i := range 15 {
			binary.LittleEndian.PutUint64(buf[i*8:], uint64(1000+i))
		}
		w.Write(buf)
	}))
	defer srv.Close()

	c := cave.NewTestClient(srv.URL, srv.Client())

	results, err := checkNeurons(c, neurons)
	if err != nil {
		t.Fatalf("checkNeurons: %v", err)
	}
	for i, r := range results {
		if r.Status != "ok" {
			t.Errorf("results[%d].Status = %q, want ok", i, r.Status)
		}
	}
}
