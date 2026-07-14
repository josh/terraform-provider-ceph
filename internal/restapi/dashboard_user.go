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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-user--username->

type DashboardUser struct {
	Username          string   `json:"username"`
	Name              *string  `json:"name"`
	Email             *string  `json:"email"`
	Roles             []string `json:"roles"`
	Enabled           bool     `json:"enabled"`
	PwdExpirationDate *int64   `json:"pwdExpirationDate"`
	PwdUpdateRequired bool     `json:"pwdUpdateRequired"`
	LastUpdate        int64    `json:"lastUpdate"`
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-user>

type DashboardUserCreateRequest struct {
	Username          string   `json:"username"`
	Password          string   `json:"password"`
	Name              *string  `json:"name"`
	Email             *string  `json:"email"`
	Roles             []string `json:"roles"`
	Enabled           bool     `json:"enabled"`
	PwdExpirationDate *int64   `json:"pwdExpirationDate"`
	PwdUpdateRequired bool     `json:"pwdUpdateRequired"`
}

// The set endpoint replaces every field rather than patching, so requests
// must always carry all values; only password is optional.

type DashboardUserUpdateRequest struct {
	Password          *string  `json:"password,omitempty"`
	Name              *string  `json:"name"`
	Email             *string  `json:"email"`
	Roles             []string `json:"roles"`
	Enabled           bool     `json:"enabled"`
	PwdExpirationDate *int64   `json:"pwdExpirationDate"`
	PwdUpdateRequired bool     `json:"pwdUpdateRequired"`
}

func (c *Client) GetDashboardUser(ctx context.Context, username string) (*DashboardUser, error) {
	url := c.endpoint.JoinPath("/api/user", username).String()

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

	var user DashboardUser
	err = json.Unmarshal(body, &user)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &user, nil
}

func (c *Client) CreateDashboardUser(ctx context.Context, req DashboardUserCreateRequest) (*DashboardUser, error) {
	if req.Roles == nil {
		req.Roles = []string{}
	}

	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/user").String()
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

	var user DashboardUser
	err = json.Unmarshal(body, &user)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &user, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#put--api-user--username->

func (c *Client) UpdateDashboardUser(ctx context.Context, username string, req DashboardUserUpdateRequest) (*DashboardUser, error) {
	if req.Roles == nil {
		req.Roles = []string{}
	}

	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/user", username).String()
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

	var user DashboardUser
	err = json.Unmarshal(body, &user)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &user, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-user--username->

func (c *Client) DeleteDashboardUser(ctx context.Context, username string) error {
	url := c.endpoint.JoinPath("/api/user", username).String()
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
