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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-erasure_code_profile>

type ErasureCodeProfile struct {
	Name                      string `json:"name"`
	K                         int    `json:"k"`
	M                         int    `json:"m"`
	Plugin                    string `json:"plugin"`
	CrushFailureDomain        string `json:"crush-failure-domain"`
	CrushNumFailureDomains    string `json:"crush-num-failure-domains,omitempty"`
	CrushOSDsPerFailureDomain string `json:"crush-osds-per-failure-domain,omitempty"`
	Technique                 string `json:"technique,omitempty"`
	CrushRoot                 string `json:"crush-root,omitempty"`
	CrushDeviceClass          string `json:"crush-device-class,omitempty"`
	Directory                 string `json:"directory,omitempty"`
	// Stored as strings; the dashboard only int-converts k and m.
	Packetsize string `json:"packetsize,omitempty"`
	W          string `json:"w,omitempty"`
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-erasure_code_profile>

type ErasureCodeProfileCreateRequest struct {
	Name                      string  `json:"name"`
	K                         *string `json:"k,omitempty"`
	M                         *string `json:"m,omitempty"`
	Plugin                    *string `json:"plugin,omitempty"`
	CrushFailureDomain        *string `json:"crush-failure-domain,omitempty"`
	CrushNumFailureDomains    *string `json:"crush-num-failure-domains,omitempty"`
	CrushOSDsPerFailureDomain *string `json:"crush-osds-per-failure-domain,omitempty"`
	Technique                 *string `json:"technique,omitempty"`
	CrushRoot                 *string `json:"crush-root,omitempty"`
	CrushDeviceClass          *string `json:"crush-device-class,omitempty"`
	Directory                 *string `json:"directory,omitempty"`
	Packetsize                *string `json:"packetsize,omitempty"`
	W                         *string `json:"w,omitempty"`
}

func (c *Client) CreateErasureCodeProfile(ctx context.Context, req ErasureCodeProfileCreateRequest) error {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/erasure_code_profile").String()
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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-erasure_code_profile--name>

func (c *Client) DeleteErasureCodeProfile(ctx context.Context, name string) error {
	url := c.endpoint.JoinPath("/api/erasure_code_profile", name).String()
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

	if httpResp.StatusCode != http.StatusAccepted && httpResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-erasure_code_profile--name>

func (c *Client) GetErasureCodeProfile(ctx context.Context, name string) (*ErasureCodeProfile, error) {
	url := c.endpoint.JoinPath("/api/erasure_code_profile", name).String()

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

	var profile ErasureCodeProfile
	err = json.Unmarshal(body, &profile)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &profile, nil
}
