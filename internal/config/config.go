package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zalando/go-keyring"
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

	credentialKeyringService  = "crantcli"
	seaTableCredentialAccount = "seatable-api-token"
	caveCredentialAccount     = "cave-api-token"
)

type credentialKeyring interface {
	Get(service, account string) (string, error)
	Set(service, account, secret string) error
	Delete(service, account string) error
}

type systemCredentialKeyring struct{}

func (systemCredentialKeyring) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}

func (systemCredentialKeyring) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret)
}

func (systemCredentialKeyring) Delete(service, account string) error {
	return keyring.Delete(service, account)
}

var (
	activeCredentialKeyring       credentialKeyring = systemCredentialKeyring{}
	credentialFileFallbackAllowed                   = runtime.GOOS == "linux"
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

func ensurePrivateConfigDir(dir string) error {
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("creating config directory: %w", err)
		}
	case err != nil:
		return fmt.Errorf("checking config directory: %w", err)
	case info.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("config directory %s must not be a symbolic link", dir)
	case !info.IsDir():
		return fmt.Errorf("config path %s is not a directory", dir)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("setting config directory permissions: %w", err)
		}
	}
	return nil
}

func readEncodedTokenAtPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("could not determine home directory")
	}

	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("credential file %s must not be a symbolic link", path)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("credential path %s is not a regular file", path)
	}

	if err := ensurePrivateConfigDir(filepath.Dir(path)); err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" {
		if info.Mode().Perm()&0o077 != 0 {
			if err := os.Chmod(path, 0o600); err != nil {
				return "", fmt.Errorf("credential file %s is accessible by other users and could not be secured: %w", path, err)
			}
			fmt.Fprintf(os.Stderr, "Warning: %s was accessible by other users (permissions %o); tightened to 0600\n", path, info.Mode().Perm())
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return "", fmt.Errorf("decoding credential file %s: %w", path, err)
	}
	return string(decoded), nil
}

func removeCredentialFile(path string) error {
	dir := filepath.Dir(path)
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("checking credential directory %s: %w", dir, err)
	case info.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("credential directory %s is a symbolic link; refusing to remove files through it", dir)
	case !info.IsDir():
		return fmt.Errorf("credential path %s is not a directory", dir)
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func removeCredentialFiles(paths []string) error {
	var removeErrors []error
	for _, path := range paths {
		if path == "" {
			continue
		}
		if err := removeCredentialFile(path); err != nil {
			removeErrors = append(removeErrors, fmt.Errorf("removing %s: %w", path, err))
		}
	}
	return errors.Join(removeErrors...)
}

func readFileCredential(paths []string) (token, sourcePath string) {
	for _, path := range paths {
		token, err := readEncodedTokenAtPath(path)
		if err == nil && token != "" {
			return token, path
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "Warning: could not read credential file %s: %v\n", path, err)
		}
	}
	return "", ""
}

