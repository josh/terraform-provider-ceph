package restapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-cephfs--fs_id--quota>

type CephFSQuota struct {
	MaxBytes int64 `json:"max_bytes"`
	MaxFiles int64 `json:"max_files"`
}

// The quota endpoints are not wrapped in the dashboard's cephfs error
// handler, so a missing path escapes as an opaque HTTP 500. When that
// happens the path's existence is probed by listing its parent through
// ls_dir, which reports entries even when tracebacks are disabled.
func (c *Client) cephFSQuotaPathMissing(ctx context.Context, fsID int, fsPath string, statusCode int, body []byte) bool {
	if isCephFSNotFoundError(statusCode, body) {
		return true
	}
	if statusCode != http.StatusInternalServerError {
		return false
	}
	if bytes.Contains(body, []byte("ObjectNotFound")) ||
		bytes.Contains(body, []byte("No such file or directory")) {
		return true
	}

	parent := path.Dir(fsPath)
	entries, err := c.cephFSListDir(ctx, fsID, parent)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Path == fsPath {
			return false
		}
	}
	return true
}

type cephFSDirEntry struct {
	Path string `json:"path"`
}

func (c *Client) cephFSListDir(ctx context.Context, fsID int, dirPath string) ([]cephFSDirEntry, error) {
	endpoint := c.endpoint.JoinPath("/api/cephfs", strconv.Itoa(fsID), "ls_dir")
	query := url.Values{}
	query.Add("path", dirPath)
	query.Add("depth", "1")
	endpoint.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return nil, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	var entries []cephFSDirEntry
	err = json.Unmarshal(body, &entries)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return entries, nil
}

func (c *Client) GetCephFSQuota(ctx context.Context, fsID int, fsPath string) (*CephFSQuota, error) {
	endpoint := c.endpoint.JoinPath("/api/cephfs", strconv.Itoa(fsID), "quota")
	query := url.Values{}
	query.Add("path", fsPath)
	endpoint.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return nil, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		if c.cephFSQuotaPathMissing(ctx, fsID, fsPath, httpResp.StatusCode, body) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	tflog.Trace(ctx, "Ceph API response body", map[string]any{
		"response_body": string(body),
		"status_code":   httpResp.StatusCode,
	})

	var quota CephFSQuota
	err = json.Unmarshal(body, &quota)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &quota, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#put--api-cephfs--fs_id--quota>

func (c *Client) SetCephFSQuota(ctx context.Context, fsID int, fsPath string, maxBytes, maxFiles int64) error {
	// Omitted values are left untouched by the server, so both are always
	// sent; zero clears the quota.
	jsonPayload, err := json.Marshal(map[string]any{
		"path":      fsPath,
		"max_bytes": maxBytes,
		"max_files": maxFiles,
	})
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/cephfs", strconv.Itoa(fsID), "quota").String()
	httpReq, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		if c.cephFSQuotaPathMissing(ctx, fsID, fsPath, httpResp.StatusCode, body) {
			return ErrNotFound
		}
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}
