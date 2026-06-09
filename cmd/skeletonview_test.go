package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"crantcli/internal/config"
	"crantcli/internal/skeleton"
)

func testSkeleton() *skeleton.Skeleton {
	return &skeleton.Skeleton{
		RootID: "111",
		Source: "test",
		Nodes:  []skeleton.SkeletonNode{{ID: 1, X: 1}, {ID: 2, X: 2}},
		Edges:  []skeleton.SkeletonEdge{{From: 1, To: 2}},
	}
}

type fakeSkeletonRemote struct {
	exists      bool
	existsSeq   []bool
	estimate    float64
	existsCalls int
	queueCalls  int
}

func (f *fakeSkeletonRemote) SkeletonExists(context.Context, string) (bool, error) {
	f.existsCalls++
	if len(f.existsSeq) > 0 {
		exists := f.existsSeq[0]
		f.existsSeq = f.existsSeq[1:]
		return exists, nil
	}
	return f.exists, nil
}

func (f *fakeSkeletonRemote) QueueSkeleton(context.Context, string) (float64, error) {
	f.queueCalls++
	return f.estimate, nil
}

func TestRunSkeletonViewInvalidRootID(t *testing.T) {
	err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "bad", skeletonViewOptions{}, skeletonViewDeps{})
	if err == nil {
		t.Fatal("expected invalid root id error")
	}
	if !strings.Contains(err.Error(), "invalid root_id") {
		t.Fatalf("error = %q, want invalid root_id", err.Error())
	}
}

func TestRunSkeletonViewMissingCAVEToken(t *testing.T) {
	err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{}, skeletonViewDeps{
		token:     func() string { return "" },
		cacheRoot: func() (string, error) { return t.TempDir(), nil },
	})
	if err == nil {
		t.Fatal("expected missing token error")
	}
	if !strings.Contains(err.Error(), "no CAVE token configured") {
		t.Fatalf("error = %q, want missing token", err.Error())
	}
}

func TestRunSkeletonViewMissingUV(t *testing.T) {
	err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{JSONDebug: true}, skeletonViewDeps{
		token:      func() string { return "token" },
		lookPathUV: func() (string, error) { return "", errors.New(skeleton.MissingUVMessage) },
		cacheRoot:  func() (string, error) { return t.TempDir(), nil },
		fetch: func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error) {
			t.Fatal("fetch should not run without uv")
			return nil, nil
		},
		remote: func(string) skeletonRemote { return &fakeSkeletonRemote{exists: true} },
	})
	if err == nil {
		t.Fatal("expected missing uv error")
	}
	if !strings.Contains(err.Error(), "uv is required") {
		t.Fatalf("error = %q, want uv guidance", err.Error())
	}
}

func TestRunSkeletonViewJSONDebugDoesNotOpenViewer(t *testing.T) {
	var out bytes.Buffer
	var launched bool
	var fetched int
	err := runSkeletonView(context.Background(), &out, &bytes.Buffer{}, "111", skeletonViewOptions{JSONDebug: true}, skeletonViewDeps{
		token:      func() string { return "token" },
		lookPathUV: func() (string, error) { return "/usr/bin/uv", nil },
		cacheRoot:  func() (string, error) { return t.TempDir(), nil },
		fetch: func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error) {
			fetched++
			return testSkeleton(), nil
		},
		remote: func(string) skeletonRemote { return &fakeSkeletonRemote{exists: true} },
		viewerPath: func() (string, error) {
			launched = true
			return "/unused", nil
		},
		launch: func(context.Context, string, string, string, string, int) error {
			launched = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runSkeletonView: %v", err)
	}
	if launched {
		t.Fatal("viewer should not open in json debug mode")
	}
	if fetched != 1 {
		t.Fatalf("fetched = %d, want 1", fetched)
	}
	var got skeleton.Skeleton
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decoding json output: %v", err)
	}
	if got.RootID != "111" || len(got.Nodes) != 2 {
		t.Fatalf("output skeleton = %#v", got)
	}
}

