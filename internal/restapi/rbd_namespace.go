package restapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-block-pool--pool_name--namespace>

type RBDNamespace struct {
	Namespace string `json:"namespace"`
	NumImages int64  `json:"num_images"`
}

func (c *Client) ListRBDNamespaces(ctx context.Context, poolName string) ([]RBDNamespace, error) {
	url := c.endpoint.JoinPath("/api/block/pool", poolName, "namespace").String()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

	var namespaces []RBDNamespace
	err = json.Unmarshal(body, &namespaces)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return namespaces, nil
}

func (c *Client) GetRBDNamespace(ctx context.Context, poolName, namespace string) (*RBDNamespace, error) {
	namespaces, err := c.ListRBDNamespaces(ctx, poolName)
	if err != nil {
		return nil, err
	}

	for _, ns := range namespaces {
		if ns.Namespace == namespace {
			return &ns, nil
		}
	}

	return nil, ErrNotFound
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-block-pool--pool_name--namespace>

func (c *Client) CreateRBDNamespace(ctx context.Context, poolName, namespace string) error {
	jsonPayload, err := json.Marshal(map[string]string{"namespace": namespace})
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/block/pool", poolName, "namespace").String()
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

	if httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-block-pool--pool_name--namespace--namespace>

func (c *Client) DeleteRBDNamespace(ctx context.Context, poolName, namespace string) error {
	url := c.endpoint.JoinPath("/api/block/pool", poolName, "namespace", namespace).String()
	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
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

	if httpResp.StatusCode == http.StatusBadRequest {
		body, _ := io.ReadAll(httpResp.Body)
		// A missing namespace surfaces as 400 with the ENOENT errno as the
		// code, not as a 404.
		if dashboardErr, parseErr := parseDashboardError(body); parseErr == nil {
			if dashboardErr.Component != nil && *dashboardErr.Component == "rbd" &&
				dashboardErr.Code != nil && *dashboardErr.Code == "2" {
				return ErrNotFound
			}
		}
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	if httpResp.StatusCode != http.StatusAccepted && httpResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}
