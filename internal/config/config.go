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

	CAVEServer         = "https://data.proofreading.zetta.ai"
	CAVETable          = "kronauer_ant_x1"
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
	enforceTokenFilePerms(path)
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return ""
	}
	return string(decoded)
}

// enforceTokenFilePerms repairs credential files that are readable by group or
// others (e.g. after a manual copy or a restore with loose umask) and warns,
// since a base64 token file is only as protected as its permissions.
func enforceTokenFilePerms(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if info.Mode().Perm()&0o077 == 0 {
		return
	}
	if err := os.Chmod(path, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %s is accessible by other users (permissions %o) and could not be fixed: %v\n", path, info.Mode().Perm(), err)
		return
	}
	fmt.Fprintf(os.Stderr, "Warning: %s was accessible by other users (permissions %o); tightened to 0600\n", path, info.Mode().Perm())
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
//  1. Stored credentials from ~/.crantcli/credentials (fallback ~/.crantinject/ and ~/.crant_type_look/)
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
//  1. Stored credentials from ~/.crantcli/cave_credentials
//  2. CAVE_TOKEN environment variable
//  3. CAVE_TOKEN_FILE environment variable (path to a file containing the token)
func GetCAVEToken() string {
	if token := ReadStoredCAVEToken(); token != "" {
		return token
	}
	if token := os.Getenv("CAVE_TOKEN"); token != "" {
		return token
	}
	if path := os.Getenv("CAVE_TOKEN_FILE"); path != "" {
		return readTokenFile(path)
	}
	return ""
}

// RunSetupPrompt interactively prompts the user for their SeaTable token and stores it.
// Returns an error if stdin is not a terminal.
func RunSetupPrompt() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("stdin is not a terminal; set CRANTTABLE_TOKEN or CRANTTABLE_TOKEN_FILE instead")
	}

	fmt.Println("SeaTable token:")
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

	fmt.Println("SeaTable token saved.")
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

	fmt.Println("CAVE token saved.")
	return nil
}
