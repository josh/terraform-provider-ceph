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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-cephfs--fs_id--ls_dir>

type CephFSDirectory struct {
	Name string
	Path string
}

type cephFSDirEntry struct {
	Name      string               `json:"name"`
	Path      string               `json:"path"`
	Snapshots []CephFSSnapshotInfo `json:"snapshots"`
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

func (c *Client) CephFSGetDirectory(ctx context.Context, fsID int, dirPath string) (*CephFSDirectory, error) {
	entries, err := c.cephFSListDir(ctx, fsID, path.Dir(dirPath))
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.Path == dirPath {
			return &CephFSDirectory{Name: entry.Name, Path: entry.Path}, nil
		}
	}

	return nil, ErrNotFound
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-cephfs--fs_id--tree>

func (c *Client) CephFSMkTree(ctx context.Context, fsID int, dirPath string) error {
	jsonPayload, err := json.Marshal(map[string]string{"path": dirPath})
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/cephfs", strconv.Itoa(fsID), "tree").String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
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

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-cephfs--fs_id--tree>

func (c *Client) CephFSRmTree(ctx context.Context, fsID int, dirPath string) error {
	endpoint := c.endpoint.JoinPath("/api/cephfs", strconv.Itoa(fsID), "tree")
	query := url.Values{}
	query.Add("path", dirPath)
	endpoint.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", endpoint.String(), nil)
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

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted && httpResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(httpResp.Body)
		// A non-empty directory escapes the dashboard as an opaque 500.
		if httpResp.StatusCode == http.StatusInternalServerError {
			return fmt.Errorf("ceph API returned status %d: %s (hint: the directory must be empty before it can be removed)", httpResp.StatusCode, string(body))
		}
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}
