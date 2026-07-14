package restapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-block-image--image_spec>

type RBDImage struct {
	Name            string                 `json:"name"`
	PoolName        string                 `json:"pool_name"`
	Namespace       string                 `json:"namespace"`
	ID              string                 `json:"id"`
	Size            int64                  `json:"size"`
	ObjSize         int64                  `json:"obj_size"`
	NumObjs         int64                  `json:"num_objs"`
	BlockNamePrefix string                 `json:"block_name_prefix"`
	StripeUnit      int64                  `json:"stripe_unit"`
	StripeCount     int64                  `json:"stripe_count"`
	FeaturesName    []string               `json:"features_name"`
	DataPool        *string                `json:"data_pool"`
	Snapshots       []RBDImageSnapshot     `json:"snapshots"`
	Configuration   []RBDImageConfigOption `json:"configuration"`
	Metadata        map[string]string      `json:"metadata"`
}

// Source indicates where the effective value comes from: 0 global
// config, 1 pool override, 2 image override.
type RBDImageConfigOption struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Source int    `json:"source"`
}

type RBDImageSnapshot struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	// null for mirror snapshots, which this provider does not manage.
	IsProtected *bool  `json:"is_protected"`
	Timestamp   string `json:"timestamp"`
}

func (c *Client) rbdImageURL(poolName, namespace, imageName string) *url.URL {
	spec := poolName + "/" + imageName
	if namespace != "" {
		spec = poolName + "/" + namespace + "/" + imageName
	}
	return c.endpoint.JoinPath("/api/block/image", url.PathEscape(spec))
}

func (c *Client) GetRBDImage(ctx context.Context, poolName, namespace, imageName string) (*RBDImage, error) {
	endpoint := c.rbdImageURL(poolName, namespace, imageName)
	endpoint.RawQuery = "omit_usage=true"

	httpReq, err := http.NewRequestWithContext(ctx, "GET", endpoint.String(), nil)
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

	var image RBDImage
	err = json.Unmarshal(body, &image)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &image, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-block-image>

type RBDImageCreateRequest struct {
	Name          string             `json:"name"`
	PoolName      string             `json:"pool_name"`
	Namespace     *string            `json:"namespace,omitempty"`
	Size          int64              `json:"size"`
	ObjSize       *int64             `json:"obj_size,omitempty"`
	StripeUnit    *int64             `json:"stripe_unit,omitempty"`
	StripeCount   *int64             `json:"stripe_count,omitempty"`
	Features      []string           `json:"features"`
	DataPool      *string            `json:"data_pool,omitempty"`
	Configuration map[string]*string `json:"configuration,omitempty"`
	Metadata      map[string]*string `json:"metadata,omitempty"`
}

func (c *Client) CreateRBDImage(ctx context.Context, req RBDImageCreateRequest) (*TaskInfo, error) {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/block/image").String()
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

	if httpResp.StatusCode == http.StatusAccepted {
		var taskInfo TaskInfo
		err = json.Unmarshal(body, &taskInfo)
		if err != nil {
			return nil, fmt.Errorf("unable to decode task response: %w", err)
		}
		tflog.Debug(ctx, "RBD image creation returned 202, task is running", map[string]any{
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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#put--api-block-image--image_spec>

type RBDImageUpdateRequest struct {
	Name     *string  `json:"name,omitempty"`
	Size     *int64   `json:"size,omitempty"`
	Features []string `json:"features"`
	// The server applies these additively: present keys are set, keys
	// with a null value are removed, absent keys are left untouched.
	Configuration map[string]*string `json:"configuration,omitempty"`
	Metadata      map[string]*string `json:"metadata,omitempty"`
}

func (c *Client) UpdateRBDImage(ctx context.Context, poolName, namespace, imageName string, req RBDImageUpdateRequest) (*TaskInfo, error) {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.rbdImageURL(poolName, namespace, imageName).String()
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

	if httpResp.StatusCode == http.StatusAccepted {
		var taskInfo TaskInfo
		err = json.Unmarshal(body, &taskInfo)
		if err != nil {
			return nil, fmt.Errorf("unable to decode task response: %w", err)
		}
		tflog.Debug(ctx, "RBD image update returned 202, task is running", map[string]any{
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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-block-image--image_spec>

func (c *Client) DeleteRBDImage(ctx context.Context, poolName, namespace, imageName string) (*TaskInfo, error) {
	url := c.rbdImageURL(poolName, namespace, imageName).String()
	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
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
		tflog.Debug(ctx, "RBD image deletion returned 202, task is running", map[string]any{
			"task_name": taskInfo.Name,
			"metadata":  taskInfo.Metadata,
		})
		return &taskInfo, nil
	}

	return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
}
