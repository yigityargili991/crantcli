//go:build linux

package cmd

import (
	"os"

	"crantcli/internal/clipboard"

	"github.com/spf13/cobra"
)

var (
	runClipboardOwner  = clipboard.RunNativeClipboardOwner
	runClipboardReader = clipboard.RunNativeClipboardReader
)

// clipboardOwnerCmd is a private self-exec mode used on Linux. It must remain
// hidden: its stdin/stdout form a protocol with the foreground process, not a
// user-facing command.
var clipboardOwnerCmd = &cobra.Command{
	Use:                clipboard.NativeOwnerCommandName,
	Hidden:             true,
	Args:               cobra.NoArgs,
	DisableFlagParsing: true,
	Run: func(cmd *cobra.Command, args []string) {
		runClipboardOwner(os.Stdin, os.Stdout)
	},
}

var clipboardReaderCmd = &cobra.Command{
	Use:                clipboard.NativeReaderCommandName,
	Hidden:             true,
	Args:               cobra.NoArgs,
	DisableFlagParsing: true,
	Run: func(cmd *cobra.Command, args []string) {
		runClipboardReader(os.Stdout)
	},
}

func init() {
	rootCmd.AddCommand(clipboardOwnerCmd, clipboardReaderCmd)
}
