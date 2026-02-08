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
// Returns empty string if the file doesn't exist or can't be read. TODO: add a log here
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

func StoreToken(token string) error {
	path := credentialFilePath()
	if path == "" {
		return fmt.Errorf("could not determine home directory")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(token))
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing credentials file: %w", err)
	}
	return nil
}

func readTokenFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// GetAPIToken retrieves the SeaTable API token from one of several sources.
// It checks sources in the following precedence order:
//  1. Stored credentials from ~/.crant_type_look/credentials (via ReadStoredToken)
//  2. CRANTTABLE_TOKEN environment variable
//  3. CRANTTABLE_TOKEN_FILE environment variable (path to a file containing the token)
//
// Returns an empty string if no token is found from any source.
func GetAPIToken() string {
	if token := ReadStoredToken(); token != "" {
		return token
	}
	if token := os.Getenv("CRANTTABLE_TOKEN"); token != "" {
		return token
	}
	if path := os.Getenv("CRANTTABLE_TOKEN_FILE"); path != "" {
		return readTokenFile(path)
	}
	return ""
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
