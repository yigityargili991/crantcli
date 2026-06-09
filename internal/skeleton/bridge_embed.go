package skeleton

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

//go:embed bridge/*
var bridgeFiles embed.FS

func EnsureBridgeRuntime(dir string) error {
	for _, name := range []string{"pyproject.toml", "bridge.py"} {
		data, err := bridgeFiles.ReadFile(path.Join("bridge", name))
		if err != nil {
			return fmt.Errorf("reading embedded bridge file %s: %w", name, err)
		}
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("creating bridge runtime directory: %w", err)
		}
		if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
			continue
		}
		if err := writeBridgeRuntimeFile(path, data); err != nil {
			return fmt.Errorf("writing bridge runtime file %s: %w", name, err)
		}
	}
	return nil
}

func writeBridgeRuntimeFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("creating bridge runtime temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing bridge runtime temp file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("setting bridge runtime temp file permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing bridge runtime temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("storing bridge runtime file: %w", err)
	}
	return nil
}
