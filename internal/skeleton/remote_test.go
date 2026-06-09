package skeleton

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type remoteRoundTripFunc func(*http.Request) (*http.Response, error)

func (f remoteRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newRemoteTestClient(handler http.Handler) RemoteClient {
	client := NewRemoteClient("http://skeleton.test", "kronauer_ant", "token")
	client.http = &http.Client{Transport: remoteRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Result(), nil
	})}
	return client
}

func TestRemoteSkeletonExists(t *testing.T) {
	client := newRemoteTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/skeletoncache/api/v1/kronauer_ant/precomputed/skeleton/exists" {
			t.Fatalf("path = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("Authorization = %q", got)
		}
		assertSkeletonCacheBody(t, r)
		fmt.Fprint(w, `true`)
	}))

	exists, err := client.SkeletonExists(context.Background(), "111")
	if err != nil {
		t.Fatalf("SkeletonExists: %v", err)
	}
	if !exists {
		t.Fatal("exists = false, want true")
	}
}

func TestRemoteQueueSkeleton(t *testing.T) {
	client := newRemoteTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/skeletoncache/api/v1/kronauer_ant/bulk/gen_skeletons" {
			t.Fatalf("path = %q", got)
		}
		assertSkeletonCacheBody(t, r)
		fmt.Fprint(w, `75`)
	}))

	estimate, err := client.QueueSkeleton(context.Background(), "111")
	if err != nil {
		t.Fatalf("QueueSkeleton: %v", err)
	}
	if estimate != 75 {
		t.Fatalf("estimate = %v, want 75", estimate)
	}
}

func TestRemoteEscapesDatastackPathSegment(t *testing.T) {
	client := NewRemoteClient("http://skeleton.test", "stack/with space", "token")
	client.http = &http.Client{Transport: remoteRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.URL.EscapedPath(); got != "/skeletoncache/api/v1/stack%2Fwith%20space/precomputed/skeleton/exists" {
			t.Fatalf("escaped path = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`true`)),
			Header:     make(http.Header),
		}, nil
	})}

	if _, err := client.SkeletonExists(context.Background(), "111"); err != nil {
		t.Fatalf("SkeletonExists: %v", err)
	}
}

func assertSkeletonCacheBody(t *testing.T, r *http.Request) {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("reading request body: %v", err)
	}
	var got struct {
		RootIDs         []uint64 `json:"root_ids"`
		SkeletonVersion int      `json:"skeleton_version"`
		VerboseLevel    int      `json:"verbose_level"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decoding request body %q: %v", string(body), err)
	}
	if len(got.RootIDs) != 1 || got.RootIDs[0] != 111 {
		t.Fatalf("root_ids = %#v, want [111]", got.RootIDs)
	}
	if got.SkeletonVersion != 4 || got.VerboseLevel != 0 {
		t.Fatalf("request body = %#v, want skeleton_version=4 verbose_level=0", got)
	}
}
