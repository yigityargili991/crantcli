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
	if err := run(*in, *version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(in, version string) error {
	if in == "" || version == "" {
		return fmt.Errorf("usage: modelenc -in MODEL.gguf -version vN")
	}
	client, err := seatable.NewClient()
	if err != nil {
		return err
	}
	m := ask.Model{Version: version}
	key, err := ask.ModelKey(client.BaseID(), m)
	if err != nil {
		return err
	}
	src, err := os.Open(in)
	if err != nil {
		return err
	}
	defer src.Close()
	plainHash := sha256.New()
	out := in + ".enc"
	dst, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	encHash := sha256.New()
	if err := ask.EncryptModel(io.MultiWriter(dst, encHash), io.TeeReader(src, plainHash), key); err != nil {
		dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	info, err := os.Stat(out)
	if err != nil {
		return err
	}
	fmt.Printf("wrote %s\n\n", out)
	fmt.Printf("\tSize:        %d,\n", info.Size())
	fmt.Printf("\tAssetSHA256: %q,\n", hex.EncodeToString(encHash.Sum(nil)))
	fmt.Printf("\tModelSHA256: %q,\n", hex.EncodeToString(plainHash.Sum(nil)))
	fmt.Printf("\tKeyCheck:    %q,\n", ask.KeyCheckFor(key))
	return nil
}
