package restapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

type SubvolumeListEntry struct {
	Name string         `json:"name"`
	Info *SubvolumeInfo `json:"info,omitempty"`
}

type SubvolumeInfo struct {
	Path          string      `json:"path"`
	Type          string      `json:"type"`
	UID           int         `json:"uid"`
	GID           int         `json:"gid"`
	Atime         string      `json:"atime"`
	Mtime         string      `json:"mtime"`
	Ctime         string      `json:"ctime"`
	Mode          int         `json:"mode"`
	DataPool      string      `json:"data_pool"`
	CreatedAt     string      `json:"created_at"`
	BytesQuota    interface{} `json:"bytes_quota"`
	BytesUsed     int         `json:"bytes_used"`
	BytesPcent    string      `json:"bytes_pcent"`
	PoolNamespace string      `json:"pool_namespace"`
	Features      []string    `json:"features"`
	State         string      `json:"state"`
	Earmark       string      `json:"earmark"`
}

type SubvolumeCreateRequest struct {
	VolName    string `json:"vol_name"`
	SubvolName string `json:"subvol_name"`
	Size       int64  `json:"size,omitempty"`
	GroupName  string `json:"group_name,omitempty"`
	// The volumes module parses mode as an octal string.
	Mode string `json:"mode,omitempty"`
	UID  *int64 `json:"uid,omitempty"`
	GID  *int64 `json:"gid,omitempty"`
	// Must name an existing data pool of the filesystem.
	PoolLayout        string `json:"pool_layout,omitempty"`
	NamespaceIsolated *bool  `json:"namespace_isolated,omitempty"`
	// Must start with the nfs or smb top-level scope.
	Earmark string `json:"earmark,omitempty"`
}

type SubvolumeUpdateRequest struct {
	SubvolName string `json:"subvol_name"`
	Size       int64  `json:"size"`
	GroupName  string `json:"group_name,omitempty"`
}

func (info *SubvolumeInfo) BytesQuotaInt64() (int64, bool) {
	switch v := info.BytesQuota.(type) {
	case float64:
		return int64(v), true
	case string:
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-cephfs-subvolume-vol_name>

func (c *Client) CephFSSubvolumeList(ctx context.Context, volName string, groupName string) ([]SubvolumeListEntry, error) {
	endpoint := c.endpoint.JoinPath("/api/cephfs/subvolume", volName)
	query := url.Values{}
	if groupName != "" {
		query.Add("group_name", groupName)
	}
	query.Add("info", "true")
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

	tflog.Trace(ctx, "Ceph API response body", map[string]any{
		"response_body": string(body),
		"status_code":   httpResp.StatusCode,
	})

	var entries []SubvolumeListEntry
	err = json.Unmarshal(body, &entries)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return entries, nil
}

func (c *Client) CephFSSubvolumeInfo(ctx context.Context, volName string, subvolName string, groupName string) (*SubvolumeInfo, error) {
	endpoint := c.endpoint.JoinPath("/api/cephfs/subvolume", volName, "info")
	query := url.Values{}
	query.Add("subvol_name", subvolName)
	if groupName != "" {
		query.Add("group_name", groupName)
	}
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
		if isCephFSNotFoundError(httpResp.StatusCode, body) {
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

	var info SubvolumeInfo
	err = json.Unmarshal(body, &info)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &info, nil
}

func (c *Client) CephFSSubvolumeCreate(ctx context.Context, req SubvolumeCreateRequest) error {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/cephfs/subvolume").String()
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

	if httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusAccepted && httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

func (c *Client) CephFSSubvolumeUpdate(ctx context.Context, volName string, req SubvolumeUpdateRequest) error {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/cephfs/subvolume", volName).String()
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

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

func (c *Client) CephFSSubvolumeDelete(ctx context.Context, volName string, subvolName string, groupName string) error {
	endpoint := c.endpoint.JoinPath("/api/cephfs/subvolume", volName)
	query := url.Values{}
	query.Add("subvol_name", subvolName)
	if groupName != "" {
		query.Add("group_name", groupName)
	}
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
		if isCephFSNotFoundError(httpResp.StatusCode, body) {
			return ErrNotFound
		}
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}
