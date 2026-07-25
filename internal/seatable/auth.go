package seatable

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"

	"crantcli/internal/config"
	"crantcli/internal/httpx"
)

// ExchangeToken exchanges an API token for a base access token.
func ExchangeToken(apiToken string) (*AuthResponse, error) {
	return exchangeToken(apiToken, config.SeaTableServer)
}

func exchangeToken(apiToken, server string) (*AuthResponse, error) {
	// Flow 1: API token (Base API-Token) -> app access token.
	appURL := server + "/api/v2.1/dtable/app-access-token/"
	auth, status, _, err := exchangeTokenAtURL(apiToken, appURL)
	if err == nil {
		return auth, nil
	}

	// Flow 2: Account/Personal token -> base access token using workspace/base path.
	// Python client usage with personal tokens commonly follows this route.
	workspaceURL := fmt.Sprintf("%s/api/v2.1/workspace/%s/dtable/%s/access-token/",
		server,
		neturl.PathEscape(config.SeaTableWorkspace),
		neturl.PathEscape(config.SeaTableBase),
	)
	auth2, status2, _, err2 := exchangeTokenAtURL(apiToken, workspaceURL)
	if err2 == nil {
		return auth2, nil
	}

	// Response bodies are deliberately excluded: a server could reflect the
	// Authorization header into its error body, which would leak the API token
	// into logs and terminal output.
	return nil, fmt.Errorf(
		"auth failed for both token flows: app-access-token (HTTP %d); workspace/base access-token (HTTP %d)",
		status, status2,
	)
}

func exchangeTokenAtURL(apiToken, url string) (*AuthResponse, int, string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, "", fmt.Errorf("creating auth request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpx.Do(httpClient, req)
	if err != nil {
		var statusErr *httpx.StatusError
		if errors.As(err, &statusErr) {
			return nil, statusErr.StatusCode, string(statusErr.Body), fmt.Errorf("auth failed (HTTP %d)", statusErr.StatusCode)
		}
		return nil, 0, "", fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, httpx.MaxErrorBody))
	var auth AuthResponse
	if err := json.Unmarshal(body, &auth); err != nil {
		return nil, resp.StatusCode, string(body), fmt.Errorf("decoding auth response: %w", err)
	}

	if auth.AccessToken == "" || auth.DTableUUID == "" {
		return nil, resp.StatusCode, string(body), fmt.Errorf("auth response missing access_token or dtable_uuid")
	}

	return &auth, resp.StatusCode, string(body), nil
}
