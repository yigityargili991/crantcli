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

	CAVEGlobalServer   = "https://global.daf-apis.com"
	CAVEServer         = "https://data.proofreading.zetta.ai"
	CAVETable          = "kronauer_ant_x1"
	CAVESkeletonTable  = "kronauer_ant"
	SupervoxelIDColumn = "supervoxel_id"

	appConfigDir        = ".crantcli"
	legacyAppConfigDir  = ".crantinject"
	legacyAppConfigDir2 = ".crant_type_look"
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

func legacyCredentialFilePaths() []string {
	return []string{
		credentialFilePathForDir(legacyAppConfigDir),
		credentialFilePathForDir(legacyAppConfigDir2),
	}
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

// ReadStoredToken reads a base64-encoded token from ~/.crantcli/credentials.
// For migration compatibility, it falls back to ~/.crantinject/ and ~/.crant_type_look/.
func ReadStoredToken() string {
	if token := readStoredTokenAtPath(credentialFilePath()); token != "" {
		return token
	}
	for _, path := range legacyCredentialFilePaths() {
		if token := readStoredTokenAtPath(path); token != "" {
			return token
		}
	}
	return ""
}

func storeEncodedToken(path, token string) error {
	if path == "" {
		return fmt.Errorf("could not determine home directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(token))
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing credentials file: %w", err)
	}
	return nil
}

func StoreToken(token string) error {
	return storeEncodedToken(credentialFilePath(), token)
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
//  1. CRANTTABLE_TOKEN environment variable
//  2. CRANTTABLE_TOKEN_FILE environment variable (path to a file containing the token)
//  3. Stored credentials from ~/.crantcli/credentials (fallback ~/.crantinject/ and ~/.crant_type_look/)
//
// Returns an empty string if no token is found from any source.
func GetAPIToken() string {
	if token := strings.TrimSpace(os.Getenv("CRANTTABLE_TOKEN")); token != "" {
		return token
	}
	if path := os.Getenv("CRANTTABLE_TOKEN_FILE"); path != "" {
		if token := readTokenFile(path); token != "" {
			return token
		}
	}
	if token := ReadStoredToken(); token != "" {
		return token
	}
	return ""
}

func caveCredentialFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, appConfigDir, "cave_credentials")
}

// ReadStoredCAVEToken reads a base64-encoded CAVE token from ~/.crantcli/cave_credentials.
func ReadStoredCAVEToken() string {
	return readStoredTokenAtPath(caveCredentialFilePath())
}

// StoreCAVEToken stores a CAVE token as base64 in ~/.crantcli/cave_credentials.
func StoreCAVEToken(token string) error {
	return storeEncodedToken(caveCredentialFilePath(), token)
}

// GetCAVEToken retrieves the CAVE API token from one of several sources.
// It checks sources in the following precedence order:
//  1. CAVE_TOKEN environment variable
//  2. CAVE_TOKEN_FILE environment variable (path to a file containing the token)
//  3. Stored credentials from ~/.crantcli/cave_credentials
func GetCAVEToken() string {
	if token := strings.TrimSpace(os.Getenv("CAVE_TOKEN")); token != "" {
		return token
	}
	if path := os.Getenv("CAVE_TOKEN_FILE"); path != "" {
		if token := readTokenFile(path); token != "" {
			return token
		}
	}
	if token := ReadStoredCAVEToken(); token != "" {
		return token
	}
	return ""
}

// RunSetupPrompt interactively prompts the user for their SeaTable token and stores it.
// Returns an error if stdin is not a terminal.
func RunSetupPrompt() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("no SeaTable token configured and stdin is not a terminal; set CRANTTABLE_TOKEN or run 'crantcli setup'")
	}

	fmt.Println("Let's get set up yeah?")
	fmt.Println("Please copy your SeaTable token here to use crantcli:")
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

// RunCAVESetupPrompt interactively prompts for the CAVE token (optional, can be skipped).
func RunCAVESetupPrompt() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil
	}

	fmt.Println("\nCAVE token (needed for check-cave). Press Enter to skip:")
	fmt.Print("> ")

	tokenBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}
	token := strings.TrimSpace(string(tokenBytes))

	if token == "" {
		fmt.Println("Skipped CAVE token setup.")
		return nil
	}

	if err := StoreCAVEToken(token); err != nil {
		return err
	}

	fmt.Println("CAVE token saved!")
	return nil
}
