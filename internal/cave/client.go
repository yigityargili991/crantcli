package cave

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"

	"crantcli/internal/config"
	"crantcli/internal/httpx"
)

// Client communicates with the CAVE chunkedgraph API to look up current root IDs.
type Client struct {
	token     string
	baseURL   string
	tableName string
	http      *http.Client
}

// ChangeLogRow is one CAVE tabular changelog row for a root ID.
type ChangeLogRow struct {
	OperationID     uint64   `json:"operation_id"`
	Timestamp       int64    `json:"timestamp"`
	UserID          uint64   `json:"user_id"`
	BeforeRootIDs   []uint64 `json:"before_root_ids"`
	AfterRootIDs    []uint64 `json:"after_root_ids"`
	IsMerge         bool     `json:"is_merge"`
	UserName        string   `json:"user_name"`
	UserAffiliation string   `json:"user_affiliation"`
}

// NewTestClient creates a CAVE client with a custom base URL and HTTP client, for testing.
func NewTestClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{
		token:     "test-token",
		baseURL:   baseURL,
		tableName: config.CAVETable,
		http:      httpClient,
	}
}

// NewClient creates a CAVE client using the stored or env-var CAVE token.
func NewClient() (*Client, error) {
	token := config.GetCAVEToken()
	if token == "" {
		return nil, fmt.Errorf("no CAVE token configured; run 'crantcli setup' or set CAVE_TOKEN")
	}
	return &Client{
		token:     token,
		baseURL:   config.CAVEServer,
		tableName: config.CAVETable,
		http:      httpx.DefaultClient,
	}, nil
}