func TestRunSkeletonViewUsesGlobalSkeletoncacheServer(t *testing.T) {
	err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{JSONDebug: true}, skeletonViewDeps{
		token:      func() string { return "token" },
		lookPathUV: func() (string, error) { return "/usr/bin/uv", nil },
		cacheRoot:  func() (string, error) { return t.TempDir(), nil },
		fetch: func(_ context.Context, opts skeleton.BridgeOptions) (*skeleton.Skeleton, error) {
			if opts.Server != config.CAVEGlobalServer {
				t.Fatalf("bridge server = %q, want %q", opts.Server, config.CAVEGlobalServer)
			}
			return testSkeleton(), nil
		},
		remote: func(string) skeletonRemote { return &fakeSkeletonRemote{exists: true} },
	})
	if err != nil {
		t.Fatalf("runSkeletonView: %v", err)
	}
}

func TestRunSkeletonViewRejectsFetchedSkeletonRootIDMismatch(t *testing.T) {
	err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{JSONDebug: true}, skeletonViewDeps{
		token:      func() string { return "token" },
		lookPathUV: func() (string, error) { return "/usr/bin/uv", nil },
		cacheRoot:  func() (string, error) { return t.TempDir(), nil },
		fetch: func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error) {
			sk := testSkeleton()
			sk.RootID = "222"
			return sk, nil
		},
		remote: func(string) skeletonRemote { return &fakeSkeletonRemote{exists: true} },
	})
	if err == nil {
		t.Fatal("expected fetched skeleton root_id mismatch error")
	}
	if !strings.Contains(err.Error(), "does not match requested root_id") {
		t.Fatalf("error = %q, want mismatch guidance", err.Error())
	}
}

func TestRunSkeletonViewUsesSkeletonCache(t *testing.T) {
	cacheDir := t.TempDir()
	var fetched int
	deps := skeletonViewDeps{
		token:      func() string { return "token" },
		lookPathUV: func() (string, error) { return "/usr/bin/uv", nil },
		cacheRoot:  func() (string, error) { return cacheDir, nil },
		fetch: func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error) {
			fetched++
			return testSkeleton(), nil
		},
		remote:     func(string) skeletonRemote { return &fakeSkeletonRemote{exists: true} },
		viewerPath: func() (string, error) { return "/viewer", nil },
		launch:     func(context.Context, string, string, string, string, int) error { return nil },
	}

	if err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{JSONDebug: true}, deps); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{JSONDebug: true}, deps); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if fetched != 1 {
		t.Fatalf("fetched = %d, want one fetch because second run uses cache", fetched)
	}

	if err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{JSONDebug: true, NoCache: true}, deps); err != nil {
		t.Fatalf("no-cache run: %v", err)
	}
	if fetched != 2 {
		t.Fatalf("fetched = %d, want no-cache to refetch", fetched)
	}
}

func TestRunSkeletonViewUsesCachedSkeletonWithoutUV(t *testing.T) {
	cacheDir := t.TempDir()
	cache := skeleton.NewCache(cacheDir)
	if err := cache.WriteSkeleton("111", testSkeleton()); err != nil {
		t.Fatalf("writing cache: %v", err)
	}

	var uvCalled bool
	var out bytes.Buffer
	err := runSkeletonView(context.Background(), &out, &bytes.Buffer{}, "111", skeletonViewOptions{JSONDebug: true}, skeletonViewDeps{
		token: func() string { return "token" },
		lookPathUV: func() (string, error) {
			uvCalled = true
			return "", errors.New(skeleton.MissingUVMessage)
		},
		cacheRoot: func() (string, error) { return cacheDir, nil },
		fetch: func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error) {
			t.Fatal("fetch should not run on cache hit")
			return nil, nil
		},
		remote: func(string) skeletonRemote { return &fakeSkeletonRemote{exists: true} },
	})
	if err != nil {
		t.Fatalf("runSkeletonView: %v", err)
	}
	if uvCalled {
		t.Fatal("uv lookup should not run on cache hit")
	}
	var got skeleton.Skeleton
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decoding json output: %v", err)
	}
	if got.RootID != "111" || len(got.Nodes) != 2 {
		t.Fatalf("output skeleton = %#v", got)
	}
}

