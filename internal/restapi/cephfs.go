package restapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

type CephFS struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	DataPoolIDs    []int  `json:"data_pool_ids"`
	MetadataPoolID int    `json:"metadata_pool_id"`
}

type CephFSListEntry struct {
	ID     int          `json:"id"`
	MdsMap CephFSMdsMap `json:"mdsmap"`
}

type CephFSMdsMap struct {
	FSName       string `json:"fs_name"`
	MetadataPool int    `json:"metadata_pool"`
	DataPools    []int  `json:"data_pools"`
}

type CephFSCreateRequest struct {
	Name        string                 `json:"name"`
	ServiceSpec map[string]interface{} `json:"service_spec"`
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-cephfs>

func (c *Client) CephFSList(ctx context.Context) ([]CephFS, error) {
	url := c.endpoint.JoinPath("/api/cephfs").String()

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

	var listEntries []CephFSListEntry
	err = json.Unmarshal(body, &listEntries)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	filesystems := make([]CephFS, len(listEntries))
	for i, entry := range listEntries {
		filesystems[i] = CephFS{
			ID:             entry.ID,
			Name:           entry.MdsMap.FSName,
			MetadataPoolID: entry.MdsMap.MetadataPool,
			DataPoolIDs:    entry.MdsMap.DataPools,
		}
	}

	return filesystems, nil
}

func (c *Client) CephFSGetByName(ctx context.Context, name string) (*CephFS, error) {
	filesystems, err := c.CephFSList(ctx)
	if err != nil {
		return nil, err
	}

	for _, fs := range filesystems {
		if fs.Name == name {
			return &fs, nil
		}
	}

	return nil, ErrNotFound
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-cephfs>

func (c *Client) CephFSCreate(ctx context.Context, req CephFSCreateRequest) error {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/cephfs").String()
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

	if httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

func (c *Client) CephFSDelete(ctx context.Context, name string) error {
	url := c.endpoint.JoinPath("/api/cephfs/remove", name).String()

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
		if isCephFSNotFoundError(httpResp.StatusCode, body) {
			return ErrNotFound
		}
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// The dashboard's cephfs controllers raise DashboardException (HTTP 400)
// rather than returning 404 when a volume, subvolume, or group is missing;
// the mgr/volumes error text always contains "does not exist".
func isCephFSNotFoundError(statusCode int, body []byte) bool {
	if statusCode != http.StatusBadRequest {
		return false
	}
	dashboardErr, err := parseDashboardError(body)
	if err != nil {
		return false
	}
	return strings.Contains(dashboardErr.Detail, "does not exist")
}
