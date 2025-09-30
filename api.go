package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type CephAPIClient struct {
	endpoint string
	token    string
	client   *http.Client
}

func (c *CephAPIClient) Configure(ctx context.Context, username string, password string, token string) error {
	if c.endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}

	if !strings.HasSuffix(c.endpoint, "/api") {
		return fmt.Errorf("endpoint MUST end with '/api', got: %s", c.endpoint)
	}

	if c.client == nil {
		c.client = &http.Client{
			Timeout: 10 * time.Second,
		}
	}

	if token != "" {
		c.token = token

		valid, err := c.AuthCheck(ctx)
		if err != nil {
			return fmt.Errorf("failed to validate token: %w", err)
		} else if !valid {
			return fmt.Errorf("provided token is invalid or expired")
		}
	} else if username != "" && password != "" {
		authToken, err := c.Auth(ctx, username, password)
		if err != nil {
			return fmt.Errorf("failed to authenticate with credentials: %w", err)
		}

		c.token = authToken
	} else {
		return fmt.Errorf("either token or username/password must be provided")
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-auth-check>

func (c *CephAPIClient) AuthCheck(ctx context.Context) (bool, error) {
	url := c.endpoint + "/auth/check?token=" + c.token
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return false, fmt.Errorf("unable to create check request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return false, fmt.Errorf("unable to make check request: %w", err)
	}
	defer httpResp.Body.Close()

	switch httpResp.StatusCode {
	case http.StatusCreated, http.StatusAccepted:
		return true, nil
	case http.StatusUnauthorized:
		return false, fmt.Errorf("token is invalid or expired")
	default:
		body, _ := io.ReadAll(httpResp.Body)
		return false, fmt.Errorf("unknown error [%d]: %s", httpResp.StatusCode, string(body))
	}
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-auth>

type CephAPIAuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type CephAPIAuthResponse struct {
	Token string `json:"token"`
}

func (c *CephAPIClient) Auth(ctx context.Context, username string, password string) (string, error) {
	requestBody := CephAPIAuthRequest{
		Username: username,
		Password: password,
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("unable to encode authentication request: %w", err)
	}

	url := c.endpoint + "/auth"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("unable to create authentication request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("unable to make authentication request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(httpResp.Body)
		return "", fmt.Errorf("authentication failed with status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read authentication response: %w", err)
	}

	var authResp CephAPIAuthResponse
	err = json.Unmarshal(body, &authResp)
	if err != nil {
		return "", fmt.Errorf("unable to decode authentication response: %w", err)
	}

	if authResp.Token == "" {
		return "", fmt.Errorf("authentication response did not contain a token")
	}

	return authResp.Token, nil
}

// https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-cluster-user-export

type CephAPIClusterUserExportRequest struct {
	Entities []string `json:"entities"`
}

func (c *CephAPIClient) ClusterExportUser(ctx context.Context, entity string) (string, error) {
	requestBody := CephAPIClusterUserExportRequest{
		Entities: []string{entity},
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint + "/cluster/user/export"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ceph API returned status %d", httpResp.StatusCode)
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read response body: %w", err)
	}

	var keyringRaw string
	err = json.Unmarshal(body, &keyringRaw)
	if err != nil {
		return "", fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return keyringRaw, nil
}
