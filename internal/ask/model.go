// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package ask

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Model is one published version of the raider model. The release asset is
// encrypted with a key derived from the CRANT base ID, which every valid
// SeaTable login for the base returns, so only CRANT users can read it. The
// hashes pin the exact files this binary accepts.
type Model struct {
	Version     string // "v4"
	File        string // decrypted model file name
	Tag         string // GitHub release that hosts the encrypted file
	Asset       string // encrypted release asset name
	Size        int64  // encrypted asset size in bytes
	AssetSHA256 string
	ModelSHA256 string
	KeyCheck    string // shows a wrong base before the download starts
}

// CurrentModel is the model version this binary uses.
var CurrentModel = Model{
	Version:     "v4",
	File:        "raider-135m-v4-q8_0.gguf",
	Tag:         "raider-v4",
	Asset:       "raider-135m-v4-q8_0.gguf.enc",
	Size:        144813180,
	AssetSHA256: "cad5d9939190425066a4739fad446fa1fb2b786715f06fd1bb1c78be7a087b4b",
	ModelSHA256: "350c0eb2f4fb808c6c008f0b0757f0b73acc10f173bd418de971457a9ddb045a",
	KeyCheck:    "9d117adeb1fbd516",
}

// ErrWrongBase means the SeaTable login is not for the base the model was
// encrypted for.
var ErrWrongBase = errors.New("the raider model cannot be unlocked with this SeaTable token; it needs a token for the CRANT base")

const (
	encMagic     = "RAIDER1\n"
	encChunk     = 1 << 20
	encPrefixLen = 4
)

// ModelKey derives the model version's key from the CRANT base ID.
func ModelKey(baseID string, m Model) ([]byte, error) {
	if baseID == "" {
		return nil, errors.New("empty base ID")
	}
	return hkdf.Key(sha256.New, []byte(baseID), []byte("crantcli raider model"), "raider-"+m.Version, 32)
}

// KeyCheckFor returns a short, public fingerprint of a model key.
func KeyCheckFor(key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("key check"))
	return hex.EncodeToString(mac.Sum(nil)[:8])
}

func chunkCipher(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func chunkNonce(prefix []byte, i uint64) []byte {
	nonce := make([]byte, 12)
	copy(nonce, prefix)
	binary.BigEndian.PutUint64(nonce[encPrefixLen:], i)
	return nonce
}

func chunkAAD(final bool) []byte {
	if final {
		return []byte(encMagic + "final")
	}
	return []byte(encMagic + "chunk")
}

// EncryptModel encrypts src to dst in 1 MiB AES-GCM chunks. The last chunk is
// marked, so a truncated file does not decrypt.
func EncryptModel(dst io.Writer, src io.Reader, key []byte) error {
	aead, err := chunkCipher(key)
	if err != nil {
		return err
	}
	prefix := make([]byte, encPrefixLen)
	if _, err := rand.Read(prefix); err != nil {
		return err
	}
	if _, err := dst.Write(append([]byte(encMagic), prefix...)); err != nil {
		return err
	}
	r := bufio.NewReaderSize(src, encChunk)
	buf := make([]byte, encChunk)
	for i := uint64(0); ; i++ {
		n, err := io.ReadFull(r, buf)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return err
		}
		_, peekErr := r.Peek(1)
		final := peekErr == io.EOF
		if _, err := dst.Write(aead.Seal(nil, chunkNonce(prefix, i), buf[:n], chunkAAD(final))); err != nil {
			return err
		}
		if final {
			return nil
		}
	}
}

// DecryptModel reverses EncryptModel. It fails with ErrWrongBase when the key
// does not open the first chunk.
func DecryptModel(dst io.Writer, src io.Reader, key []byte) error {
	aead, err := chunkCipher(key)
	if err != nil {
		return err
	}
	header := make([]byte, len(encMagic)+encPrefixLen)
	if _, err := io.ReadFull(src, header); err != nil || string(header[:len(encMagic)]) != encMagic {
		return errors.New("not an encrypted raider model")
	}
	prefix := header[len(encMagic):]
	r := bufio.NewReaderSize(src, encChunk+aead.Overhead())
	buf := make([]byte, encChunk+aead.Overhead())
	for i := uint64(0); ; i++ {
		n, err := io.ReadFull(r, buf)
		if err != nil && err != io.ErrUnexpectedEOF {
			return errors.New("the encrypted model is truncated")
		}
		_, peekErr := r.Peek(1)
		final := peekErr == io.EOF
		plain, err := aead.Open(nil, chunkNonce(prefix, i), buf[:n], chunkAAD(final))
		if err != nil {
			if i == 0 {
				return ErrWrongBase
			}
			return errors.New("the encrypted model is damaged")
		}
		if _, err := dst.Write(plain); err != nil {
			return err
		}
		if final {
			return nil
		}
	}
}

// ModelPath is where the decrypted model of m lives under dir.
func ModelPath(dir string, m Model) string {
	return filepath.Join(dir, m.File)
}

// InstallModel downloads the encrypted model of m, checks its hash, decrypts
// it with the key for baseID, checks the result, and moves it into dir. Older
// raider model files in dir are removed. download returns the asset body.
func InstallModel(dir string, m Model, baseID string, download func() (io.ReadCloser, error), progress func(done int64)) (string, error) {
	if m.AssetSHA256 == "" || m.ModelSHA256 == "" {
		return "", errors.New("this build has no published raider model")
	}
	key, err := ModelKey(baseID, m)
	if err != nil {
		return "", err
	}
	if m.KeyCheck != "" && KeyCheckFor(key) != m.KeyCheck {
		return "", ErrWrongBase
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating ask cache directory: %w", err)
	}

	encPath := filepath.Join(dir, m.Asset+".part")
	defer os.Remove(encPath)
	body, err := download()
	if err != nil {
		return "", fmt.Errorf("downloading the raider model: %w", err)
	}
	if err := writeHashed(encPath, &progressReader{r: body, progress: progress}, m.AssetSHA256); err != nil {
		body.Close()
		return "", fmt.Errorf("downloading the raider model: %w", err)
	}
	body.Close()

	enc, err := os.Open(encPath)
	if err != nil {
		return "", err
	}
	defer enc.Close()
	plainPath := ModelPath(dir, m) + ".part"
	defer os.Remove(plainPath)
	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(DecryptModel(pw, enc, key)) }()
	if err := writeHashed(plainPath, pr, m.ModelSHA256); err != nil {
		if errors.Is(err, ErrWrongBase) {
			return "", ErrWrongBase
		}
		return "", fmt.Errorf("unlocking the raider model: %w", err)
	}
	path := ModelPath(dir, m)
	if err := os.Rename(plainPath, path); err != nil {
		return "", err
	}
	removeOldModels(dir, m)
	return path, nil
}

// writeHashed copies r to path and fails unless the copy has the wanted SHA-256.
func writeHashed(path string, r io.Reader, want string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), r); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("checksum mismatch (got %s, want %s)", got, want)
	}
	return nil
}

func removeOldModels(dir string, keep Model) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "raider-") && name != keep.File && !e.IsDir() {
			os.Remove(filepath.Join(dir, name))
		}
	}
}

type progressReader struct {
	r        io.Reader
	done     int64
	progress func(done int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.progress != nil && n > 0 {
		p.progress(p.done)
	}
	return n, err
}