func TestRunSkeletonViewUsesCachedSkeletonWithoutCAVEToken(t *testing.T) {
	cacheDir := t.TempDir()
	cache := skeleton.NewCache(cacheDir)
	if err := cache.WriteSkeleton("111", testSkeleton()); err != nil {
		t.Fatalf("writing cache: %v", err)
	}

	var out bytes.Buffer
	remote := &fakeSkeletonRemote{exists: true}
	err := runSkeletonView(context.Background(), &out, &bytes.Buffer{}, "111", skeletonViewOptions{JSONDebug: true}, skeletonViewDeps{
		token:     func() string { return "" },
		cacheRoot: func() (string, error) { return cacheDir, nil },
		fetch: func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error) {
			t.Fatal("fetch should not run on cache hit")
			return nil, nil
		},
		remote: func(string) skeletonRemote { return remote },
	})
	if err != nil {
		t.Fatalf("runSkeletonView: %v", err)
	}
	if remote.existsCalls != 0 || remote.queueCalls != 0 {
		t.Fatalf("remote calls = exists %d queue %d, want none on cache hit", remote.existsCalls, remote.queueCalls)
	}
	var got skeleton.Skeleton
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decoding json output: %v", err)
	}
	if got.RootID != "111" {
		t.Fatalf("RootID = %q, want cached root", got.RootID)
	}
}

func TestRunSkeletonViewCacheMissRequiresCAVETokenBeforeRemote(t *testing.T) {
	remote := &fakeSkeletonRemote{exists: true}
	err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{JSONDebug: true}, skeletonViewDeps{
		token:     func() string { return "" },
		cacheRoot: func() (string, error) { return t.TempDir(), nil },
		fetch: func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error) {
			t.Fatal("fetch should not run without a CAVE token")
			return nil, nil
		},
		remote: func(string) skeletonRemote { return remote },
	})
	if err == nil {
		t.Fatal("expected missing CAVE token error")
	}
	if !strings.Contains(err.Error(), "no CAVE token configured") {
		t.Fatalf("error = %q, want missing token guidance", err.Error())
	}
	if remote.existsCalls != 0 || remote.queueCalls != 0 {
		t.Fatalf("remote calls = exists %d queue %d, want none without token", remote.existsCalls, remote.queueCalls)
	}
}

