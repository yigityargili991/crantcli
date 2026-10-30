// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

// Command modelenc encrypts a raider model for release. It logs in with the
// configured SeaTable token, derives the model key from the CRANT base ID,
// writes the encrypted asset, and prints the values to pin in
// internal/ask.CurrentModel. It prints no secrets.
//
//	go run ./tools/modelenc -in raider-135m-v4-q8_0.gguf -version v4
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"

	"crantcli/internal/ask"
	"crantcli/internal/seatable"
)

func main() {
	in := flag.String("in", "", "decrypted model (GGUF)")
	version := flag.String("version", "", "model version, such as v4")
	flag.Parse()
	if err := run(*in, *version, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(in, version string, out io.Writer) error {
	if in == "" || version == "" {
		return fmt.Errorf("usage: modelenc -in MODEL.gguf -version vN")
	}
	client, err := seatable.NewClient()
	if err != nil {
		return err
	}
	pin, err := encryptFile(in, version, client.BaseID())
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s\n\n", in+".enc")
	fmt.Fprintf(out, "\tSize:        %d,\n", pin.Size)
	fmt.Fprintf(out, "\tAssetSHA256: %q,\n", pin.AssetSHA256)
	fmt.Fprintf(out, "\tModelSHA256: %q,\n", pin.ModelSHA256)
	fmt.Fprintf(out, "\tKeyCheck:    %q,\n", pin.KeyCheck)
	return nil
}

// encryptFile writes in+".enc", encrypted for baseID, and returns the values
// a crantcli build pins for it.
func encryptFile(in, version, baseID string) (ask.Model, error) {
	m := ask.Model{Version: version}
	key, err := ask.ModelKey(baseID, m)
	if err != nil {
		return m, err
	}
	src, err := os.Open(in)
	if err != nil {
		return m, err
	}
	defer src.Close()
	dst, err := os.OpenFile(in+".enc", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return m, err
	}
	plainHash, encHash := sha256.New(), sha256.New()
	counter := &countingWriter{w: io.MultiWriter(dst, encHash)}
	if err := ask.EncryptModel(counter, io.TeeReader(src, plainHash), key); err != nil {
		dst.Close()
		return m, err
	}
	if err := dst.Close(); err != nil {
		return m, err
	}
	m.Size = counter.n
	m.AssetSHA256 = hex.EncodeToString(encHash.Sum(nil))
	m.ModelSHA256 = hex.EncodeToString(plainHash.Sum(nil))
	m.KeyCheck = ask.KeyCheckFor(key)
	return m, nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	c.n += int64(n)
	return n, err
}
