package skeleton

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCacheReadWriteSkeleton(t *testing.T) {
	cache := NewCache(t.TempDir())
	want := &Skeleton{
		RootID: "111",
		Source: "test",
		Nodes:  []SkeletonNode{{ID: 1, X: 1}, {ID: 2, X: 2}},
		Edges:  []SkeletonEdge{{From: 1, To: 2}},
	}
	if err := cache.WriteSkeleton("111", want); err != nil {
		t.Fatalf("WriteSkeleton: %v", err)
	}

	got, ok, err := cache.ReadSkeleton("111")
	if err != nil {
		t.Fatalf("ReadSkeleton: %v", err)
	}
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.RootID != want.RootID || got.Source != want.Source || len(got.Nodes) != 2 || len(got.Edges) != 1 {
		t.Fatalf("cached skeleton = %#v, want %#v", got, want)
	}
}

func TestCacheReadSkeletonRejectsMismatchedRootID(t *testing.T) {
	cache := NewCache(t.TempDir())
	path := cache.SkeletonPath("111")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("creating cache dir: %v", err)
	}
	data := []byte(`{"root_id":"222","nodes":[{"id":1}],"edges":[]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing cache file: %v", err)
	}

	got, ok, err := cache.ReadSkeleton("111")
	if err != nil {
		t.Fatalf("ReadSkeleton: %v", err)
	}
	if ok || got != nil {
		t.Fatalf("got hit %#v, want mismatched cache miss", got)
	}
}

func TestCacheReadSkeletonRejectsMissingRootID(t *testing.T) {
	cache := NewCache(t.TempDir())
	path := cache.SkeletonPath("111")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("creating cache dir: %v", err)
	}
	data := []byte(`{"nodes":[{"id":1}],"edges":[]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing cache file: %v", err)
	}

	got, ok, err := cache.ReadSkeleton("111")
	if err != nil {
		t.Fatalf("ReadSkeleton: %v", err)
	}
	if ok || got != nil {
		t.Fatalf("got hit %#v, want rootless cache miss", got)
	}
}

func TestCacheWriteSkeletonRejectsMismatchedRootID(t *testing.T) {
	cache := NewCache(t.TempDir())
	err := cache.WriteSkeleton("111", &Skeleton{
		RootID: "222",
		Nodes:  []SkeletonNode{{ID: 1}},
	})
	if err == nil {
		t.Fatal("expected mismatched root id error")
	}
	if !strings.Contains(err.Error(), "does not match requested root_id") {
		t.Fatalf("error = %q, want mismatch guidance", err.Error())
	}
}

func TestCacheWriteSkeletonFillsRequestedRootID(t *testing.T) {
	cache := NewCache(t.TempDir())
	if err := cache.WriteSkeleton("111", &Skeleton{
		Nodes: []SkeletonNode{{ID: 1}},
	}); err != nil {
		t.Fatalf("WriteSkeleton: %v", err)
	}

	got, ok, err := cache.ReadSkeleton("111")
	if err != nil {
		t.Fatalf("ReadSkeleton: %v", err)
	}
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.RootID != "111" {
		t.Fatalf("RootID = %q, want requested root id", got.RootID)
	}
}

func TestCacheMiss(t *testing.T) {
	cache := NewCache(t.TempDir())
	got, ok, err := cache.ReadSkeleton("missing")
	if err != nil {
		t.Fatalf("ReadSkeleton: %v", err)
	}
	if ok || got != nil {
		t.Fatalf("got hit %#v, want miss", got)
	}
}

func TestCacheReadWriteViewerInfo(t *testing.T) {
	cache := NewCache(t.TempDir())
	want := ViewerInfo{
		RootID: "111",
		Lines:  []string{"root_info", "cell: EPG/PEG"},
	}
	if err := cache.WriteViewerInfo(want); err != nil {
		t.Fatalf("WriteViewerInfo: %v", err)
	}

	got, ok, err := cache.ReadViewerInfo("111")
	if err != nil {
		t.Fatalf("ReadViewerInfo: %v", err)
	}
	if !ok {
		t.Fatal("expected viewer info cache hit")
	}
	if got.RootID != want.RootID || len(got.Lines) != 2 || got.Lines[1] != "cell: EPG/PEG" {
		t.Fatalf("cached viewer info = %#v, want %#v", got, want)
	}
}

func TestViewerInfoCacheMiss(t *testing.T) {
	cache := NewCache(t.TempDir())
	got, ok, err := cache.ReadViewerInfo("missing")
	if err != nil {
		t.Fatalf("ReadViewerInfo: %v", err)
	}
	if ok || got.RootID != "" {
		t.Fatalf("got hit %#v, want miss", got)
	}
}

func TestViewerInfoErrorIsCacheMiss(t *testing.T) {
	cache := NewCache(t.TempDir())
	want := ViewerInfo{
		RootID: "111",
		Error:  "SeaTable unavailable",
	}
	if err := cache.WriteViewerInfo(want); err != nil {
		t.Fatalf("WriteViewerInfo: %v", err)
	}

	got, ok, err := cache.ReadViewerInfo("111")
	if err != nil {
		t.Fatalf("ReadViewerInfo: %v", err)
	}
	if ok {
		t.Fatalf("ok = true, want cached errors to be retryable misses")
	}
	if got.Error != want.Error {
		t.Fatalf("cached viewer info = %#v, want error retained for caller", got)
	}
}