func TestRunSkeletonViewSkipsLiveRootInfoWithoutCAVEToken(t *testing.T) {
	cacheDir := t.TempDir()
	cache := skeleton.NewCache(cacheDir)
	if err := cache.WriteSkeleton("111", testSkeleton()); err != nil {
		t.Fatalf("writing cache: %v", err)
	}

	var gotInfoPath string
	err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{}, skeletonViewDeps{
		token:      func() string { return "" },
		cacheRoot:  func() (string, error) { return cacheDir, nil },
		viewerPath: func() (string, error) { return "/viewer", nil },
		rootInfo: func(string) ([]string, error) {
			t.Fatal("root info should not be fetched without credentials")
			return nil, nil
		},
		launch: func(_ context.Context, _, _, infoPath, _ string, _ int) error {
			gotInfoPath = infoPath
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runSkeletonView: %v", err)
	}
	if gotInfoPath != "" {
		t.Fatalf("infoPath = %q, want no live root-info path without token", gotInfoPath)
	}
}

func TestRunSkeletonViewLaunchesHelper(t *testing.T) {
	cacheDir := t.TempDir()
	var gotViewerPath, gotSkeletonPath, gotInfoPath, gotProjection string
	var gotMaxNodes int
	err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{
		Projection: string(skeleton.ProjectionIso),
		MaxNodes:   25,
	}, skeletonViewDeps{
		token:      func() string { return "token" },
		lookPathUV: func() (string, error) { return "/usr/bin/uv", nil },
		cacheRoot:  func() (string, error) { return cacheDir, nil },
		fetch: func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error) {
			return testSkeleton(), nil
		},
		remote:     func(string) skeletonRemote { return &fakeSkeletonRemote{exists: true} },
		viewerPath: func() (string, error) { return "/viewer", nil },
		rootInfo: func(rootID string) ([]string, error) {
			if rootID != "111" {
				t.Fatalf("rootInfo rootID = %q, want 111", rootID)
			}
			return []string{"root_info", "cell: EPG/PEG", "cave: ok"}, nil
		},
		launch: func(_ context.Context, viewerPath, skeletonPath, infoPath, projection string, maxNodes int) error {
			gotViewerPath = viewerPath
			gotSkeletonPath = skeletonPath
			gotInfoPath = infoPath
			gotProjection = projection
			gotMaxNodes = maxNodes
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runSkeletonView: %v", err)
	}
	if gotViewerPath != "/viewer" {
		t.Fatalf("viewer path = %q, want /viewer", gotViewerPath)
	}
	if !strings.HasSuffix(gotSkeletonPath, "skeletons/111.json") {
		t.Fatalf("skeleton path = %q, want cache skeleton path", gotSkeletonPath)
	}
	if !strings.HasSuffix(gotInfoPath, "skeleton-info/111.json") {
		t.Fatalf("info path = %q, want cache info path", gotInfoPath)
	}
	infoData, err := os.ReadFile(gotInfoPath)
	if err != nil {
		t.Fatalf("reading info file: %v", err)
	}
	if !strings.Contains(string(infoData), "cell: EPG/PEG") {
		t.Fatalf("info file = %s, want root-info line", string(infoData))
	}
	if gotProjection != "iso" || gotMaxNodes != 25 {
		t.Fatalf("projection/max = %q/%d, want iso/25", gotProjection, gotMaxNodes)
	}
}

func TestRunSkeletonViewLaunchesWhenRootInfoFails(t *testing.T) {
	cacheDir := t.TempDir()
	var gotInfoPath string
	err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{}, skeletonViewDeps{
		token:      func() string { return "token" },
		lookPathUV: func() (string, error) { return "/usr/bin/uv", nil },
		cacheRoot:  func() (string, error) { return cacheDir, nil },
		fetch: func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error) {
			return testSkeleton(), nil
		},
		remote:     func(string) skeletonRemote { return &fakeSkeletonRemote{exists: true} },
		viewerPath: func() (string, error) { return "/viewer", nil },
		rootInfo:   func(string) ([]string, error) { return nil, errors.New("SeaTable unavailable") },
		launch: func(_ context.Context, _, _, infoPath, _ string, _ int) error {
			gotInfoPath = infoPath
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runSkeletonView: %v", err)
	}
	infoData, err := os.ReadFile(gotInfoPath)
	if err != nil {
		t.Fatalf("reading info file: %v", err)
	}
	if !strings.Contains(string(infoData), "SeaTable unavailable") {
		t.Fatalf("info file = %s, want root-info error", string(infoData))
	}
}

func TestRunSkeletonViewUsesRootInfoCache(t *testing.T) {
	cacheDir := t.TempDir()
	var rootInfoCalls int
	deps := skeletonViewDeps{
		token:      func() string { return "token" },
		lookPathUV: func() (string, error) { return "/usr/bin/uv", nil },
		cacheRoot:  func() (string, error) { return cacheDir, nil },
		fetch: func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error) {
			return testSkeleton(), nil
		},
		remote:     func(string) skeletonRemote { return &fakeSkeletonRemote{exists: true} },
		viewerPath: func() (string, error) { return "/viewer", nil },
		rootInfo: func(string) ([]string, error) {
			rootInfoCalls++
			return []string{"root_info", "cell: cached"}, nil
		},
		launch: func(context.Context, string, string, string, string, int) error { return nil },
	}

	if err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{}, deps); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{}, deps); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if rootInfoCalls != 1 {
		t.Fatalf("rootInfoCalls = %d, want 1 because second run uses cached viewer info", rootInfoCalls)
	}
	if err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{NoCache: true}, deps); err != nil {
		t.Fatalf("no-cache run: %v", err)
	}
	if rootInfoCalls != 2 {
		t.Fatalf("rootInfoCalls = %d, want --no-cache to refresh viewer info", rootInfoCalls)
	}
}

