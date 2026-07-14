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
	"github.com/josh/terraform-provider-ceph/internal/keyring"
)

// https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-cluster-user-export

type clusterUserExportRequest struct {
	Entities []string `json:"entities"`
}

func (c *Client) ClusterExportUser(ctx context.Context, entity string) (string, error) {
	requestBody := clusterUserExportRequest{
		Entities: []string{entity},
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/cluster/user/export").String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return "", fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read response body: %w", err)
	}

	if httpResp.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}

	if httpResp.StatusCode == http.StatusBadRequest {
		if dashboardErr, parseErr := parseDashboardError(body); parseErr == nil {
			if strings.Contains(dashboardErr.Detail, "no key for auth") {
				return "", ErrNotFound
			}
		}
		return "", fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	if httpResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	var keyringRaw string
	err = json.Unmarshal(body, &keyringRaw)
	if err != nil {
		return "", fmt.Errorf("unable to decode JSON response: %w", err)
	}

	users, err := keyring.Parse(keyringRaw)
	if err == nil {
		for _, user := range users {
			if user.Key != "" {
				ctx = tflog.MaskLogStrings(ctx, user.Key)
			}
		}
	}

	tflog.Trace(ctx, "Ceph API response body", map[string]any{
		"response_body": string(body),
		"status_code":   httpResp.StatusCode,
	})

	return keyringRaw, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-cluster-user>

type clusterUserCapability struct {
	Entity string `json:"entity"`
	Cap    string `json:"cap"`
}

type clusterUserCreateRequest struct {
	UserEntity   *string                 `json:"user_entity,omitempty"`
	Capabilities []clusterUserCapability `json:"capabilities,omitempty"`
	ImportData   *string                 `json:"import_data,omitempty"`
}

func clusterCapabilities(c keyring.Caps) []clusterUserCapability {
	capabilitySlice := make([]clusterUserCapability, 0, 4)

	if c.MDS != "" {
		capabilitySlice = append(capabilitySlice, clusterUserCapability{Entity: "mds", Cap: c.MDS})
	}

	if c.MGR != "" {
		capabilitySlice = append(capabilitySlice, clusterUserCapability{Entity: "mgr", Cap: c.MGR})
	}

	if c.MON != "" {
		capabilitySlice = append(capabilitySlice, clusterUserCapability{Entity: "mon", Cap: c.MON})
	}

	if c.OSD != "" {
		capabilitySlice = append(capabilitySlice, clusterUserCapability{Entity: "osd", Cap: c.OSD})
	}

	return capabilitySlice
}

func (c *Client) ClusterCreateUser(ctx context.Context, entity string, capabilities keyring.Caps) error {
	capabilitySlice := clusterCapabilities(capabilities)

	requestBody := clusterUserCreateRequest{}

	if entity != "" {
		requestBody.UserEntity = &entity
	}

	if len(capabilitySlice) > 0 {
		requestBody.Capabilities = capabilitySlice
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/cluster/user").String()
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

func (c *Client) ClusterImportUser(ctx context.Context, importData string) error {
	requestBody := clusterUserCreateRequest{}

	if importData != "" {
		requestBody.ImportData = &importData
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	if users, err := keyring.Parse(importData); err == nil {
		for _, user := range users {
			if user.Key != "" {
				ctx = tflog.MaskLogStrings(ctx, user.Key)
			}
		}
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/cluster/user").String()
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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#put--api-cluster-user>

type clusterUserUpdateRequest struct {
	UserEntity   string                  `json:"user_entity"`
	Capabilities []clusterUserCapability `json:"capabilities"`
}

func (c *Client) ClusterUpdateUser(ctx context.Context, entity string, capabilities keyring.Caps) error {
	capabilitySlice := clusterCapabilities(capabilities)

	requestBody := clusterUserUpdateRequest{
		UserEntity:   entity,
		Capabilities: capabilitySlice,
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/cluster/user").String()
	httpReq, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewBuffer(jsonPayload))
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

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-cluster-user-user_entities>

func (c *Client) ClusterDeleteUser(ctx context.Context, userEntities string) error {
	url := c.endpoint.JoinPath("/api/cluster/user", userEntities).String()
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

	if httpResp.StatusCode != http.StatusAccepted && httpResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}
