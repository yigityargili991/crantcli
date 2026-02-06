package seatable

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"crant_type_look/internal/config"
)

// ExchangeToken exchanges an API token for a base access token.
func ExchangeToken(apiToken string) (*AuthResponse, error) {
	url := config.SeaTableServer + "/api/v2.1/dtable/app-access-token/"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating auth request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("auth failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var auth AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&auth); err != nil {
		return nil, fmt.Errorf("decoding auth response: %w", err)
	}

	if auth.AccessToken == "" || auth.DTableUUID == "" {
		return nil, fmt.Errorf("auth response missing access_token or dtable_uuid")
	}

	return &auth, nil
}
