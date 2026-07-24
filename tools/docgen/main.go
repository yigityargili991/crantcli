package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"crantcli/cmd"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

const outputDirectory = "docs/reference/commands"

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate() error {
	root := cmd.RootCommand()
	root.InitDefaultCompletionCmd()
	root.InitDefaultVersionFlag()
	disableAutoGenTags(root)

	parentDirectory := filepath.Dir(outputDirectory)
	if err := os.MkdirAll(parentDirectory, 0o755); err != nil {
		return fmt.Errorf("creating documentation parent directory: %w", err)
	}

	stagingDirectory, err := os.MkdirTemp(parentDirectory, ".commands-staging-")
	if err != nil {
		return fmt.Errorf("creating documentation staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDirectory)

	if err := doc.GenMarkdownTree(root, stagingDirectory); err != nil {
		return fmt.Errorf("generating command documentation: %w", err)
	}
	if err := normalizeGeneratedMarkdown(stagingDirectory); err != nil {
		return err
	}
	return replaceDirectory(stagingDirectory, outputDirectory)
}

func disableAutoGenTags(command *cobra.Command) {
	command.DisableAutoGenTag = true
	for _, child := range command.Commands() {
		disableAutoGenTags(child)
	}
}

func normalizeGeneratedMarkdown(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("finding generated command documentation: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading generated documentation %q: %w", path, err)
		}
		normalized := normalizeMarkdown(string(data))
		if err := os.WriteFile(path, []byte(normalized), 0o644); err != nil {
			return fmt.Errorf("writing generated documentation %q: %w", path, err)
		}
	}
	return nil
}

func replaceDirectory(stagingDirectory, destinationDirectory string) error {
	parentDirectory := filepath.Dir(destinationDirectory)
	backupDirectory, err := os.MkdirTemp(parentDirectory, ".commands-backup-")
	if err != nil {
		return fmt.Errorf("reserving documentation backup path: %w", err)
	}
	if err := os.Remove(backupDirectory); err != nil {
		return fmt.Errorf("preparing documentation backup path: %w", err)
	}

	hadDestination := true
	if err := os.Rename(destinationDirectory, backupDirectory); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("backing up existing command documentation: %w", err)
		}
		hadDestination = false
	}

	if err := os.Rename(stagingDirectory, destinationDirectory); err != nil {
		if hadDestination {
			if restoreErr := os.Rename(backupDirectory, destinationDirectory); restoreErr != nil {
				return fmt.Errorf("installing generated command documentation: %w (restoring previous documentation: %v)", err, restoreErr)
			}
		}
		return fmt.Errorf("installing generated command documentation: %w", err)
	}

	if hadDestination {
		if err := os.RemoveAll(backupDirectory); err != nil {
			return fmt.Errorf("cleaning previous command documentation: %w", err)
		}
	}
	return nil
}

func normalizeMarkdown(markdown string) string {
	markdown = promoteHeadings(markdown)
	markdown = strings.ReplaceAll(markdown, "\n## SEE ALSO\n", "\n## See also\n")
	return strings.ReplaceAll(markdown, "\n## Examples\n\n```\n", "\n## Examples\n\n```bash\n")
}

func promoteHeadings(markdown string) string {
	lines := strings.Split(markdown, "\n")
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for level := 6; level >= 2; level-- {
			prefix := strings.Repeat("#", level) + " "
			if strings.HasPrefix(line, prefix) {
				lines[i] = line[1:]
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}
