package restapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

type RGWRole struct {
	RoleID                   string `json:"RoleId"`
	RoleName                 string `json:"RoleName"`
	Path                     string `json:"Path"`
	Arn                      string `json:"Arn"`
	CreateDate               string `json:"CreateDate"`
	MaxSessionDuration       int64  `json:"MaxSessionDuration"`
	AssumeRolePolicyDocument string `json:"AssumeRolePolicyDocument"`
	AccountID                string `json:"AccountId"`
	// Read-only fields with no management endpoints.
	PermissionPolicies json.RawMessage `json:"PermissionPolicies"`
	Description        string          `json:"Description"`
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-rgw-roles>

func (c *Client) RGWListRoles(ctx context.Context) ([]RGWRole, error) {
	url := c.endpoint.JoinPath("/api/rgw/roles").String()

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

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	tflog.Trace(ctx, "Ceph API response body", map[string]any{
		"response_body": string(body),
		"status_code":   httpResp.StatusCode,
	})

	var roles []RGWRole
	if err := json.Unmarshal(body, &roles); err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return roles, nil
}

func (c *Client) RGWGetRole(ctx context.Context, name string) (*RGWRole, error) {
	roles, err := c.RGWListRoles(ctx)
	if err != nil {
		return nil, err
	}

	for _, role := range roles {
		if role.RoleName == name {
			return &role, nil
		}
	}

	return nil, ErrNotFound
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-rgw-roles>

type RGWRoleCreateRequest struct {
	RoleName            string `json:"role_name"`
	RolePath            string `json:"role_path"`
	RoleAssumePolicyDoc string `json:"role_assume_policy_doc"`
}

func (c *Client) RGWCreateRole(ctx context.Context, req RGWRoleCreateRequest) error {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/rgw/roles").String()
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

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#put--api-rgw-roles>

type rgwRoleUpdateRequest struct {
	RoleName           string  `json:"role_name"`
	MaxSessionDuration float64 `json:"max_session_duration"`
}

func (c *Client) RGWUpdateRole(ctx context.Context, name string, maxSessionDuration int64) error {
	// The edit endpoint expects max_session_duration in hours and truncates
	// int(hours * 3600) back into seconds, unlike the read endpoint which
	// reports seconds. Plain division loses a second for many values (e.g.
	// 3603/3600.0*3600 truncates to 3602), so nudge the quotient up until it
	// round-trips exactly.
	hours := float64(maxSessionDuration) / 3600.0
	for int64(hours*3600.0) < maxSessionDuration {
		hours = math.Nextafter(hours, math.Inf(1))
	}
	requestBody := rgwRoleUpdateRequest{
		RoleName:           name,
		MaxSessionDuration: hours,
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/rgw/roles").String()
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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-rgw-roles-role_name>

func (c *Client) RGWDeleteRole(ctx context.Context, name string) error {
	url := c.endpoint.JoinPath("/api/rgw/roles", name).String()
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
		if dashboardErr, err := parseDashboardError(body); err == nil {
			if rgwErr, ok := dashboardErr.RGWError(); ok {
				if rgwErr.Code == "NoSuchEntity" {
					return ErrNotFound
				}
			}
		}
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}
