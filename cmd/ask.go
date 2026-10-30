// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"crantcli/internal/ask"
	"crantcli/internal/httpx"
	"crantcli/internal/seatable"
	"crantcli/internal/textout"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const askVocabMaxAge = 24 * time.Hour

const askExperimentalWarning = "Warning: crantcli ask is experimental and can suggest the wrong command. Check it before running it."

// askDownloadClient allows for slow connections: the model is about 145 MB.
var askDownloadClient = &http.Client{Timeout: 30 * time.Minute}

// askModelBaseURL is where model releases live; tests point it elsewhere.
var askModelBaseURL = updateReleaseDownloadURL

// Seams for tests.
var (
	askCacheDir         = os.UserCacheDir
	askNewClient        = seatable.NewClient
	askQueryDistinct    = seatable.QueryDistinct
	askRunSelf          = runSelf
	askStdinIsTerminal  = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
	askStderrIsTerminal = func() bool { return term.IsTerminal(int(os.Stderr.Fd())) }
	askLoadEngine       = func(path string) (askEngine, error) {
		e, err := ask.LoadEngine(path)
		if err != nil {
			return nil, err
		}
		return e, nil
	}
	askBaseID = func() (string, error) {
		client, err := askNewClient()
		if err != nil {
			return "", err
		}
		return client.BaseID(), nil
	}
)

// askModelDir is where the raider model and its vocabulary live under the
// user cache directory.
func askModelDir(cacheRoot string) string {
	return filepath.Join(cacheRoot, "crantcli", "ask")
}

// askEngine is the loaded model.
type askEngine interface {
	ask.Completer
	Close() error
}

var askCmd = &cobra.Command{
	Use:   "ask <request>",
	Short: "Turn a plain-English request into a crantcli command (experimental)",
	Long: `Turn a plain-English request into a crantcli command with raider, a small
model we fine-tuned for crantcli (from SmolLM2-135M-Instruct) that runs on
your computer.

ask is experimental: the model can suggest the wrong command, flags, or
values, so check the suggestion before running it.

ask prints the suggested command and asks before running it. Table values
mentioned in the request are looked up in the CRANT table (cached for a day),
and the model can only write flags and values the request gives evidence for.

The first run offers to download the raider model (about 145 MB) into the user
cache directory. The download is encrypted and only unlocks with a SeaTable
token for the CRANT base. Use --download to install or update it without a
request, or --model / CRANTCLI_ASK_MODEL to use another model file.`,
	Example: `  crantcli ask "show the PEN neurons colored by proofreading status and open them"
  crantcli ask --print "which column is 576460752669867599 in"
  crantcli ask -y "edit history of 576460752669867599 as json"
  crantcli ask --download`,
	Annotations: map[string]string{"requiresToken": "true"},
	Args: func(cmd *cobra.Command, args []string) error {
		if download, _ := cmd.Flags().GetBool("download"); download {
			return nil
		}
		return cobra.MinimumNArgs(1)(cmd, args)
	},
}

func init() {
	var (
		askYes      bool
		askPrint    bool
		askRefresh  bool
		askDownload bool
		askModel    string
	)

	askCmd.Flags().BoolVarP(&askYes, "yes", "y", false, "Run the suggested command without asking")
	askCmd.Flags().BoolVar(&askPrint, "print", false, "Only print the suggested command")
	askCmd.Flags().BoolVar(&askRefresh, "refresh", false, "Fetch table values again instead of using the cache")
	askCmd.Flags().BoolVar(&askDownload, "download", false, "Install the raider model without asking")
	askCmd.Flags().StringVar(&askModel, "model", "", "Path to another model file (GGUF)")
	askCmd.ValidArgsFunction = noFileCompletion

	askCmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Warn on stderr so --print output stays a clean command line.
		fmt.Fprintln(cmd.ErrOrStderr(), askExperimentalWarning)
		in := bufio.NewReader(cmd.InOrStdin())
		cacheDir, err := askCacheDir()
		if err != nil {
			return fmt.Errorf("finding the cache directory: %w", err)
		}
		dir := askModelDir(cacheDir)

		modelPath, err := askModelPath(cmd, in, dir, askModel, askDownload)
		if err != nil || modelPath == "" || len(args) == 0 {
			return err
		}

		maxAge := askVocabMaxAge
		if askRefresh {
			maxAge = 0
		}
		vocab, err := ask.LoadVocab(filepath.Join(dir, "vocab.json"), maxAge, fetchAskVocab)
		if err != nil {
			return err
		}

		engine, err := askLoadEngine(modelPath)
		if err != nil {
			return err
		}
		argv, err := ask.Suggest(engine, ask.BuildSpec(cmd.Root(), "ask"), vocab, strings.Join(args, " "))
		engine.Close()
		if err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), textout.Sanitize("crantcli "+ask.QuoteArgv(argv)))
		if askPrint {
			return nil
		}
		if !askYes {
			if !askStdinIsTerminal() {
				fmt.Fprintln(cmd.ErrOrStderr(), "Not running it: stdin is not a terminal. Pass --yes to run it.")
				return nil
			}
			if !askConfirm(cmd, in, "Run it? [y/N] ", false) {
				return nil
			}
		}
		return askRunSelf(argv)
	}

	rootCmd.AddCommand(askCmd)
}

