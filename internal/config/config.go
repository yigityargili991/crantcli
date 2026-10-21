package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	// CAVEAuthServer issues the CAVE token: CAVEServer and the state server
	// on this host name it as the realm of their 401 responses.
	CAVEAuthServer = "https://proofreading.zetta.ai"
	// CAVEGlobalAuthServer issues the global CAVE token. NglStateServer, where
	// Spelunker's Share button stores states, names it as its realm.
	CAVEGlobalAuthServer = "https://" + caveGlobalAuthHost
	caveGlobalAuthHost   = "global.daf-apis.com"

	appConfigDir        = ".crantcli"
	legacyAppConfigDir  = ".crantinject"
	legacyAppConfigDir2 = ".crant_type_look"

	credentialKeyringService    = "crantcli"
	seaTableCredentialAccount   = "seatable-api-token"
	caveCredentialAccount       = "cave-api-token"
	caveGlobalCredentialAccount = "cave-global-api-token"

	// storedCredentialSource names the credential crantcli setup stores
	// when a message says where a token came from.
	storedCredentialSource = "crantcli's credential store"

	// maxCAVEClientTokenFileBytes bounds the read of a caveclient token
	// file, which holds a small JSON object.
	maxCAVEClientTokenFileBytes = 64 << 10
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
		token = strings.TrimSpace(token)
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

// readKeyringCredential returns the token stored for account, or "" when
// there is none. A stored value that is only whitespace is no token.
func readKeyringCredential(account string) string {
	stored, err := activeCredentialKeyring.Get(credentialKeyringService, account)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(stored)
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
	// On Linux the current fallback file can be newer than the system
	// credential store, so it is read first. Legacy files are only migration
	// sources and never replace a token the store already holds.
	var token, sourcePath string
	legacyPaths := filePaths
	if credentialFileFallbackAllowed {
		token, sourcePath = readFileCredential(filePaths[:1])
		legacyPaths = filePaths[1:]
	}

	if token == "" {
		if stored := readKeyringCredential(account); stored != "" {
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

	if token == "" {
		token, sourcePath = readFileCredential(legacyPaths)
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

// maxTokenFileBytes bounds the read of a file named by a token-file
// variable, which holds a single token.
const maxTokenFileBytes = 64 << 10

// readTokenFile returns the trimmed token in the file at path, or "" when
// there is none. Only a regular file (or a symbolic link to one) of at most
// maxTokenFileBytes is read, so a FIFO cannot block the lookup and a huge
// file is not loaded into memory. A missing file is ignored silently; any
// other problem draws a warning that names the path but never the contents.
func readTokenFile(path string) string {
	token, err := readRawTokenAtPath(path)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read token file %s: %v\n", path, err)
		return ""
	}
	return token
}

func readRawTokenAtPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxTokenFileBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxTokenFileBytes {
		return "", fmt.Errorf("larger than %d bytes", maxTokenFileBytes)
	}
	return strings.TrimSpace(string(data)), nil
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
	if token := strings.TrimSpace(os.Getenv("CRANTTABLE_TOKEN")); token != "" {
		return token
	}
	if path := os.Getenv("CRANTTABLE_TOKEN_FILE"); path != "" {
		return readTokenFile(path)
	}
	return ""
}

func caveCredentialFilePathForDir(configDir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, configDir, "cave_credentials")
}

func caveCredentialFilePath() string {
	return caveCredentialFilePathForDir(appConfigDir)
}

// The CAVE token was added after the rename to crantinject, so
// ~/.crant_type_look never held one.
func legacyCAVECredentialFilePaths() []string {
	return []string{caveCredentialFilePathForDir(legacyAppConfigDir)}
}

// ReadStoredCAVEToken reads the CAVE token from the system credential store.
// Existing file credentials are migrated automatically.
func ReadStoredCAVEToken() string {
	return readStoredCredential(
		caveCredentialAccount,
		append([]string{caveCredentialFilePath()}, legacyCAVECredentialFilePaths()...),
	)
}

// StoreCAVEToken stores a CAVE token in the system credential store.
func StoreCAVEToken(token string) error {
	return storeCredential(
		caveCredentialAccount,
		caveCredentialFilePath(),
		token,
		legacyCAVECredentialFilePaths(),
	)
}

// GetCAVEToken retrieves the CAVE API token from one of several sources.
// It checks sources in the following precedence order:
//  1. System credential store (with migration from older credential files)
//  2. CAVE_TOKEN environment variable
//  3. CAVE_TOKEN_FILE environment variable (path to a file containing the token)
func GetCAVEToken() string {
	token, _ := LookupCAVEToken()
	return token
}

// LookupCAVEToken is GetCAVEToken that also returns where the token came
// from, named as in CAVETokenSources, so a message can say which token a
// server rejected without showing it.
func LookupCAVEToken() (token, source string) {
	if token := ReadStoredCAVEToken(); token != "" {
		return token, storedCredentialSource
	}
	if token := strings.TrimSpace(os.Getenv("CAVE_TOKEN")); token != "" {
		return token, "CAVE_TOKEN"
	}
	if path := os.Getenv("CAVE_TOKEN_FILE"); path != "" {
		if token := readTokenFile(path); token != "" {
			return token, "CAVE_TOKEN_FILE"
		}
	}
	return "", ""
}

// CAVETokenSources names the places GetCAVEToken looks, in order.
func CAVETokenSources() []string {
	return []string{storedCredentialSource, "CAVE_TOKEN", "CAVE_TOKEN_FILE"}
}

func caveGlobalCredentialFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, appConfigDir, "cave_global_credentials")
}

