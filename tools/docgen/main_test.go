package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeMarkdown(t *testing.T) {
	input := "## crantcli example\n\nDescription.\n\n### Examples\n\n```\n  crantcli example --flag\n  ### shell comment\n```\n\n### Options\n\n#### Platform\n\n### SEE ALSO\n"
	want := "# crantcli example\n\nDescription.\n\n## Examples\n\n```bash\n  crantcli example --flag\n  ### shell comment\n```\n\n## Options\n\n### Platform\n\n## See also\n"
	if got := normalizeMarkdown(input); got != want {
		t.Fatalf("normalizeMarkdown() = %q, want %q", got, want)
	}
}

func TestNormalizeMarkdownWithoutExamples(t *testing.T) {
	input := "## crantcli example\n\n### Synopsis\n\nNo examples.\n"
	want := "# crantcli example\n\n## Synopsis\n\nNo examples.\n"
	if got := normalizeMarkdown(input); got != want {
		t.Fatalf("normalizeMarkdown() = %q, want %q", got, want)
	}
}

func TestReplaceDirectory(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "commands")
	staging := filepath.Join(parent, "staging")
	writeTestFile(t, filepath.Join(destination, "old.md"), "old")
	writeTestFile(t, filepath.Join(staging, "new.md"), "new")

	if err := replaceDirectory(staging, destination); err != nil {
		t.Fatalf("replaceDirectory() error = %v", err)
	}
	if got := readTestFile(t, filepath.Join(destination, "new.md")); got != "new" {
		t.Fatalf("new documentation = %q, want %q", got, "new")
	}
	if _, err := os.Stat(filepath.Join(destination, "old.md")); !os.IsNotExist(err) {
		t.Fatalf("old documentation still exists, stat error = %v", err)
	}
}

func TestReplaceDirectoryRestoresDestinationOnFailure(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "commands")
	writeTestFile(t, filepath.Join(destination, "old.md"), "old")

	err := replaceDirectory(filepath.Join(parent, "missing-staging"), destination)
	if err == nil {
		t.Fatal("replaceDirectory() error = nil, want installation failure")
	}
	if got := readTestFile(t, filepath.Join(destination, "old.md")); got != "old" {
		t.Fatalf("restored documentation = %q, want %q", got, "old")
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating test directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading test file: %v", err)
	}
	return string(data)
}
