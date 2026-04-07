package seatable

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"crantinject/internal/config"
)

// Client holds authenticated SeaTable connection details.
type Client struct {
	accessToken       string
	dtableUUID        string
	metadata          *MetadataResponse
	executeSQLFunc    func(string) (*SQLResponse, error)
	fetchMetadataFunc func() (*MetadataResponse, error)
}

// NewClient creates a new authenticated SeaTable client.
func NewClient() (*Client, error) {
	apiToken := config.GetAPIToken()
	if apiToken == "" {
		return nil, fmt.Errorf("no SeaTable token configured; run 'crantcli setup' to set one")
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
	if c.executeSQLFunc != nil {
		return c.executeSQLFunc(sql)
	}

	url := fmt.Sprintf("%s/api-gateway/api/v2/dtables/%s/sql/", config.SeaTableServer, c.dtableUUID)

	payload, err := json.Marshal(map[string]string{"sql": sql})
	if err != nil {
		return nil, fmt.Errorf("marshaling SQL payload: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating SQL request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
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

// MetadataResponse is the response from the metadata endpoint.
type MetadataResponse struct {
	Metadata struct {
		Tables []TableMeta `json:"tables"`
	} `json:"metadata"`
}

// TableMeta describes a table in the metadata response.
type TableMeta struct {
	Name    string      `json:"name"`
	Columns []ColumnDef `json:"columns"`
}

// ColumnDef describes a column definition including its options.
type ColumnDef struct {
	Key  string      `json:"key"`
	Name string      `json:"name"`
	Type string      `json:"type"`
	Data *ColumnData `json:"data"`
}

// ColumnData holds column-type-specific configuration.
type ColumnData struct {
	Options []SelectOption `json:"options"`
}

// SelectOption represents a single/multiple select option.
type SelectOption struct {
	ID   interface{} `json:"id"`
	Name string      `json:"name"`
}

// FetchMetadata retrieves the base metadata including table and column definitions.
func (c *Client) FetchMetadata() (*MetadataResponse, error) {
	if c.metadata != nil {
		return c.metadata, nil
	}
	if c.fetchMetadataFunc != nil {
		meta, err := c.fetchMetadataFunc()
		if err != nil {
			return nil, err
		}
		c.metadata = meta
		return meta, nil
	}

	url := fmt.Sprintf("%s/api-gateway/api/v2/dtables/%s/metadata/", config.SeaTableServer, c.dtableUUID)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating metadata request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("metadata request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("metadata request failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result MetadataResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding metadata response: %w", err)
	}
	c.metadata = &result
	return c.metadata, nil
}

// columnOptions returns the select options for the given column name
// in the configured table, or nil if not found.
func columnOptions(meta *MetadataResponse, columnName string) []SelectOption {
	for _, table := range meta.Metadata.Tables {
		if table.Name != config.SeaTableTable {
			continue
		}
		for _, col := range table.Columns {
			if col.Name == columnName && col.Data != nil {
				return col.Data.Options
			}
		}
	}
	return nil
}

// SelectOptionMap returns a map from option ID (as string) to option name
// for a given column in the configured table.
func SelectOptionMap(meta *MetadataResponse, columnName string) map[string]string {
	m := make(map[string]string)
	for _, opt := range columnOptions(meta, columnName) {
		m[fmt.Sprintf("%v", opt.ID)] = opt.Name
	}
	return m
}

// SelectOptionNameMap returns a map from lowercased option name to option ID.
func SelectOptionNameMap(meta *MetadataResponse, columnName string) map[string]string {
	m := make(map[string]string)
	for _, opt := range columnOptions(meta, columnName) {
		m[strings.ToLower(strings.TrimSpace(opt.Name))] = fmt.Sprintf("%v", opt.ID)
	}
	return m
}

// normalizeResultKeys ensures each row can be read by both SeaTable column key
// (e.g. "0000") and column name (e.g. "root_id").
func normalizeResultKeys(resp *SQLResponse) {
	if resp == nil || len(resp.Metadata) == 0 || len(resp.Results) == 0 {
		return
	}

	for _, row := range resp.Results {
		if row == nil {
			continue
		}
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
