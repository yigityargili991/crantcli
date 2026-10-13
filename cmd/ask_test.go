package cmd

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"crantcli/internal/ask"
	"crantcli/internal/seatable"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type fakeAskEngine struct {
	out    string
	closed bool
}

func (f *fakeAskEngine) Complete(prompt, grammar string) (string, error) { return f.out, nil }
func (f *fakeAskEngine) Close() error                                    { f.closed = true; return nil }

// setupAskTest points every ask seam at test doubles and returns the cache dir
// and the fake engine. Commands run without a terminal unless a test says so.
func setupAskTest(t *testing.T) (string, *fakeAskEngine) {
	t.Helper()
	cache := t.TempDir()
	engine := &fakeAskEngine{out: " add --cell-type PEN"}
	saved := []func(){}
	save := func(restore func()) { saved = append(saved, restore) }

	prevCache, prevClient, prevQuery, prevRun := askCacheDir, askNewClient, askQueryDistinct, askRunSelf
	prevIn, prevErr, prevLoad, prevBase := askStdinIsTerminal, askStderrIsTerminal, askLoadEngine, askBaseID
	prevToken, prevURL, prevModel := getAPIToken, askModelBaseURL, ask.CurrentModel
	save(func() {
		askCacheDir, askNewClient, askQueryDistinct, askRunSelf = prevCache, prevClient, prevQuery, prevRun
		askStdinIsTerminal, askStderrIsTerminal, askLoadEngine, askBaseID = prevIn, prevErr, prevLoad, prevBase
		getAPIToken, askModelBaseURL, ask.CurrentModel = prevToken, prevURL, prevModel
	})

	askCacheDir = func() (string, error) { return cache, nil }
	askNewClient = func() (*seatable.Client, error) { return &seatable.Client{}, nil }
	askQueryDistinct = func(_ *seatable.Client, field string, _ *seatable.Filters, _ bool) (*seatable.SQLResponse, error) {
		if field == "cell_type" {
			return &seatable.SQLResponse{Results: []map[string]interface{}{{"cell_type": "PEN"}}}, nil
		}
		return &seatable.SQLResponse{}, nil
	}
	askRunSelf = func([]string) error { t.Fatal("command ran unexpectedly"); return nil }
	askStdinIsTerminal = func() bool { return false }
	askStderrIsTerminal = func() bool { return false }
	askLoadEngine = func(string) (askEngine, error) { return engine, nil }
	askBaseID = func() (string, error) { return "test-base", nil }
	getAPIToken = func() string { return "token" }

	t.Cleanup(func() {
		resetAskFlags()
		rootCmd.SetIn(nil)
		for _, restore := range saved {
			restore()
		}
	})
	return cache, engine
}

// resetAskFlags undoes flag values a previous run left behind.
func resetAskFlags() {
	askCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Value.Set(f.DefValue)
		f.Changed = false
	})
}

func executeAsk(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	resetAskFlags()
	return executeRootForTest(t, args...)
}

// modelFile writes a stand-in model file and returns its path.
func modelFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(path, []byte("gguf"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func hexSum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// publishTestModel encrypts a stand-in model for base, serves it like a
// GitHub release, and makes it the current model.
func publishTestModel(t *testing.T, base string) []byte {
	t.Helper()
	m := ask.Model{Version: "v9", File: "raider-135m-v9-q8_0.gguf", Tag: "raider-v9", Asset: "raider-135m-v9-q8_0.gguf.enc"}
	key, err := ask.ModelKey(base, m)
	if err != nil {
		t.Fatal(err)
	}
	plain := bytes.Repeat([]byte("raider "), 1000)
	var enc bytes.Buffer
	if err := ask.EncryptModel(&enc, bytes.NewReader(plain), key); err != nil {
		t.Fatal(err)
	}
	m.Size, m.AssetSHA256, m.ModelSHA256, m.KeyCheck = int64(enc.Len()), hexSum(enc.Bytes()), hexSum(plain), ask.KeyCheckFor(key)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+m.Tag+"/"+m.Asset {
			http.NotFound(w, r)
			return
		}
		w.Write(enc.Bytes())
	}))
	t.Cleanup(srv.Close)
	askModelBaseURL = srv.URL + "/"
	ask.CurrentModel = m
	return plain
}

func TestAskPrintsSuggestion(t *testing.T) {
	_, engine := setupAskTest(t)
	out, errOut, err := executeAsk(t, "ask", "--print", "--model", modelFile(t), "show", "PEN", "neurons")
	if err != nil {
		t.Fatal(err)
	}
	if out != "crantcli add --cell-type PEN\n" {
		t.Errorf("stdout = %q", out)
	}
	if !strings.Contains(errOut, askExperimentalWarning) {
		t.Errorf("stderr lacks the experimental warning: %q", errOut)
	}
	if !engine.closed {
		t.Error("engine was not closed")
	}
}

