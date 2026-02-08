package seatable

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"crant_type_look/internal/config"
)

// Client holds authenticated SeaTable connection details.
type Client struct {
	accessToken string
	dtableUUID  string
}

// NewClient creates a new authenticated SeaTable client.
func NewClient() (*Client, error) {
	apiToken := config.GetAPIToken()
	if apiToken == "" {
		return nil, fmt.Errorf("no SeaTable token configured; run 'crant_type_look setup' to set one")
	}

	auth, err := ExchangeToken(apiToken)
	if err != nil {
		return nil, err
	}

	return &Client{
		accessToken: auth.AccessToken,
		dtableUUID:  auth.DTableUUID,
	}, nil
}

// ExecuteSQL runs a SQL query against the SeaTable base and returns results.
func (c *Client) ExecuteSQL(sql string) (*SQLResponse, error) {
	url := fmt.Sprintf("%s/api-gateway/api/v2/dtables/%s/sql/", config.SeaTableServer, c.dtableUUID)

	payload, err := json.Marshal(map[string]string{"sql": sql})
	if err != nil {
		return nil, fmt.Errorf("marshaling SQL payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating SQL request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SQL request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SQL query failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result SQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding SQL response: %w", err)
	}
	normalizeResultKeys(&result)

	return &result, nil
}

// normalizeResultKeys ensures each row can be read by both SeaTable column key
// (e.g. "0000") and column name (e.g. "root_id").
func normalizeResultKeys(resp *SQLResponse) {
	if resp == nil || len(resp.Metadata) == 0 || len(resp.Results) == 0 {
		return
	}

	for _, row := range resp.Results {
		for _, col := range resp.Metadata {
			if col.Key == "" || col.Name == "" {
				continue
			}
			if _, exists := row[col.Name]; exists {
				continue
			}
			if v, ok := row[col.Key]; ok {
				row[col.Name] = v
			}
		}
	}
}
