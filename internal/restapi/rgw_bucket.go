package restapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-rgw-bucket-bucket>

type RGWBucket struct {
	Bucket        string `json:"bucket"`
	Zonegroup     string `json:"zonegroup"`
	PlacementRule string `json:"placement_rule"`
	ID            string `json:"id"`
	Owner         string `json:"owner"`
	CreationTime  string `json:"creation_time"`
	ACL           string `json:"acl"`
	Bid           string `json:"bid"`
}

func (c *Client) RGWGetBucket(ctx context.Context, bucketName string) (*RGWBucket, error) {
	url := c.endpoint.JoinPath("/api/rgw/bucket", bucketName).String()

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

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	tflog.Trace(ctx, "Ceph API response body", map[string]any{
		"response_body": string(body),
		"status_code":   httpResp.StatusCode,
	})

	if httpResp.StatusCode != http.StatusOK {
		if dashboardErr, err := parseDashboardError(body); err == nil {
			if rgwErr, ok := dashboardErr.RGWError(); ok {
				if rgwErr.Code == "NoSuchBucket" {
					return nil, ErrNotFound
				}
			}
		}
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	var bucket RGWBucket
	err = json.Unmarshal(body, &bucket)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &bucket, nil
}

func (c *Client) RGWGetBucketWithRetry(ctx context.Context, bucketName string) (*RGWBucket, error) {
	var retryDelays = [...]time.Duration{
		500 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
	}
	url := c.endpoint.JoinPath("/api/rgw/bucket", bucketName).String()

	for attempt := 0; attempt < len(retryDelays); attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

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

		if httpResp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(httpResp.Body)
			httpResp.Body.Close() //nolint:errcheck
			if err != nil {
				return nil, fmt.Errorf("unable to read response body: %w", err)
			}

			tflog.Trace(ctx, "Ceph API response body", map[string]any{
				"response_body": string(body),
				"status_code":   httpResp.StatusCode,
			})

			var bucket RGWBucket
			err = json.Unmarshal(body, &bucket)
			if err != nil {
				return nil, fmt.Errorf("unable to decode JSON response: %w", err)
			}

			return &bucket, nil
		}

		if httpResp.StatusCode == http.StatusNotFound {
			httpResp.Body.Close() //nolint:errcheck
			return nil, ErrNotFound
		}

		body, err := io.ReadAll(httpResp.Body)
		httpResp.Body.Close() //nolint:errcheck
		if err != nil {
			return nil, fmt.Errorf("unable to read response body: %w", err)
		}

		if dashboardErr, err := parseDashboardError(body); err == nil {
			if rgwErr, ok := dashboardErr.RGWError(); ok {
				if rgwErr.Code == "NoSuchBucket" {
					return nil, ErrNotFound
				}
			}
		}

		isRetryable := httpResp.StatusCode == 500

		if isRetryable && attempt < len(retryDelays)-1 {
			backoff := retryDelays[attempt]

			tflog.Debug(ctx, "Retrying RGW bucket GET due to server error", map[string]any{
				"bucket":     bucketName,
				"attempt":    attempt + 1,
				"status":     httpResp.StatusCode,
				"backoff_ms": backoff.Milliseconds(),
			})

			select {
			case <-time.After(backoff):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil, fmt.Errorf("max retries exceeded")
}

type RGWBucketCreateRequest struct {
	Bucket    string  `json:"bucket"`
	UID       string  `json:"uid"`
	Zonegroup *string `json:"zonegroup,omitempty"`
}

func (c *Client) RGWCreateBucket(ctx context.Context, req RGWBucketCreateRequest) (*RGWBucket, error) {
	url := c.endpoint.JoinPath("/api/rgw/bucket").String()

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal request: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(reqBody),
	})

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
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

	if httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusOK {
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

	var bucket RGWBucket
	err = json.Unmarshal(body, &bucket)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &bucket, nil
}

func (c *Client) RGWDeleteBucket(ctx context.Context, bucketName string) error {
	url := c.endpoint.JoinPath("/api/rgw/bucket", bucketName).String()

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

	if httpResp.StatusCode != http.StatusNoContent && httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		if dashboardErr, err := parseDashboardError(body); err == nil {
			if rgwErr, ok := dashboardErr.RGWError(); ok {
				if rgwErr.Code == "NoSuchBucket" {
					return ErrNotFound
				}
			}
		}
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}
