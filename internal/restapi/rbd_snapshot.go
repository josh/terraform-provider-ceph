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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-block-image--image_spec--snap>

func (c *Client) rbdSnapshotURL(poolName, namespace, imageName string, extra ...string) *url.URL {
	endpoint := c.rbdImageURL(poolName, namespace, imageName).JoinPath("snap")
	for _, segment := range extra {
		endpoint = endpoint.JoinPath(url.PathEscape(segment))
	}
	return endpoint
}

func (c *Client) GetRBDSnapshot(ctx context.Context, poolName, namespace, imageName, snapName string) (*RBDImageSnapshot, error) {
	image, err := c.GetRBDImage(ctx, poolName, namespace, imageName)
	if err != nil {
		return nil, err
	}

	for _, snap := range image.Snapshots {
		if snap.Name == snapName {
			return &snap, nil
		}
	}

	return nil, ErrNotFound
}

func (c *Client) CreateRBDSnapshot(ctx context.Context, poolName, namespace, imageName, snapName string) (*TaskInfo, error) {
	jsonPayload, err := json.Marshal(map[string]any{
		"snapshot_name":       snapName,
		"mirrorImageSnapshot": false,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.rbdSnapshotURL(poolName, namespace, imageName).String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
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

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	if httpResp.StatusCode == http.StatusAccepted {
		return decodeTaskInfo(ctx, body, "RBD snapshot creation")
	}

	if httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#put--api-block-image--image_spec--snap--snapshot_name->

type RBDSnapshotUpdateRequest struct {
	NewSnapName *string `json:"new_snap_name,omitempty"`
	IsProtected *bool   `json:"is_protected,omitempty"`
}

func (c *Client) UpdateRBDSnapshot(ctx context.Context, poolName, namespace, imageName, snapName string, req RBDSnapshotUpdateRequest) (*TaskInfo, error) {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.rbdSnapshotURL(poolName, namespace, imageName, snapName).String()
	httpReq, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewBuffer(jsonPayload))
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

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	if httpResp.StatusCode == http.StatusAccepted {
		return decodeTaskInfo(ctx, body, "RBD snapshot update")
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-block-image--image_spec--snap--snapshot_name->

func (c *Client) DeleteRBDSnapshot(ctx context.Context, poolName, namespace, imageName, snapName string) (*TaskInfo, error) {
	url := c.rbdSnapshotURL(poolName, namespace, imageName, snapName).String()
	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
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

	if httpResp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	if httpResp.StatusCode == http.StatusAccepted {
		return decodeTaskInfo(ctx, body, "RBD snapshot deletion")
	}

	if httpResp.StatusCode == http.StatusBadRequest {
		// A missing snapshot surfaces as 400 with the ENOENT errno as the
		// code, not as a 404.
		if dashboardErr, parseErr := parseDashboardError(body); parseErr == nil {
			if dashboardErr.Component != nil && *dashboardErr.Component == "rbd" &&
				dashboardErr.Code != nil && *dashboardErr.Code == "2" {
				return nil, ErrNotFound
			}
		}
	}

	return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
}