// askModelPath returns the model file to load. An explicit --model or
// CRANTCLI_ASK_MODEL wins; otherwise the raider model in dir is used, and
// installed after asking (or right away with --download) when missing. An
// empty path with a nil error means the user declined the download.
func askModelPath(cmd *cobra.Command, in *bufio.Reader, dir, flagPath string, download bool) (string, error) {
	path := flagPath
	if path == "" {
		path = os.Getenv("CRANTCLI_ASK_MODEL")
	}
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("no model at %s", path)
		}
		return path, nil
	}

	m := ask.CurrentModel
	path = ask.ModelPath(dir, m)
	if _, err := os.Stat(path); err == nil {
		if download {
			fmt.Fprintf(cmd.ErrOrStderr(), "The raider model %s is already installed.\n", m.Version)
		}
		return path, nil
	}
	if !download {
		if !askStdinIsTerminal() {
			return "", fmt.Errorf("the raider model is not installed; run 'crantcli ask --download' to install it")
		}
		fmt.Fprintln(cmd.ErrOrStderr(), askModelIntro)
		if !askConfirm(cmd, in, askDownloadPrompt(m), true) {
			return "", nil
		}
	}
	return installAskModel(cmd, dir, m)
}

// askModelIntro explains in one line what the model download is for.
const askModelIntro = "crantcli ask (experimental) turns a plain-English request into a crantcli command, using raider, a small model we fine-tuned for crantcli that runs on your computer."

func askDownloadPrompt(m ask.Model) string {
	return fmt.Sprintf("Download the raider model %s (%d MB) now? [Y/n] ", m.Version, megabytes(m.Size))
}

// offerAskModel is the last setup step: it explains crantcli ask and offers
// to download its model, unless the model is already installed. A failed
// download does not fail setup.
func offerAskModel(cmd *cobra.Command) error {
	if !askStdinIsTerminal() {
		return nil
	}
	cacheDir, err := askCacheDir()
	if err != nil {
		return nil
	}
	dir := askModelDir(cacheDir)
	m := ask.CurrentModel
	if _, err := os.Stat(ask.ModelPath(dir, m)); err == nil {
		return nil
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "\n"+askModelIntro)
	if !askConfirm(cmd, bufio.NewReader(cmd.InOrStdin()), askDownloadPrompt(m), true) {
		fmt.Fprintln(cmd.ErrOrStderr(), "Skipped. 'crantcli ask --download' installs it later.")
		return nil
	}
	if _, err := installAskModel(cmd, dir, m); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Could not install the raider model: %v\nRun 'crantcli ask --download' to try again.\n", err)
	}
	return nil
}

// installAskModel downloads and unlocks model m into dir with the configured
// SeaTable token, showing progress on a terminal.
func installAskModel(cmd *cobra.Command, dir string, m ask.Model) (string, error) {
	baseID, err := askBaseID()
	if err != nil {
		return "", err
	}
	stderr := cmd.ErrOrStderr()
	showProgress := askStderrIsTerminal()
	path, err := ask.InstallModel(dir, m, baseID, func() (io.ReadCloser, error) {
		return downloadAskModel(m)
	}, func(done int64) {
		if showProgress && m.Size > 0 {
			fmt.Fprintf(stderr, "\rDownloading the raider model: %3d%% of %d MB", done*100/m.Size, megabytes(m.Size))
		}
	})
	if showProgress {
		fmt.Fprintln(stderr)
	}
	if err != nil {
		return "", err
	}
	fmt.Fprintf(stderr, "Installed the raider model %s.\n", m.Version)
	return path, nil
}

func megabytes(n int64) int64 { return (n + 500_000) / 1_000_000 }

func askConfirm(cmd *cobra.Command, in *bufio.Reader, prompt string, defaultYes bool) bool {
	fmt.Fprint(cmd.ErrOrStderr(), prompt)
	answer, _ := in.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	case "":
		return defaultYes
	}
	return false
}

// downloadAskModel fetches the encrypted model asset from its GitHub release.
func downloadAskModel(m ask.Model) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, askModelBaseURL+m.Tag+"/"+m.Asset, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "crantcli-ask")
	resp, err := httpx.Do(askDownloadClient, req)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// fetchAskVocab reads the distinct values of the fields a request can name,
// most neurons first.
func fetchAskVocab() (map[string][]string, error) {
	client, err := askNewClient()
	if err != nil {
		return nil, err
	}
	values := map[string][]string{}
	for _, field := range ask.VocabFields {
		resp, err := askQueryDistinct(client, field, &seatable.Filters{}, true)
		if err != nil {
			return nil, err
		}
		for _, row := range resp.Results {
			if val := row[field]; val != nil && val != "" {
				values[field] = append(values[field], fmt.Sprint(val))
			}
		}
	}
	return values, nil
}

// runSelf runs this crantcli binary with argv and exits with its status, so
// the suggested command behaves exactly as if it had been typed.
func runSelf(argv []string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding the crantcli binary: %w", err)
	}
	c := exec.Command(exe, argv...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}
