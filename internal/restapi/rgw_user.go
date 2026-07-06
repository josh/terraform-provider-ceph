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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-rgw-user-ratelimit>

type RGWS3Key struct {
	User       string `json:"user"`
	AccessKey  string `json:"access_key"`
	SecretKey  string `json:"secret_key"`
	Active     bool   `json:"active"`
	CreateDate string `json:"create_date"`
}

type RGWSwiftKey struct {
	User       string `json:"user"`
	SecretKey  string `json:"secret_key"`
	Active     bool   `json:"active"`
	CreateDate string `json:"create_date"`
}

type RGWSubuser struct {
	ID          string `json:"id"`
	Permissions string `json:"permissions"`
}

type RGWUser struct {
	Tenant      string        `json:"tenant"`
	UserID      string        `json:"user_id"`
	DisplayName string        `json:"display_name"`
	Email       string        `json:"email"`
	Suspended   int           `json:"suspended"`
	MaxBuckets  int           `json:"max_buckets"`
	Subusers    []RGWSubuser  `json:"subusers"`
	Keys        []RGWS3Key    `json:"keys"`
	SwiftKeys   []RGWSwiftKey `json:"swift_keys"`
	System      bool          `json:"system"`
	Admin       bool          `json:"admin"`
}

func (c *Client) RGWGetUser(ctx context.Context, uid string) (*RGWUser, error) {
	url := c.endpoint.JoinPath("/api/rgw/user", uid).String()

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

	if httpResp.StatusCode != http.StatusOK {
		tflog.Trace(ctx, "Ceph API response body", map[string]any{
			"response_body": string(body),
			"status_code":   httpResp.StatusCode,
		})
		if dashboardErr, err := parseDashboardError(body); err == nil {
			if rgwErr, ok := dashboardErr.RGWError(); ok {
				if rgwErr.Code == "NoSuchUser" {
					return nil, ErrNotFound
				}
			}
		}
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	var user RGWUser
	err = json.Unmarshal(body, &user)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	for _, key := range user.Keys {
		ctx = tflog.MaskLogStrings(ctx, key.AccessKey, key.SecretKey)
	}
	for _, key := range user.SwiftKeys {
		ctx = tflog.MaskLogStrings(ctx, key.SecretKey)
	}

	tflog.Trace(ctx, "Ceph API response body", map[string]any{
		"response_body": string(body),
		"status_code":   httpResp.StatusCode,
	})

	return &user, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-rgw-user>

type RGWUserCreateRequest struct {
	UID         string  `json:"uid"`
	DisplayName string  `json:"display_name"`
	Email       *string `json:"email,omitempty"`
	MaxBuckets  *int    `json:"max_buckets,omitempty"`
	Suspended   *int    `json:"suspended,omitempty"`
	System      *bool   `json:"system,omitempty"`
	GenerateKey bool    `json:"generate_key"`
}

func (c *Client) RGWCreateUser(ctx context.Context, req RGWUserCreateRequest) (*RGWUser, error) {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/rgw/user").String()
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

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated {
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

	var user RGWUser
	err = json.Unmarshal(body, &user)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &user, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#put--api-rgw-user-uid>

type RGWUserUpdateRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Email       *string `json:"email,omitempty"`
	MaxBuckets  *int    `json:"max_buckets,omitempty"`
	Suspended   *int    `json:"suspended,omitempty"`
	System      *bool   `json:"system,omitempty"`
}

func (c *Client) RGWUpdateUser(ctx context.Context, uid string, req RGWUserUpdateRequest) (*RGWUser, error) {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/rgw/user", uid).String()
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

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted {
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

	var user RGWUser
	err = json.Unmarshal(body, &user)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &user, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-rgw-user-uid>

func (c *Client) RGWDeleteUser(ctx context.Context, uid string) error {
	url := c.endpoint.JoinPath("/api/rgw/user", uid).String()
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
				if rgwErr.Code == "NoSuchUser" {
					return ErrNotFound
				}
			}
		}
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-rgw-user-uid-key>

type rgwS3KeyCreateRequest struct {
	UID         string  `json:"uid"`
	KeyType     string  `json:"key_type"`
	SubUser     *string `json:"subuser,omitempty"`
	AccessKey   *string `json:"access_key,omitempty"`
	SecretKey   *string `json:"secret_key,omitempty"`
	GenerateKey *bool   `json:"generate_key,omitempty"`
}

func (c *Client) RGWCreateS3Key(ctx context.Context, uid string, subuser *string, accessKey *string, secretKey *string, generateKey *bool) ([]RGWS3Key, error) {
	if accessKey != nil {
		ctx = tflog.MaskLogStrings(ctx, *accessKey)
	}
	if secretKey != nil {
		ctx = tflog.MaskLogStrings(ctx, *secretKey)
	}

	payload := rgwS3KeyCreateRequest{
		UID:         uid,
		KeyType:     "s3",
		SubUser:     subuser,
		AccessKey:   accessKey,
		SecretKey:   secretKey,
		GenerateKey: generateKey,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/rgw/user", uid, "key").String()
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

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	var keys []RGWS3Key
	err = json.Unmarshal(body, &keys)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	for _, key := range keys {
		ctx = tflog.MaskLogStrings(ctx, key.AccessKey, key.SecretKey)
	}

	tflog.Trace(ctx, "Ceph API response body", map[string]any{
		"response_body": string(body),
		"status_code":   httpResp.StatusCode,
	})

	return keys, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-rgw-user-uid-key>

func (c *Client) RGWDeleteS3Key(ctx context.Context, uid string, accessKey string, subuser *string) error {
	ctx = tflog.MaskLogStrings(ctx, accessKey)

	endpoint := c.endpoint.JoinPath("/api/rgw/user", uid, "key")
	query := url.Values{}
	query.Add("key_type", "s3")
	query.Add("access_key", accessKey)
	if subuser != nil {
		query.Add("subuser", *subuser)
	}
	endpoint.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", endpoint.String(), nil)
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

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted && httpResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}
