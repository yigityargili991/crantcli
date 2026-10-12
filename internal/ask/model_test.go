package ask

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func testKey(t *testing.T, base string) []byte {
	t.Helper()
	key, err := ModelKey(base, Model{Version: "v9"})
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestModelEncryptionRoundTrip(t *testing.T) {
	key := testKey(t, "base-uuid")
	for _, n := range []int{0, 1, encChunk - 1, encChunk, encChunk + 1, 3*encChunk + 5} {
		plain := make([]byte, n)
		rand.Read(plain)
		var enc, dec bytes.Buffer
		if err := EncryptModel(&enc, bytes.NewReader(plain), key); err != nil {
			t.Fatalf("encrypt %d bytes: %v", n, err)
		}
		if err := DecryptModel(&dec, bytes.NewReader(enc.Bytes()), key); err != nil {
			t.Fatalf("decrypt %d bytes: %v", n, err)
		}
		if !bytes.Equal(dec.Bytes(), plain) {
			t.Fatalf("round trip of %d bytes changed the data", n)
		}
	}
}

func TestModelDecryptionFailures(t *testing.T) {
	key := testKey(t, "base-uuid")
	plain := make([]byte, 2*encChunk+10)
	rand.Read(plain)
	var enc bytes.Buffer
	if err := EncryptModel(&enc, bytes.NewReader(plain), key); err != nil {
		t.Fatal(err)
	}
	data := enc.Bytes()

	if err := DecryptModel(io.Discard, bytes.NewReader(data), testKey(t, "other-base")); !errors.Is(err, ErrWrongBase) {
		t.Errorf("wrong key: err = %v, want ErrWrongBase", err)
	}
	chunkLen := encChunk + 16
	header := len(encMagic) + encPrefixLen
	truncated := data[:header+2*chunkLen] // drops the final chunk
	if err := DecryptModel(io.Discard, bytes.NewReader(truncated), key); err == nil {
		t.Error("truncated file decrypted")
	}
	damaged := bytes.Clone(data)
	damaged[header+chunkLen+100] ^= 1
	if err := DecryptModel(io.Discard, bytes.NewReader(damaged), key); err == nil || errors.Is(err, ErrWrongBase) {
		t.Errorf("damaged file: err = %v", err)
	}
	if err := DecryptModel(io.Discard, bytes.NewReader([]byte("GGUF....")), key); err == nil {
		t.Error("plain file accepted as encrypted")
	}
}

func hexSHA(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestInstallModel(t *testing.T) {
	const base = "crant-base-uuid"
	m := Model{Version: "v9", File: "raider-135m-v9-q8_0.gguf", Asset: "raider-135m-v9-q8_0.gguf.enc"}
	key, _ := ModelKey(base, m)
	plain := bytes.Repeat([]byte("GGUF model bytes "), 100000)
	var enc bytes.Buffer
	if err := EncryptModel(&enc, bytes.NewReader(plain), key); err != nil {
		t.Fatal(err)
	}
	m.Size = int64(enc.Len())
	m.AssetSHA256 = hexSHA(enc.Bytes())
	m.ModelSHA256 = hexSHA(plain)
	m.KeyCheck = KeyCheckFor(key)

	dir := t.TempDir()
	old := filepath.Join(dir, "raider-135m-v4-q8_0.gguf")
	os.WriteFile(old, []byte("old"), 0o600)
	downloads := 0
	download := func() (io.ReadCloser, error) {
		downloads++
		return io.NopCloser(bytes.NewReader(enc.Bytes())), nil
	}

	if _, err := InstallModel(dir, m, "other-base", download, nil); !errors.Is(err, ErrWrongBase) || downloads != 0 {
		t.Fatalf("wrong base: err = %v, downloads = %d; want ErrWrongBase before downloading", err, downloads)
	}
	var last int64
	path, err := InstallModel(dir, m, base, download, func(done int64) { last = done })
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, plain) {
		t.Error("installed model differs from the original")
	}
	if last != m.Size {
		t.Errorf("progress ended at %d, want %d", last, m.Size)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("old model version was not removed")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("cache dir holds %d files, want only the model", len(entries))
	}

	bad := m
	bad.AssetSHA256 = hexSHA([]byte("something else"))
	if _, err := InstallModel(t.TempDir(), bad, base, download, nil); err == nil {
		t.Error("asset with the wrong checksum was installed")
	}
	unpublished := m
	unpublished.AssetSHA256 = ""
	if _, err := InstallModel(t.TempDir(), unpublished, base, download, nil); err == nil {
		t.Error("model without pinned hashes was installed")
	}
}