func TestRunSkeletonViewRetriesCachedRootInfoError(t *testing.T) {
	cacheDir := t.TempDir()
	var rootInfoCalls int
	var gotInfoPath string
	deps := skeletonViewDeps{
		token:      func() string { return "token" },
		lookPathUV: func() (string, error) { return "/usr/bin/uv", nil },
		cacheRoot:  func() (string, error) { return cacheDir, nil },
		fetch: func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error) {
			return testSkeleton(), nil
		},
		remote:     func(string) skeletonRemote { return &fakeSkeletonRemote{exists: true} },
		viewerPath: func() (string, error) { return "/viewer", nil },
		rootInfo: func(string) ([]string, error) {
			rootInfoCalls++
			if rootInfoCalls == 1 {
				return nil, errors.New("SeaTable unavailable")
			}
			return []string{"root_info", "cell: recovered"}, nil
		},
		launch: func(_ context.Context, _, _, infoPath, _ string, _ int) error {
			gotInfoPath = infoPath
			return nil
		},
	}

	if err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{}, deps); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{}, deps); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if rootInfoCalls != 2 {
		t.Fatalf("rootInfoCalls = %d, want failed cached info to be retried", rootInfoCalls)
	}
	infoData, err := os.ReadFile(gotInfoPath)
	if err != nil {
		t.Fatalf("reading info file: %v", err)
	}
	if !strings.Contains(string(infoData), "cell: recovered") || strings.Contains(string(infoData), "SeaTable unavailable") {
		t.Fatalf("info file = %s, want recovered root info without old error", string(infoData))
	}
}

func TestRunSkeletonViewQueuesUncachedSkeleton(t *testing.T) {
	var out, errOut bytes.Buffer
	var fetched bool
	var uvCalled bool
	err := runSkeletonView(context.Background(), &out, &errOut, "111", skeletonViewOptions{JSONDebug: true}, skeletonViewDeps{
		token: func() string { return "token" },
		lookPathUV: func() (string, error) {
			uvCalled = true
			return "", errors.New(skeleton.MissingUVMessage)
		},
		cacheRoot: func() (string, error) { return t.TempDir(), nil },
		fetch: func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error) {
			fetched = true
			return testSkeleton(), nil
		},
		remote: func(string) skeletonRemote { return &fakeSkeletonRemote{exists: false, estimate: 90} },
	})
	if err != nil {
		t.Fatalf("runSkeletonView: %v", err)
	}
	if fetched {
		t.Fatal("fetch should not run when uncached skeleton is queued without --wait")
	}
	if uvCalled {
		t.Fatal("uv lookup should not run when uncached skeleton is only queued")
	}
	if !strings.Contains(out.String(), `"queued": true`) {
		t.Fatalf("stdout = %s, want queued JSON", out.String())
	}
}

func TestRunSkeletonViewWaitFetchesUncachedSkeleton(t *testing.T) {
	var fetched bool
	remote := &fakeSkeletonRemote{existsSeq: []bool{false, false, true}}
	err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{JSONDebug: true, Wait: true, WaitTimeout: time.Minute}, skeletonViewDeps{
		token:      func() string { return "token" },
		lookPathUV: func() (string, error) { return "/usr/bin/uv", nil },
		cacheRoot:  func() (string, error) { return t.TempDir(), nil },
		fetch: func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error) {
			if remote.queueCalls != 1 {
				t.Fatalf("fetch ran before queueing; queueCalls = %d, want 1", remote.queueCalls)
			}
			fetched = true
			return testSkeleton(), nil
		},
		remote: func(string) skeletonRemote { return remote },
		sleep:  func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatalf("runSkeletonView: %v", err)
	}
	if !fetched {
		t.Fatal("fetch should run with --wait even when server cache is missing")
	}
	if remote.queueCalls != 1 {
		t.Fatalf("queueCalls = %d, want 1", remote.queueCalls)
	}
	if remote.existsCalls != 3 {
		t.Fatalf("existsCalls = %d, want initial check plus two polls", remote.existsCalls)
	}
}

