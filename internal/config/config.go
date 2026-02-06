package config

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

const (
	SeaTableServer    = "https://cloud.seatable.io"
	SeaTableWorkspace = "62919"
	SeaTableBase      = "CRANTb"
	SeaTableTable     = "CRANTb_meta"

	NglViewer      = "https://spelunker.cave-explorer.org"
	NglStateServer = "https://global.daf-apis.com/nglstate"

	SegmentationSource = "graphene://middleauth+https://data.proofreading.zetta.ai/segmentation/table/kronauer_ant_x1"
	ImageSource        = "precomputed://gs://dkronauer-ant-001-alignment-final/aligned"
	MeshSource         = "precomputed://gs://dkronauer-ant-001-alignment-final/tissue_mesh/mesh#type=mesh"
)

func credentialFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".crant_type_look", "credentials")
}

// ReadStoredToken reads the base64-encoded token from ~/.crant_type_look/credentials.
// Returns empty string if the file doesn't exist or can't be read.
func ReadStoredToken() string {
	path := credentialFilePath()
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return ""
	}
	return string(decoded)
}

// StoreToken base64-encodes the token and writes it to ~/.crant_type_look/credentials.
func StoreToken(token string) error {
	path := credentialFilePath()
	if path == "" {
		return fmt.Errorf("could not determine home directory")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(token))
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0600); err != nil {
		return fmt.Errorf("writing credentials file: %w", err)
	}
	return nil
}

// GetAPIToken returns the SeaTable API token, checking the stored credential file
// first, then falling back to the CRANTTABLE_TOKEN environment variable.
func GetAPIToken() string {
	if token := ReadStoredToken(); token != "" {
		return token
	}
	return os.Getenv("CRANTTABLE_TOKEN")
}

// RunSetupPrompt interactively prompts the user for their SeaTable token and stores it.
// Returns an error if stdin is not a terminal.
func RunSetupPrompt() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("no SeaTable token configured and stdin is not a terminal; set CRANTTABLE_TOKEN or run 'crant_type_look setup'")
	}

	fmt.Println("Let's get set up yeah?")
	fmt.Println("Please copy your SeaTable token here to use crant TypeLook:")
	fmt.Print("> ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return fmt.Errorf("failed to read input")
	}
	token := strings.TrimSpace(scanner.Text())

	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	if err := StoreToken(token); err != nil {
		return err
	}

	fmt.Println("Token saved! You're all set.")
	return nil
}
