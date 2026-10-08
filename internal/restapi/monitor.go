package restapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-monitor>

type MonitorCipher struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type Monitor struct {
	MonStatus struct {
		Monmap struct {
			AuthAllowedCiphers  []MonitorCipher `json:"auth_allowed_ciphers"`
			AuthPreferredCipher *MonitorCipher  `json:"auth_preferred_cipher"`
			AuthServiceCipher   *MonitorCipher  `json:"auth_service_cipher"`
		} `json:"monmap"`
	} `json:"mon_status"`
}

func (c *Client) GetMonitor(ctx context.Context) (*Monitor, error) {
	url := c.endpoint.JoinPath("/api/monitor").String()

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

	var monitor Monitor
	if err := json.Unmarshal(body, &monitor); err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &monitor, nil
}
