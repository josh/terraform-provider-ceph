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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-block-mirroring-pool--pool_name->

type RBDMirroringPoolMode struct {
	MirrorMode string `json:"mirror_mode"`
}

func (c *Client) rbdMirroringPoolURL(poolName string) *url.URL {
	return c.endpoint.JoinPath("/api/block/mirroring/pool", url.PathEscape(poolName))
}

// A missing pool surfaces as 400 with the ENOENT errno as the code, not
// as a 404.
func isRBDMirroringNotFound(statusCode int, body []byte) bool {
	if statusCode != http.StatusBadRequest {
		return false
	}
	dashboardErr, err := parseDashboardError(body)
	if err != nil {
		return false
	}
	return dashboardErr.Component != nil && *dashboardErr.Component == "rbd-mirroring" &&
		dashboardErr.Code != nil && *dashboardErr.Code == "2"
}

func (c *Client) GetRBDMirroringPoolMode(ctx context.Context, poolName string) (*RBDMirroringPoolMode, error) {
	url := c.rbdMirroringPoolURL(poolName).String()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
		if isRBDMirroringNotFound(httpResp.StatusCode, body) {
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

	var mode RBDMirroringPoolMode
	err = json.Unmarshal(body, &mode)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &mode, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#put--api-block-mirroring-pool--pool_name->

func (c *Client) SetRBDMirroringPoolMode(ctx context.Context, poolName, mirrorMode string) (*TaskInfo, error) {
	jsonPayload, err := json.Marshal(map[string]string{"mirror_mode": mirrorMode})
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.rbdMirroringPoolURL(poolName).String()
	httpReq, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewBuffer(jsonPayload))
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

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	if httpResp.StatusCode == http.StatusAccepted {
		return decodeTaskInfo(ctx, body, "RBD mirroring pool mode update")
	}

	if httpResp.StatusCode != http.StatusOK {
		if isRBDMirroringNotFound(httpResp.StatusCode, body) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-block-mirroring-pool--pool_name--peer--peer_uuid->

type RBDMirroringPoolPeer struct {
	UUID        string `json:"uuid"`
	ClusterName string `json:"cluster_name"`
	ClientID    string `json:"client_id"`
	Direction   string `json:"direction"`
	MirrorUUID  string `json:"mirror_uuid"`
	MonHost     string `json:"mon_host"`
	Key         string `json:"key"`
}

type RBDMirroringPoolPeerSpec struct {
	ClusterName string  `json:"cluster_name"`
	ClientID    string  `json:"client_id"`
	MonHost     *string `json:"mon_host,omitempty"`
	Key         *string `json:"key,omitempty"`
}

// A missing peer surfaces as 400 with the rbd component and the ENOENT
// errno as the code; a missing pool uses the rbd-mirroring component.
func isRBDMirroringPeerNotFound(statusCode int, body []byte) bool {
	if statusCode != http.StatusBadRequest {
		return false
	}
	dashboardErr, err := parseDashboardError(body)
	if err != nil {
		return false
	}
	if dashboardErr.Component == nil || dashboardErr.Code == nil || *dashboardErr.Code != "2" {
		return false
	}
	return *dashboardErr.Component == "rbd" || *dashboardErr.Component == "rbd-mirroring"
}

func (c *Client) rbdMirroringPoolPeerURL(poolName string, segments ...string) *url.URL {
	endpoint := c.rbdMirroringPoolURL(poolName).JoinPath("peer")
	for _, segment := range segments {
		endpoint = endpoint.JoinPath(url.PathEscape(segment))
	}
	return endpoint
}

func (c *Client) CreateRBDMirroringPoolPeer(ctx context.Context, poolName string, spec RBDMirroringPoolPeerSpec) (string, error) {
	jsonPayload, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.rbdMirroringPoolPeerURL(poolName).String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return "", fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read response body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	tflog.Trace(ctx, "Ceph API response body", map[string]any{
		"response_body": string(body),
		"status_code":   httpResp.StatusCode,
	})

	var result struct {
		UUID string `json:"uuid"`
	}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return result.UUID, nil
}

func (c *Client) GetRBDMirroringPoolPeer(ctx context.Context, poolName, peerUUID string) (*RBDMirroringPoolPeer, error) {
	url := c.rbdMirroringPoolPeerURL(poolName, peerUUID).String()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
		if isRBDMirroringPeerNotFound(httpResp.StatusCode, body) {
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

	var peer RBDMirroringPoolPeer
	err = json.Unmarshal(body, &peer)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &peer, nil
}

func (c *Client) UpdateRBDMirroringPoolPeer(ctx context.Context, poolName, peerUUID string, spec RBDMirroringPoolPeerSpec) error {
	jsonPayload, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.rbdMirroringPoolPeerURL(poolName, peerUUID).String()
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
		if isRBDMirroringPeerNotFound(httpResp.StatusCode, body) {
			return ErrNotFound
		}
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// Deleting a missing peer succeeds; librbd tolerates the ENOENT.
func (c *Client) DeleteRBDMirroringPoolPeer(ctx context.Context, poolName, peerUUID string) error {
	url := c.rbdMirroringPoolPeerURL(poolName, peerUUID).String()
	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
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
		if isRBDMirroringPeerNotFound(httpResp.StatusCode, body) {
			return ErrNotFound
		}
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}
