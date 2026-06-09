package skeleton

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"crantcli/internal/httperror"
)

type RemoteClient struct {
	server    string
	datastack string
	token     string
	http      *http.Client
}

func NewRemoteClient(server, datastack, token string) RemoteClient {
	return RemoteClient{
		server:    strings.TrimRight(server, "/"),
		datastack: datastack,
		token:     token,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (c RemoteClient) SkeletonExists(ctx context.Context, rootID string) (bool, error) {
	var result bool
	body, err := skeletonCacheRequest(rootID)
	if err != nil {
		return false, err
	}
	err = c.doJSON(ctx, "/precomputed/skeleton/exists", body, &result)
	return result, err
}

func (c RemoteClient) QueueSkeleton(ctx context.Context, rootID string) (float64, error) {
	var raw json.RawMessage
	body, err := skeletonCacheRequest(rootID)
	if err != nil {
		return 0, err
	}
	if err := c.doJSON(ctx, "/bulk/gen_skeletons", body, &raw); err != nil {
		return 0, err
	}
	if len(raw) == 0 {
		return 0, nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		estimate, _ := strconv.ParseFloat(number.String(), 64)
		return estimate, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		estimate, _ := strconv.ParseFloat(text, 64)
		return estimate, nil
	}
	return 0, fmt.Errorf("unexpected skeleton queue response: %s", string(raw))
}

func (c RemoteClient) doJSON(ctx context.Context, path string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding skeletoncache request: %w", err)
	}
	requestURL := fmt.Sprintf("%s/skeletoncache/api/v1/%s%s", c.server, url.PathEscape(c.datastack), path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating skeletoncache request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("skeletoncache request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return httperror.Format("skeletoncache request", resp.StatusCode, resp.Body)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading skeletoncache response: %w", err)
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decoding skeletoncache response: %w", err)
	}
	return nil
}

func skeletonCacheRequest(rootID string) (map[string]any, error) {
	parsed, _, err := ParseRootID(rootID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"root_ids":         []uint64{parsed},
		"skeleton_version": 4,
		"verbose_level":    0,
	}, nil
}
