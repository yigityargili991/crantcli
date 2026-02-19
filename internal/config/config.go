package config

import (
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

	appConfigDir       = ".crantinject"
	legacyAppConfigDir = ".crant_type_look"
)

func credentialFilePathForDir(configDir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, configDir, "credentials")
}

func credentialFilePath() string {
	return credentialFilePathForDir(appConfigDir)
}

func legacyCredentialFilePath() string {
	return credentialFilePathForDir(legacyAppConfigDir)
}

func readStoredTokenAtPath(path string) string {
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

// ReadStoredToken reads a base64-encoded token from ~/.crantinject/credentials.
// For migration compatibility, it falls back to ~/.crant_type_look/credentials.
func ReadStoredToken() string {
	if token := readStoredTokenAtPath(credentialFilePath()); token != "" {
		return token
	}
	return readStoredTokenAtPath(legacyCredentialFilePath())
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
//  1. Stored credentials from ~/.crantinject/credentials (fallback ~/.crant_type_look/credentials)
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
		return fmt.Errorf("no SeaTable token configured and stdin is not a terminal; set CRANTTABLE_TOKEN or run 'crantinject setup'")
	}

	fmt.Println("Let's get set up yeah?")
	fmt.Println("Please copy your SeaTable token here to use crantinject:")
	fmt.Print("> ")

	tokenBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}
	token := strings.TrimSpace(string(tokenBytes))

	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	if err := StoreToken(token); err != nil {
		return err
	}

	fmt.Println("Token saved! You're all set.")
	return nil
}
