package skeleton

import "testing"

func TestCacheReadWriteSkeleton(t *testing.T) {
	cache := NewCache(t.TempDir())
	want := &Skeleton{
		RootID: "111",
		Source: "test",
		Nodes:  []SkeletonNode{{ID: 1, X: 1}, {ID: 2, X: 2}},
		Edges:  []SkeletonEdge{{From: 1, To: 2}},
	}
	if err := cache.WriteSkeleton(want); err != nil {
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
