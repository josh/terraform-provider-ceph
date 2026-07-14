package restapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-cephfs-subvolume-snapshot--vol_name---subvol_name--info>

type SubvolumeSnapshotInfo struct {
	CreatedAt        string `json:"created_at"`
	DataPool         string `json:"data_pool"`
	HasPendingClones string `json:"has_pending_clones"`
}

type SubvolumeSnapshotCreateRequest struct {
	VolName    string `json:"vol_name"`
	SubvolName string `json:"subvol_name"`
	SnapName   string `json:"snap_name"`
	GroupName  string `json:"group_name,omitempty"`
}

func (c *Client) CephFSSubvolumeSnapshotInfo(ctx context.Context, volName, subvolName, snapName, groupName string) (*SubvolumeSnapshotInfo, error) {
	endpoint := c.endpoint.JoinPath("/api/cephfs/subvolume/snapshot", volName, subvolName, "info")
	query := url.Values{}
	query.Add("snap_name", snapName)
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
	c.setAuthHeader(httpReq)

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

	var info SubvolumeSnapshotInfo
	err = json.Unmarshal(body, &info)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &info, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-cephfs-subvolume-snapshot>

func (c *Client) CephFSSubvolumeSnapshotCreate(ctx context.Context, req SubvolumeSnapshotCreateRequest) error {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/cephfs/subvolume/snapshot").String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-cephfs-subvolume-snapshot--vol_name---subvol_name->

func (c *Client) CephFSSubvolumeSnapshotDelete(ctx context.Context, volName, subvolName, snapName, groupName string) error {
	endpoint := c.endpoint.JoinPath("/api/cephfs/subvolume/snapshot", volName, subvolName)
	query := url.Values{}
	query.Add("snap_name", snapName)
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
	c.setAuthHeader(httpReq)

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