func TestAskRunGate(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		terminal bool
		input    string
		wantRun  bool
		wantErr  string
	}{
		{name: "no terminal", args: nil, wantErr: "Not running it"},
		{name: "yes flag", args: []string{"--yes"}, wantRun: true},
		{name: "confirmed", terminal: true, input: "y\n", wantRun: true},
		{name: "declined", terminal: true, input: "n\n"},
		{name: "enter declines", terminal: true, input: "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupAskTest(t)
			askStdinIsTerminal = func() bool { return tt.terminal }
			var ran []string
			askRunSelf = func(argv []string) error { ran = argv; return nil }
			rootCmd.SetIn(strings.NewReader(tt.input))
			args := append([]string{"ask", "--model", modelFile(t)}, tt.args...)
			_, errOut, err := executeAsk(t, append(args, "show PEN")...)
			if err != nil {
				t.Fatal(err)
			}
			if got := ran != nil; got != tt.wantRun {
				t.Fatalf("ran = %v, want run %v", ran, tt.wantRun)
			}
			if tt.wantRun && !reflect.DeepEqual(ran, []string{"add", "--cell-type", "PEN"}) {
				t.Errorf("ran %q", ran)
			}
			if !strings.Contains(errOut, tt.wantErr) {
				t.Errorf("stderr = %q, want %q", errOut, tt.wantErr)
			}
		})
	}
}

func TestAskModelPathErrors(t *testing.T) {
	setupAskTest(t)
	if _, _, err := executeAsk(t, "ask", "--model", "/does/not/exist.gguf", "show PEN"); err == nil || !strings.Contains(err.Error(), "no model at") {
		t.Errorf("missing --model file: err = %v", err)
	}
	t.Setenv("CRANTCLI_ASK_MODEL", modelFile(t))
	if _, _, err := executeAsk(t, "ask", "--print", "show PEN"); err != nil {
		t.Errorf("CRANTCLI_ASK_MODEL: %v", err)
	}
	t.Setenv("CRANTCLI_ASK_MODEL", "")
	if _, _, err := executeAsk(t, "ask", "show PEN"); err == nil || !strings.Contains(err.Error(), "ask --download") {
		t.Errorf("missing model without a terminal: err = %v", err)
	}
	if _, _, err := executeAsk(t, "ask"); err == nil {
		t.Error("ask without a request should fail")
	}
}

func TestAskDownload(t *testing.T) {
	cache, _ := setupAskTest(t)
	plain := publishTestModel(t, "test-base")
	_, errOut, err := executeAsk(t, "ask", "--download")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "Installed the raider model v9.") {
		t.Errorf("stderr = %q", errOut)
	}
	got, err := os.ReadFile(filepath.Join(cache, "crantcli", "ask", ask.CurrentModel.File))
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("installed model: %v", err)
	}
	_, errOut, err = executeAsk(t, "ask", "--download")
	if err != nil || !strings.Contains(errOut, "already installed") {
		t.Errorf("second download: err = %v, stderr = %q", err, errOut)
	}
}

func TestAskFirstRunOffersDownload(t *testing.T) {
	setupAskTest(t)
	publishTestModel(t, "test-base")
	askStdinIsTerminal = func() bool { return true }
	askStderrIsTerminal = func() bool { return true }
	rootCmd.SetIn(strings.NewReader("\n"))
	out, errOut, err := executeAsk(t, "ask", "show PEN")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{askModelIntro, "Download the raider model v9", "Downloading the raider model: 100%", "Installed"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr lacks %q: %q", want, errOut)
		}
	}
	if out != "crantcli add --cell-type PEN\n" {
		t.Errorf("stdout = %q", out)
	}
}

func TestAskDownloadFailures(t *testing.T) {
	setupAskTest(t)
	publishTestModel(t, "test-base")
	askStdinIsTerminal = func() bool { return true }
	rootCmd.SetIn(strings.NewReader("n\n"))
	if out, _, err := executeAsk(t, "ask", "show PEN"); err != nil || out != "" {
		t.Errorf("declined download: out = %q, err = %v", out, err)
	}

	askBaseID = func() (string, error) { return "another-base", nil }
	if _, _, err := executeAsk(t, "ask", "--download"); !errors.Is(err, ask.ErrWrongBase) {
		t.Errorf("wrong base: err = %v", err)
	}
	askBaseID = func() (string, error) { return "", errors.New("no token") }
	if _, _, err := executeAsk(t, "ask", "--download"); err == nil || !strings.Contains(err.Error(), "no token") {
		t.Errorf("login failure: err = %v", err)
	}
	askBaseID = func() (string, error) { return "test-base", nil }
	ask.CurrentModel.Asset = "missing.enc"
	if _, _, err := executeAsk(t, "ask", "--download"); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("missing asset: err = %v", err)
	}
}

