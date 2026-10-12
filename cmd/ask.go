//go:build ask

package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"crantcli/internal/ask"
	"crantcli/internal/seatable"
	"crantcli/internal/textout"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const askVocabMaxAge = 24 * time.Hour

var askCmd = &cobra.Command{
	Use:   "ask <request>",
	Short: "Turn a plain-English request into a crantcli command",
	Long: `Turn a plain-English request into a crantcli command with a small local model.

ask prints the suggested command and asks before running it. Table values
mentioned in the request are looked up in the CRANT table (cached for a day),
and the model can only write flags and values the request gives evidence for.

The model file is read from --model, then CRANTCLI_ASK_MODEL, then
<user cache dir>/crantcli/ask/model.gguf.`,
	Example: `  crantcli ask "show the PEN neurons colored by proofreading status and open them"
  crantcli ask --print "which column is 576460752669867599 in"
  crantcli ask -y "edit history of 576460752669867599 as json"`,
	Annotations: map[string]string{"requiresToken": "true"},
	Args:        cobra.MinimumNArgs(1),
}

func init() {
	var (
		askYes     bool
		askPrint   bool
		askRefresh bool
		askModel   string
	)

	askCmd.Flags().BoolVarP(&askYes, "yes", "y", false, "Run the suggested command without asking")
	askCmd.Flags().BoolVar(&askPrint, "print", false, "Only print the suggested command")
	askCmd.Flags().BoolVar(&askRefresh, "refresh", false, "Fetch table values again instead of using the cache")
	askCmd.Flags().StringVar(&askModel, "model", "", "Path to the ask model (GGUF)")
	askCmd.ValidArgsFunction = noFileCompletion

	askCmd.RunE = func(cmd *cobra.Command, args []string) error {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return fmt.Errorf("finding the cache directory: %w", err)
		}
		dir := filepath.Join(cacheDir, "crantcli", "ask")
		modelPath := askModel
		if modelPath == "" {
			modelPath = os.Getenv("CRANTCLI_ASK_MODEL")
		}
		if modelPath == "" {
			modelPath = filepath.Join(dir, "model.gguf")
		}
		if _, err := os.Stat(modelPath); err != nil {
			return fmt.Errorf("no ask model at %s; pass --model or set CRANTCLI_ASK_MODEL", modelPath)
		}

		maxAge := askVocabMaxAge
		if askRefresh {
			maxAge = 0
		}
		vocab, err := ask.LoadVocab(filepath.Join(dir, "vocab.json"), maxAge, fetchAskVocab)
		if err != nil {
			return err
		}

		engine, err := ask.LoadEngine(modelPath)
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
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				fmt.Fprintln(cmd.ErrOrStderr(), "Not running it: stdin is not a terminal. Pass --yes to run it.")
				return nil
			}
			fmt.Fprint(cmd.ErrOrStderr(), "Run it? [y/N] ")
			answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
			if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
				return nil
			}
		}
		return runSelf(argv)
	}

	rootCmd.AddCommand(askCmd)
}

// fetchAskVocab reads the distinct values of the fields a request can name,
// most neurons first.
func fetchAskVocab() (map[string][]string, error) {
	client, err := seatable.NewClient()
	if err != nil {
		return nil, err
	}
	values := map[string][]string{}
	for _, field := range ask.VocabFields {
		resp, err := seatable.QueryDistinct(client, field, &seatable.Filters{}, true)
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