// GetRootID returns the current root ID for a single supervoxel ID.
func (c *Client) GetRootID(supervoxelID uint64) (uint64, error) {
	url := fmt.Sprintf("%s/segmentation/api/v1/table/%s/node/%d/root",
		c.baseURL, c.tableName, supervoxelID)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("creating CAVE request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := httpx.Do(c.http, req)
	if err != nil {
		return 0, fmt.Errorf("CAVE request failed: %w", err)
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()

	var result map[string]json.Number
	if err := dec.Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding CAVE response: %w", err)
	}

	rootNum, ok := result["root_id"]
	if !ok {
		return 0, fmt.Errorf("CAVE response missing root_id field")
	}

	rootID, err := strconv.ParseUint(string(rootNum), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing CAVE root_id %q: %w", rootNum, err)
	}
	return rootID, nil
}

// GetRootIDs returns the current root IDs for multiple supervoxel IDs using
// the binary batch endpoint.
func (c *Client) GetRootIDs(supervoxelIDs []uint64) ([]uint64, error) {
	if len(supervoxelIDs) == 0 {
		return nil, nil
	}

	url := fmt.Sprintf("%s/segmentation/api/v1/table/%s/roots_binary",
		c.baseURL, c.tableName)

	buf := make([]byte, 8*len(supervoxelIDs))
	for i, id := range supervoxelIDs {
		binary.LittleEndian.PutUint64(buf[i*8:], id)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("creating CAVE batch request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(buf))

	resp, err := httpx.Do(c.http, req)
	if err != nil {
		return nil, fmt.Errorf("CAVE batch request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading CAVE batch response: %w", err)
	}

	if len(respBody)%8 != 0 {
		return nil, fmt.Errorf("CAVE batch response has unexpected length %d (not a multiple of 8)", len(respBody))
	}

	rootIDs := make([]uint64, len(respBody)/8)
	for i := range rootIDs {
		rootIDs[i] = binary.LittleEndian.Uint64(respBody[i*8:])
	}
	return rootIDs, nil
}

// GetRootChangeLog returns the tabular CAVE edit history for a root ID.
func (c *Client) GetRootChangeLog(rootID uint64, filtered bool) ([]ChangeLogRow, error) {
	url := fmt.Sprintf("%s/segmentation/api/v1/table/%s/root/%d/tabular_change_log",
		c.baseURL, c.tableName, rootID)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating CAVE changelog request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	query := req.URL.Query()
	query.Set("filtered", strconv.FormatBool(filtered))
	req.URL.RawQuery = query.Encode()

	resp, err := httpx.Do(c.http, req)
	if err != nil {
		// CAVE sometimes returns HTTP 500 "Read timed out" when a root's
		// history is too large to compute; treat that as "no history".
		var statusErr *httpx.StatusError
		if errors.As(err, &statusErr) &&
			statusErr.StatusCode == http.StatusInternalServerError &&
			bytes.Contains(statusErr.Body, []byte("Read timed out")) {
			return nil, nil
		}
		return nil, fmt.Errorf("CAVE changelog request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading CAVE changelog response: %w", err)
	}

	rows, err := decodeChangeLogRows(respBody)
	if err != nil {
		return nil, fmt.Errorf("decoding CAVE changelog response: %w", err)
	}
	return rows, nil
}

type changeLogTableResponse struct {
	OperationID     map[string]json.RawMessage `json:"operation_id"`
	Timestamp       map[string]json.RawMessage `json:"timestamp"`
	UserID          map[string]json.RawMessage `json:"user_id"`
	BeforeRootIDs   map[string][]uint64        `json:"before_root_ids"`
	AfterRootIDs    map[string][]uint64        `json:"after_root_ids"`
	IsMerge         map[string]bool            `json:"is_merge"`
	UserName        map[string]string          `json:"user_name"`
	UserAffiliation map[string]string          `json:"user_affiliation"`
}

func decodeChangeLogRows(data []byte) ([]ChangeLogRow, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}

	if data[0] == '[' {
		var rows []ChangeLogRow
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, err
		}
		return rows, nil
	}

	var table changeLogTableResponse
	if err := json.Unmarshal(data, &table); err != nil {
		return nil, err
	}

	keys := changeLogRowKeys(table)
	rows := make([]ChangeLogRow, 0, len(keys))
	for _, key := range keys {
		operationID, err := requiredUint(table.OperationID, key, "operation_id")
		if err != nil {
			return nil, err
		}
		timestamp, err := requiredInt64(table.Timestamp, key, "timestamp")
		if err != nil {
			return nil, err
		}
		userID, err := requiredUint(table.UserID, key, "user_id")
		if err != nil {
			return nil, err
		}

		rows = append(rows, ChangeLogRow{
			OperationID:     operationID,
			Timestamp:       timestamp,
			UserID:          userID,
			BeforeRootIDs:   table.BeforeRootIDs[key],
			AfterRootIDs:    table.AfterRootIDs[key],
			IsMerge:         table.IsMerge[key],
			UserName:        table.UserName[key],
			UserAffiliation: table.UserAffiliation[key],
		})
	}
	return rows, nil
}

func changeLogRowKeys(table changeLogTableResponse) []string {
	seen := make(map[string]struct{})
	addRawKeys := func(m map[string]json.RawMessage) {
		for key := range m {
			seen[key] = struct{}{}
		}
	}
	addRootListKeys := func(m map[string][]uint64) {
		for key := range m {
			seen[key] = struct{}{}
		}
	}
	addBoolKeys := func(m map[string]bool) {
		for key := range m {
			seen[key] = struct{}{}
		}
	}
	addStringKeys := func(m map[string]string) {
		for key := range m {
			seen[key] = struct{}{}
		}
	}

	addRawKeys(table.OperationID)
	addRawKeys(table.Timestamp)
	addRawKeys(table.UserID)
	addRootListKeys(table.BeforeRootIDs)
	addRootListKeys(table.AfterRootIDs)
	addBoolKeys(table.IsMerge)
	addStringKeys(table.UserName)
	addStringKeys(table.UserAffiliation)

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, leftErr := strconv.ParseInt(keys[i], 10, 64)
		right, rightErr := strconv.ParseInt(keys[j], 10, 64)
		if leftErr == nil && rightErr == nil {
			return left < right
		}
		if leftErr == nil {
			return true
		}
		if rightErr == nil {
			return false
		}
		return keys[i] < keys[j]
	})
	return keys
}

func requiredUint(values map[string]json.RawMessage, key, field string) (uint64, error) {
	raw, ok := values[key]
	if !ok {
		return 0, fmt.Errorf("missing %s for changelog row %s", field, key)
	}
	return parseJSONUint(raw, field, key)
}

func requiredInt64(values map[string]json.RawMessage, key, field string) (int64, error) {
	raw, ok := values[key]
	if !ok {
		return 0, fmt.Errorf("missing %s for changelog row %s", field, key)
	}
	return parseJSONInt64(raw, field, key)
}

func parseJSONUint(raw json.RawMessage, field, key string) (uint64, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		value, err := strconv.ParseUint(text, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parsing %s %q for changelog row %s: %w", field, text, key, err)
		}
		return value, nil
	}

	var value uint64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("parsing %s for changelog row %s: %w", field, key, err)
	}
	return value, nil
}

func parseJSONInt64(raw json.RawMessage, field, key string) (int64, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		value, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parsing %s %q for changelog row %s: %w", field, text, key, err)
		}
		return value, nil
	}

	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("parsing %s for changelog row %s: %w", field, key, err)
	}
	return value, nil
}
