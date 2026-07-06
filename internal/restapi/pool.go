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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-pool>

type PoolOptions struct {
	CompressionMode          string  `json:"compression_mode"`
	CompressionAlgorithm     string  `json:"compression_algorithm"`
	CompressionRequiredRatio float64 `json:"compression_required_ratio"`
	CompressionMinBlobSize   int     `json:"compression_min_blob_size"`
	CompressionMaxBlobSize   int     `json:"compression_max_blob_size"`
}

type Pool struct {
	PoolName            string      `json:"pool_name"`
	Type                string      `json:"type"`
	PoolID              int         `json:"pool_id"`
	Size                int         `json:"size"`
	MinSize             int         `json:"min_size"`
	PGNum               int         `json:"pg_num"`
	PGPlacementNum      int         `json:"pg_placement_num"`
	CrushRule           string      `json:"crush_rule"`
	ApplicationMetadata []string    `json:"application_metadata"`
	Flags               int         `json:"flags"`
	ErasureCodeProfile  string      `json:"erasure_code_profile"`
	PGAutoscaleMode     string      `json:"pg_autoscale_mode"`
	QuotaMaxObjects     int         `json:"quota_max_objects"`
	QuotaMaxBytes       int         `json:"quota_max_bytes"`
	Options             PoolOptions `json:"options"`
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-pool>

type PoolCreateRequest struct {
	Pool                     string   `json:"pool"`
	PoolType                 *string  `json:"pool_type,omitempty"`
	PgNum                    *int     `json:"pg_num,omitempty"`
	PgpNum                   *int     `json:"pgp_num,omitempty"`
	RuleName                 *string  `json:"rule_name,omitempty"`
	ErasureCodeProfile       *string  `json:"erasure_code_profile,omitempty"`
	ApplicationMetadata      []string `json:"application_metadata,omitempty"`
	Flags                    []string `json:"flags,omitempty"`
	MinSize                  *int     `json:"min_size,omitempty"`
	Size                     *int     `json:"size,omitempty"`
	PgAutoscaleMode          *string  `json:"pg_autoscale_mode,omitempty"`
	QuotaMaxObjects          *int     `json:"quota_max_objects,omitempty"`
	QuotaMaxBytes            *int     `json:"quota_max_bytes,omitempty"`
	CompressionMode          *string  `json:"compression_mode,omitempty"`
	CompressionAlgorithm     *string  `json:"compression_algorithm,omitempty"`
	CompressionRequiredRatio *float64 `json:"compression_required_ratio,omitempty"`
	CompressionMinBlobSize   *int     `json:"compression_min_blob_size,omitempty"`
	CompressionMaxBlobSize   *int     `json:"compression_max_blob_size,omitempty"`
}

func (c *Client) CreatePool(ctx context.Context, req PoolCreateRequest) (*TaskInfo, error) {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/pool").String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
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

	if httpResp.StatusCode == http.StatusAccepted {
		var taskInfo TaskInfo
		err = json.Unmarshal(body, &taskInfo)
		if err != nil {
			return nil, fmt.Errorf("unable to decode task response: %w", err)
		}
		tflog.Debug(ctx, "Pool creation returned 202, task is running", map[string]any{
			"task_name": taskInfo.Name,
			"metadata":  taskInfo.Metadata,
		})
		return &taskInfo, nil
	}

	if httpResp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-pool--pool_name>

func (c *Client) DeletePool(ctx context.Context, poolName string) (*TaskInfo, error) {
	url := c.endpoint.JoinPath("/api/pool", poolName).String()
	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
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

	if httpResp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	if httpResp.StatusCode == http.StatusAccepted {
		var taskInfo TaskInfo
		err = json.Unmarshal(body, &taskInfo)
		if err != nil {
			return nil, fmt.Errorf("unable to decode task response: %w", err)
		}
		tflog.Debug(ctx, "Pool deletion returned 202, task is running", map[string]any{
			"task_name": taskInfo.Name,
			"metadata":  taskInfo.Metadata,
		})
		return &taskInfo, nil
	}

	return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-pool--pool_name>

func (c *Client) GetPool(ctx context.Context, poolName string) (*Pool, error) {
	url := c.endpoint.JoinPath("/api/pool", poolName).String()

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

	var pool Pool
	err = json.Unmarshal(body, &pool)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &pool, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#put--api-pool--pool_name>

type PoolUpdateRequest struct {
	Pool                     *string  `json:"pool,omitempty"`
	PgNum                    *int     `json:"pg_num,omitempty"`
	PgpNum                   *int     `json:"pgp_num,omitempty"`
	CrushRule                *string  `json:"crush_rule,omitempty"`
	Size                     *int     `json:"size,omitempty"`
	MinSize                  *int     `json:"min_size,omitempty"`
	PgAutoscaleMode          *string  `json:"pg_autoscale_mode,omitempty"`
	QuotaMaxObjects          *int     `json:"quota_max_objects,omitempty"`
	QuotaMaxBytes            *int     `json:"quota_max_bytes,omitempty"`
	CompressionMode          *string  `json:"compression_mode,omitempty"`
	CompressionAlgorithm     *string  `json:"compression_algorithm,omitempty"`
	CompressionRequiredRatio *float64 `json:"compression_required_ratio,omitempty"`
	CompressionMinBlobSize   *int     `json:"compression_min_blob_size,omitempty"`
	CompressionMaxBlobSize   *int     `json:"compression_max_blob_size,omitempty"`
	ApplicationMetadata      []string `json:"application_metadata,omitempty"`
	Flags                    []string `json:"flags,omitempty"`
}

func (c *Client) UpdatePool(ctx context.Context, poolName string, req PoolUpdateRequest) (*TaskInfo, error) {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/pool", poolName).String()
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

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	if httpResp.StatusCode == http.StatusAccepted {
		var taskInfo TaskInfo
		err = json.Unmarshal(body, &taskInfo)
		if err != nil {
			return nil, fmt.Errorf("unable to decode task response: %w", err)
		}
		tflog.Debug(ctx, "Pool update returned 202, task is running", map[string]any{
			"task_name": taskInfo.Name,
			"metadata":  taskInfo.Metadata,
		})
		return &taskInfo, nil
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil, nil
}