func TestRunSkeletonViewWaitTimesOut(t *testing.T) {
	var fetched bool
	remote := &fakeSkeletonRemote{exists: false}
	err := runSkeletonView(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, "111", skeletonViewOptions{JSONDebug: true, Wait: true, WaitTimeout: time.Second}, skeletonViewDeps{
		token:      func() string { return "token" },
		lookPathUV: func() (string, error) { return "/usr/bin/uv", nil },
		cacheRoot:  func() (string, error) { return t.TempDir(), nil },
		fetch: func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error) {
			fetched = true
			return testSkeleton(), nil
		},
		remote: func(string) skeletonRemote { return remote },
		sleep:  func(context.Context, time.Duration) error { return context.DeadlineExceeded },
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "was not ready before --wait-timeout") {
		t.Fatalf("error = %q, want wait-timeout guidance", err.Error())
	}
	if fetched {
		t.Fatal("fetch should not run after wait timeout")
	}
	if remote.queueCalls != 1 {
		t.Fatalf("queueCalls = %d, want 1", remote.queueCalls)
	}
}

func TestFindSkeletonViewerUsesAbsoluteOverride(t *testing.T) {
	helperPath := filepath.Join(t.TempDir(), "crantcli-skeleton-viewer")
	if err := os.WriteFile(helperPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("writing helper: %v", err)
	}
	t.Setenv(skeletonViewerOverrideEnv, helperPath)

	got, err := findSkeletonViewer()
	if err != nil {
		t.Fatalf("findSkeletonViewer: %v", err)
	}
	if got != helperPath {
		t.Fatalf("helper path = %q, want %q", got, helperPath)
	}
}

func TestFindSkeletonViewerRejectsRelativeOverride(t *testing.T) {
	t.Setenv(skeletonViewerOverrideEnv, "relative-helper")

	_, err := findSkeletonViewer()
	if err == nil {
		t.Fatal("expected relative override error")
	}
	if !strings.Contains(err.Error(), "must be an absolute path") {
		t.Fatalf("error = %q, want absolute path guidance", err.Error())
	}
}

func TestFindSkeletonViewerDoesNotUsePathFallback(t *testing.T) {
	tmpDir := t.TempDir()
	helperPath := filepath.Join(tmpDir, "crantcli-skeleton-viewer")
	if err := os.WriteFile(helperPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("writing helper: %v", err)
	}
	t.Setenv(skeletonViewerOverrideEnv, "")
	t.Setenv("PATH", tmpDir)

	got, err := findSkeletonViewer()
	if err == nil && got == helperPath {
		t.Fatalf("findSkeletonViewer used PATH helper %q", got)
	}
}

func TestSkeletonViewerEnvDropsTokenVars(t *testing.T) {
	env := skeletonViewerEnv([]string{
		"PATH=/bin",
		"DISPLAY=:0",
		"LC_ALL=C",
		"CAVE_TOKEN=secret",
		"CAVE_TOKEN_FILE=/tmp/cave",
		"CRANTTABLE_TOKEN=secret",
		"UNRELATED_SECRET=secret",
	})
	joined := strings.Join(env, "\n")
	for _, want := range []string{"PATH=/bin", "DISPLAY=:0", "LC_ALL=C"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("env = %#v, want %s", env, want)
		}
	}
	for _, secret := range []string{"CAVE_TOKEN=", "CAVE_TOKEN_FILE=", "CRANTTABLE_TOKEN=", "UNRELATED_SECRET="} {
		if strings.Contains(joined, secret) {
			t.Fatalf("env leaked %s in %#v", secret, env)
		}
	}
}
