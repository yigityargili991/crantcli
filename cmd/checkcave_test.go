package cmd

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"crantcli/internal/cave"
	"crantcli/internal/seatable"
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
	if results[0].Status != statusOK {
		t.Errorf("status = %q, want %s", results[0].Status, statusOK)
	}
}

func TestStaleRootMappings(t *testing.T) {
	results := []checkResult{
		{RootID: "111", CaveRootID: "111", Status: statusOK},
		{RootID: "222", CaveRootID: "333", Status: statusStale},
		{RootID: "444", CaveRootID: "-", Status: statusError},
		{RootID: "555", CaveRootID: "666", Status: statusStale},
	}

	got := staleRootMappings(results)
	want := []rootMapping{
		{OldRootID: "222", CurrentRootID: "333"},
		{OldRootID: "555", CurrentRootID: "666"},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d mappings, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mapping[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestWriteMappings(t *testing.T) {
	var buf bytes.Buffer
	mappings := []rootMapping{
		{OldRootID: "222", CurrentRootID: "333"},
		{OldRootID: "555", CurrentRootID: "666"},
	}

	if err := writeMappings(&buf, mappings); err != nil {
		t.Fatalf("writeMappings: %v", err)
	}

	want := "222\t333\n555\t666\n"
	if got := buf.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
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
	if results[0].Status != statusStale {
		t.Errorf("status = %q, want %s", results[0].Status, statusStale)
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
	if results[0].Status != statusNoSupervoxel {
		t.Errorf("status = %q, want %s", results[0].Status, statusNoSupervoxel)
	}
}

func TestCheckNeurons_InvalidSupervoxelID(t *testing.T) {
	c := cave.NewTestClient("http://unused", http.DefaultClient)

	neurons := []seatable.NeuronCaveCheckRow{
		{RootID: "999", SupervoxelID: "not-a-number"},
	}

	results, err := checkNeurons(c, neurons)
	if err != nil {
		t.Fatalf("checkNeurons: %v", err)
	}
	if results[0].Status != statusError {
		t.Errorf("status = %q, want %s", results[0].Status, statusError)
	}
	if results[0].Err == nil {
		t.Error("expected Err to be set for invalid supervoxel_id")
	}
}

func TestCheckNeurons_CaveAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error")
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
	if results[0].Status != statusError {
		t.Errorf("status = %q, want %s", results[0].Status, statusError)
	}
}

func TestCheckNeurons_MixedStatuses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"root_id": 1000}`)
	}))
	defer srv.Close()

	c := cave.NewTestClient(srv.URL, srv.Client())

	neurons := []seatable.NeuronCaveCheckRow{
		{RootID: "1000", SupervoxelID: "100"},
		{RootID: "2000", SupervoxelID: "200"},
		{RootID: "3000", SupervoxelID: ""},
	}

	results, err := checkNeurons(c, neurons)
	if err != nil {
		t.Fatalf("checkNeurons: %v", err)
	}
	if results[0].Status != statusOK {
		t.Errorf("results[0].Status = %q, want %s", results[0].Status, statusOK)
	}
	if results[1].Status != statusStale {
		t.Errorf("results[1].Status = %q, want %s", results[1].Status, statusStale)
	}
	if results[2].Status != statusNoSupervoxel {
		t.Errorf("results[2].Status = %q, want %s", results[2].Status, statusNoSupervoxel)
	}
}

func TestCheckNeurons_BatchMode(t *testing.T) {
	neurons := make([]seatable.NeuronCaveCheckRow, 15)
	for i := range neurons {
		neurons[i] = seatable.NeuronCaveCheckRow{
			RootID:       fmt.Sprintf("%d", 1000+i),
			SupervoxelID: fmt.Sprintf("%d", 100+i),
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if r.Status != statusOK {
			t.Errorf("results[%d].Status = %q, want %s", i, r.Status, statusOK)
		}
	}
}