// ReadStoredCAVEGlobalToken reads the global CAVE token from the system
// credential store.
func ReadStoredCAVEGlobalToken() string {
	return readStoredCredential(caveGlobalCredentialAccount, []string{caveGlobalCredentialFilePath()})
}

// StoreCAVEGlobalToken stores a global CAVE token in the system credential
// store.
func StoreCAVEGlobalToken(token string) error {
	return storeCredential(caveGlobalCredentialAccount, caveGlobalCredentialFilePath(), token, nil)
}

// LookupCAVEGlobalToken retrieves the token CAVEGlobalAuthServer issues,
// which loads Share links stored on global.daf-apis.com, and where it came
// from, named as in CAVEGlobalTokenSources. It checks, in order:
//  1. System credential store (written by crantcli setup)
//  2. CAVE_GLOBAL_TOKEN environment variable
//  3. CAVE_GLOBAL_TOKEN_FILE environment variable (path to a file containing the token)
//  4. caveclient's token file for the server,
//     ~/.cloudvolume/secrets/global.daf-apis.com-cave-secret.json
func LookupCAVEGlobalToken() (token, source string) {
	if token := ReadStoredCAVEGlobalToken(); token != "" {
		return token, storedCredentialSource
	}
	if token := strings.TrimSpace(os.Getenv("CAVE_GLOBAL_TOKEN")); token != "" {
		return token, "CAVE_GLOBAL_TOKEN"
	}
	if path := os.Getenv("CAVE_GLOBAL_TOKEN_FILE"); path != "" {
		if token := readTokenFile(path); token != "" {
			return token, "CAVE_GLOBAL_TOKEN_FILE"
		}
	}
	path := caveClientTokenPath(caveGlobalAuthHost)
	if token := readCAVEClientToken(path); token != "" {
		return token, caveClientTokenSource(path)
	}
	return "", ""
}

// CAVEGlobalTokenSources names the places LookupCAVEGlobalToken looks, in
// order.
func CAVEGlobalTokenSources() []string {
	return []string{
		storedCredentialSource,
		"CAVE_GLOBAL_TOKEN",
		"CAVE_GLOBAL_TOKEN_FILE",
		caveClientTokenSource(caveClientTokenPath(caveGlobalAuthHost)),
	}
}

// caveClientTokenPath is where caveclient saves its token for the auth
// server on host.
func caveClientTokenPath(host string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cloudvolume", "secrets", host+"-cave-secret.json")
}

func caveClientTokenSource(path string) string {
	return strings.TrimSpace("caveclient's token file " + path)
}

// readCAVEClientToken returns the "token" entry of a caveclient token file,
// or "" when there is none. caveclient owns the file, so crantcli only reads
// it: it is never migrated, rewritten, or re-permissioned, and it may be a
// symbolic link. A file that exists but cannot be read draws a warning.
func readCAVEClientToken(path string) string {
	if path == "" {
		return ""
	}
	token, err := readCAVEClientTokenFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read caveclient token file %s: %v\n", path, err)
		return ""
	}
	return token
}

func readCAVEClientTokenFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxCAVEClientTokenFileBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxCAVEClientTokenFileBytes {
		return "", fmt.Errorf("larger than %d bytes", maxCAVEClientTokenFileBytes)
	}
	var secrets struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &secrets); err != nil {
		return "", err
	}
	return strings.TrimSpace(secrets.Token), nil
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
	return runOptionalTokenPrompt(
		"\nCAVE token (needed for check-cave). Press Enter to skip:",
		"Skipped CAVE token setup.",
		"CAVE token saved.",
		StoreCAVEToken,
	)
}

// RunCAVEGlobalSetupPrompt interactively prompts for the global CAVE token
// (optional, can be skipped).
func RunCAVEGlobalSetupPrompt() error {
	return runOptionalTokenPrompt(
		"\nGlobal CAVE token, for Share links stored on global.daf-apis.com (copy one\n"+
			"from "+CAVEGlobalAuthServer+"/sticky_auth/settings/tokens). Without it, crantcli\n"+
			"reads caveclient's ~/.cloudvolume/secrets/"+caveGlobalAuthHost+"-cave-secret.json.\n"+
			"Press Enter to skip:",
		"Skipped global CAVE token setup.",
		"Global CAVE token saved.",
		StoreCAVEGlobalToken,
	)
}

// runOptionalTokenPrompt asks for a token that may be skipped and stores the
// one entered. Without a terminal it does nothing, since the token is
// optional.
func runOptionalTokenPrompt(prompt, skipped, saved string, store func(string) error) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil
	}

	fmt.Println(prompt)
	fmt.Print("> ")

	tokenBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}
	token := strings.TrimSpace(string(tokenBytes))

	if token == "" {
		fmt.Println(skipped)
		return nil
	}

	if err := store(token); err != nil {
		return err
	}

	fmt.Println(saved)
	return nil
}
