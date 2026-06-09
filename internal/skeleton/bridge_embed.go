package skeleton

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed bridge/*
var bridgeFiles embed.FS

func EnsureBridgeRuntime(dir string) error {
	for _, name := range []string{"pyproject.toml", "bridge.py"} {
		data, err := bridgeFiles.ReadFile(filepath.Join("bridge", name))
		if err != nil {
			return fmt.Errorf("reading embedded bridge file %s: %w", name, err)
		}
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("creating bridge runtime directory: %w", err)
		}
		if existing, err := os.ReadFile(path); err == nil && string(existing) == string(data) {
			continue
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return fmt.Errorf("writing bridge runtime file %s: %w", name, err)
		}
	}
	return nil
}
