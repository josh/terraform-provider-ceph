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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-role--name->

type DashboardRole struct {
	Name              string              `json:"name"`
	Description       *string             `json:"description"`
	ScopesPermissions map[string][]string `json:"scopes_permissions"`
	System            bool                `json:"system"`
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-role>

type DashboardRoleCreateRequest struct {
	Name              string              `json:"name"`
	Description       *string             `json:"description"`
	ScopesPermissions map[string][]string `json:"scopes_permissions"`
}

// The set endpoint replaces the description and all scope permissions
// rather than patching, so requests must always carry both values.

type DashboardRoleUpdateRequest struct {
	Description       *string             `json:"description"`
	ScopesPermissions map[string][]string `json:"scopes_permissions"`
}

func (c *Client) GetDashboardRole(ctx context.Context, name string) (*DashboardRole, error) {
	url := c.endpoint.JoinPath("/api/role", name).String()

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

	var role DashboardRole
	err = json.Unmarshal(body, &role)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &role, nil
}

func (c *Client) CreateDashboardRole(ctx context.Context, req DashboardRoleCreateRequest) (*DashboardRole, error) {
	if req.ScopesPermissions == nil {
		req.ScopesPermissions = map[string][]string{}
	}

	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/role").String()
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

	if httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	tflog.Trace(ctx, "Ceph API response body", map[string]any{
		"response_body": string(body),
		"status_code":   httpResp.StatusCode,
	})

	var role DashboardRole
	err = json.Unmarshal(body, &role)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &role, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#put--api-role--name->

func (c *Client) UpdateDashboardRole(ctx context.Context, name string, req DashboardRoleUpdateRequest) (*DashboardRole, error) {
	if req.ScopesPermissions == nil {
		req.ScopesPermissions = map[string][]string{}
	}

	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/role", name).String()
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

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	tflog.Trace(ctx, "Ceph API response body", map[string]any{
		"response_body": string(body),
		"status_code":   httpResp.StatusCode,
	})

	var role DashboardRole
	err = json.Unmarshal(body, &role)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &role, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-role--name->

func (c *Client) DeleteDashboardRole(ctx context.Context, name string) error {
	url := c.endpoint.JoinPath("/api/role", name).String()
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