func storeEncodedToken(path, token string) error {
	if path == "" {
		return fmt.Errorf("could not determine home directory")
	}

	dir := filepath.Dir(path)
	if err := ensurePrivateConfigDir(dir); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("credential file %s must not be a symbolic link", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking credential file: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(token))
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temporary credential file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("setting temporary credential permissions: %w", err)
	}
	if _, err := temp.WriteString(encoded + "\n"); err != nil {
		temp.Close()
		return fmt.Errorf("writing temporary credential file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("syncing temporary credential file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("closing temporary credential file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("installing credential file: %w", err)
	}
	return nil
}

func storeKeyringCredential(account, token string) error {
	if err := activeCredentialKeyring.Set(credentialKeyringService, account, token); err != nil {
		return err
	}
	stored, err := activeCredentialKeyring.Get(credentialKeyringService, account)
	if err != nil {
		err = fmt.Errorf("verifying stored credential: %w", err)
	} else if stored != token {
		err = fmt.Errorf("verifying stored credential: value does not match")
	}
	if err == nil {
		return nil
	}
	if deleteErr := activeCredentialKeyring.Delete(credentialKeyringService, account); deleteErr != nil &&
		!errors.Is(deleteErr, keyring.ErrNotFound) {
		return fmt.Errorf("%w; removing unverifiable credential: %v", err, deleteErr)
	}
	return err
}

func readStoredCredential(account string, filePaths []string) string {
	var token, sourcePath string
	if credentialFileFallbackAllowed {
		token, sourcePath = readFileCredential(filePaths)
	}

	if token == "" {
		if stored, err := activeCredentialKeyring.Get(credentialKeyringService, account); err == nil {
			if !credentialFileFallbackAllowed {
				fileToken, _ := readFileCredential(filePaths)
				if fileToken != "" && fileToken != stored {
					fmt.Fprintln(os.Stderr, "Warning: a legacy credential file differs from the system credential store and was retained for manual review")
					return stored
				}
				if fileToken != "" {
					if err := removeCredentialFiles(filePaths); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: credential is stored securely, but an old credential file could not be removed: %v\n", err)
					}
				}
			}
			return stored
		}
	}

	if token == "" && !credentialFileFallbackAllowed {
		token, sourcePath = readFileCredential(filePaths)
	}
	if token == "" {
		return ""
	}

	if err := storeKeyringCredential(account, token); err == nil {
		if err := removeCredentialFiles(filePaths); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: credential was migrated to the system credential store, but an old credential file could not be removed: %v\n", err)
		}
		return token
	}

	if !credentialFileFallbackAllowed {
		fmt.Fprintln(os.Stderr, "Warning: the system credential store is unavailable; use the token environment variable or token-file environment variable")
		return ""
	}

	currentPath := filePaths[0]
	if sourcePath != currentPath {
		if err := storeEncodedToken(currentPath, token); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not migrate legacy credential file: %v\n", err)
			return token
		}
		if err := removeCredentialFiles(filePaths[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: migrated credential, but an old credential file could not be removed: %v\n", err)
		}
	}
	return token
}

func storeCredential(account, fallbackPath, token string, obsoletePaths []string) error {
	if err := storeKeyringCredential(account, token); err == nil {
		if err := removeCredentialFiles(append([]string{fallbackPath}, obsoletePaths...)); err != nil {
			if credentialFileFallbackAllowed {
				if fallbackErr := storeEncodedToken(fallbackPath, token); fallbackErr != nil {
					return fmt.Errorf("credential was saved securely, but old credential cleanup failed (%v) and the fallback could not be refreshed: %w", err, fallbackErr)
				}
			}
			return fmt.Errorf("credential was saved securely, but an old credential file could not be removed: %w", err)
		}
		return nil
	} else if !credentialFileFallbackAllowed {
		return fmt.Errorf("system credential store is unavailable: %w; use the token environment variable or token-file environment variable instead", err)
	}

	if err := storeEncodedToken(fallbackPath, token); err != nil {
		return err
	}
	if err := removeCredentialFiles(obsoletePaths); err != nil {
		return fmt.Errorf("credential was saved, but an old credential file could not be removed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Warning: system credential store unavailable; token saved in owner-only file %s\n", fallbackPath)
	return nil
}

// ReadStoredToken reads the SeaTable token from the system credential store.
// Existing file credentials are migrated automatically.
func ReadStoredToken() string {
	return readStoredCredential(
		seaTableCredentialAccount,
		append([]string{credentialFilePath()}, legacyCredentialFilePaths()...),
	)
}

func StoreToken(token string) error {
	return storeCredential(
		seaTableCredentialAccount,
		credentialFilePath(),
		token,
		legacyCredentialFilePaths(),
	)
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
//  1. System credential store (with migration from older credential files)
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

// ReadStoredCAVEToken reads the CAVE token from the system credential store.
// Existing file credentials are migrated automatically.
func ReadStoredCAVEToken() string {
	return readStoredCredential(caveCredentialAccount, []string{caveCredentialFilePath()})
}

// StoreCAVEToken stores a CAVE token in the system credential store.
func StoreCAVEToken(token string) error {
	return storeCredential(caveCredentialAccount, caveCredentialFilePath(), token, nil)
}

// GetCAVEToken retrieves the CAVE API token from one of several sources.
// It checks sources in the following precedence order:
//  1. System credential store (with migration from the older credential file)
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
