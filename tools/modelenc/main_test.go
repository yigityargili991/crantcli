// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"crantcli/internal/ask"
)

func TestEncryptFileProducesTheInstalledModel(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "raider-135m-v9-q8_0.gguf")
	plain := bytes.Repeat([]byte("model "), 300000)
	if err := os.WriteFile(in, plain, 0o600); err != nil {
		t.Fatal(err)
	}
	pin, err := encryptFile(in, "v9", "base-uuid")
	if err != nil {
		t.Fatal(err)
	}
	enc, err := os.ReadFile(in + ".enc")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(enc)
	if pin.Size != int64(len(enc)) || pin.AssetSHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("pin %+v does not describe the written asset (%d bytes)", pin, len(enc))
	}

	pin.File, pin.Asset = "raider-135m-v9-q8_0.gguf", "raider-135m-v9-q8_0.gguf.enc"
	installed, err := ask.InstallModel(t.TempDir(), pin, "base-uuid", func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(enc)), nil
	}, nil)
	if err != nil {
		t.Fatalf("crantcli would not accept the encrypted model: %v", err)
	}
	got, _ := os.ReadFile(installed)
	if !bytes.Equal(got, plain) {
		t.Error("installed model differs from the input")
	}
}

func TestEncryptFileErrors(t *testing.T) {
	if _, err := encryptFile(filepath.Join(t.TempDir(), "missing.gguf"), "v9", "base"); err == nil {
		t.Error("expected an error for a missing input")
	}
	in := filepath.Join(t.TempDir(), "m.gguf")
	os.WriteFile(in, []byte("x"), 0o600)
	if _, err := encryptFile(in, "v9", ""); err == nil {
		t.Error("expected an error for an empty base ID")
	}
	if err := run("", "", io.Discard); err == nil {
		t.Error("expected a usage error")
	}
}
