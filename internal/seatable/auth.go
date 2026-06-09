package seatable

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"

	"crantcli/internal/config"
	"crantcli/internal/httperror"
)

// ExchangeToken exchanges an API token for a base access token.
func ExchangeToken(apiToken string) (*AuthResponse, error) {
	// Flow 1: API token (Base API-Token) -> app access token.
	appURL := config.SeaTableServer + "/api/v2.1/dtable/app-access-token/"
	auth, status, body, err := exchangeTokenAtURL(apiToken, appURL)
	if err == nil {
		return auth, nil
	}

	// Flow 2: Account/Personal token -> base access token using workspace/base path.
	// Python client usage with personal tokens commonly follows this route.
	workspaceURL := fmt.Sprintf("%s/api/v2.1/workspace/%s/dtable/%s/access-token/",
		config.SeaTableServer,
		neturl.PathEscape(config.SeaTableWorkspace),
		neturl.PathEscape(config.SeaTableBase),
	)
	auth2, status2, body2, err2 := exchangeTokenAtURL(apiToken, workspaceURL)
	if err2 == nil {
		return auth2, nil
	}

	return nil, fmt.Errorf(
		"auth failed for both token flows: app-access-token (HTTP %d): %s; workspace/base access-token (HTTP %d): %s",
		status, body, status2, body2,
	)
}

func exchangeTokenAtURL(apiToken, url string) (*AuthResponse, int, string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, "", fmt.Errorf("creating auth request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, "", fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyPreview, err := httperror.Preview(resp.Body)
		if err != nil {
			return nil, resp.StatusCode, "", fmt.Errorf("reading auth error response: %w", err)
		}
		return nil, resp.StatusCode, bodyPreview, fmt.Errorf("auth failed (HTTP %d)", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyPreview := httperror.Redact(strings.TrimSpace(string(body)))

	var auth AuthResponse
	if err := json.Unmarshal(body, &auth); err != nil {
		return nil, resp.StatusCode, bodyPreview, fmt.Errorf("decoding auth response: %w", err)
	}

	if auth.AccessToken == "" || auth.DTableUUID == "" {
		return nil, resp.StatusCode, bodyPreview, fmt.Errorf("auth response missing access_token or dtable_uuid")
	}

	return &auth, resp.StatusCode, bodyPreview, nil
}