func TestOfferAskModel(t *testing.T) {
	cache, _ := setupAskTest(t)
	publishTestModel(t, "test-base")
	run := func(terminal bool, input string) string {
		askStdinIsTerminal = func() bool { return terminal }
		var errOut bytes.Buffer
		c := &cobra.Command{}
		c.SetErr(&errOut)
		c.SetIn(strings.NewReader(input))
		if err := offerAskModel(c); err != nil {
			t.Fatal(err)
		}
		return errOut.String()
	}
	if got := run(false, ""); got != "" {
		t.Errorf("without a terminal: %q", got)
	}
	if got := run(true, "n\n"); !strings.Contains(got, askModelIntro) || !strings.Contains(got, "Skipped") {
		t.Errorf("declined: %q", got)
	}
	askBaseID = func() (string, error) { return "another-base", nil }
	if got := run(true, "y\n"); !strings.Contains(got, "Could not install the raider model") {
		t.Errorf("failed install: %q", got)
	}
	askBaseID = func() (string, error) { return "test-base", nil }
	if got := run(true, "y\n"); !strings.Contains(got, "Installed") {
		t.Errorf("install: %q", got)
	}
	if _, err := os.Stat(filepath.Join(cache, "crantcli", "ask", ask.CurrentModel.File)); err != nil {
		t.Fatal(err)
	}
	if got := run(true, ""); got != "" {
		t.Errorf("already installed: %q", got)
	}
}

func TestFetchAskVocab(t *testing.T) {
	setupAskTest(t)
	var fields []string
	askQueryDistinct = func(_ *seatable.Client, field string, _ *seatable.Filters, withCount bool) (*seatable.SQLResponse, error) {
		if !withCount {
			t.Error("values should be fetched with counts, most neurons first")
		}
		fields = append(fields, field)
		return &seatable.SQLResponse{Results: []map[string]interface{}{{field: "A"}, {field: nil}, {field: ""}, {field: 7.0}}}, nil
	}
	values, err := fetchAskVocab()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fields, ask.VocabFields) {
		t.Errorf("queried %v", fields)
	}
	if got := values["cell_type"]; !reflect.DeepEqual(got, []string{"A", "7"}) {
		t.Errorf("cell_type values = %q", got)
	}

	askQueryDistinct = func(*seatable.Client, string, *seatable.Filters, bool) (*seatable.SQLResponse, error) {
		return nil, errors.New("offline")
	}
	if _, err := fetchAskVocab(); err == nil {
		t.Error("expected the query error")
	}
	askNewClient = func() (*seatable.Client, error) { return nil, errors.New("no token") }
	if _, err := fetchAskVocab(); err == nil {
		t.Error("expected the client error")
	}
}

func TestAskConfirm(t *testing.T) {
	tests := []struct {
		input      string
		defaultYes bool
		want       bool
	}{
		{"y\n", false, true},
		{"YES\n", false, true},
		{"\n", true, true},
		{"\n", false, false},
		{"", true, true},
		{"n\n", true, false},
		{"maybe\n", true, false},
	}
	for _, tt := range tests {
		var errOut bytes.Buffer
		c := &cobra.Command{}
		c.SetErr(&errOut)
		if got := askConfirm(c, bufio.NewReader(strings.NewReader(tt.input)), "? ", tt.defaultYes); got != tt.want {
			t.Errorf("askConfirm(%q, default %v) = %v, want %v", tt.input, tt.defaultYes, got, tt.want)
		}
		if errOut.String() != "? " {
			t.Errorf("prompt = %q", errOut.String())
		}
	}
}

func TestMegabytes(t *testing.T) {
	for n, want := range map[int64]int64{0: 0, 499_999: 0, 500_000: 1, 144_813_180: 145} {
		if got := megabytes(n); got != want {
			t.Errorf("megabytes(%d) = %d, want %d", n, got, want)
		}
	}
}

// TestRunSelfRunsTheBinary runs the test binary itself as the "crantcli"
// child; inside the child the test returns at once.
func TestRunSelfRunsTheBinary(t *testing.T) {
	if os.Getenv("CRANTCLI_RUNSELF_CHILD") == "1" {
		return
	}
	t.Setenv("CRANTCLI_RUNSELF_CHILD", "1")
	if err := runSelf([]string{"-test.run=^TestRunSelfRunsTheBinary$"}); err != nil {
		t.Fatal(err)
	}
}
