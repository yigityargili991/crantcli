package cave

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"crantinject/internal/config"
)

// Client communicates with the CAVE chunkedgraph API to look up current root IDs.
type Client struct {
	token     string
	baseURL   string
	tableName string
	http      *http.Client
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
		http:      &http.Client{Timeout: 30 * time.Second},
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

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("CAVE request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("CAVE request failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

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

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("CAVE batch request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("CAVE batch request failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

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
