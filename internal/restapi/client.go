package restapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var ErrNotFound = errors.New("ceph API returned status 404: resource not found")

type dashboardError struct {
	Detail    string  `json:"detail"`
	Code      *string `json:"code"`
	Component *string `json:"component"`
	Status    *int    `json:"status"`
}

func parseDashboardError(body []byte) (*dashboardError, error) {
	var dashboardErr dashboardError
	if err := json.Unmarshal(body, &dashboardErr); err != nil {
		return nil, err
	}
	return &dashboardErr, nil
}

type rgwErrorResponse struct {
	Code      string `json:"Code"`
	RequestID string `json:"RequestId"`
	HostID    string `json:"HostId"`
}

var pythonByteStringJoinReplacer = strings.NewReplacer(
	"'\r\nb'", "",
	"'\nb'", "",
	"'\n", "'",
	"\r", "",
	"\n", "",
)

func (e *dashboardError) RGWError() (*rgwErrorResponse, bool) {
	if e.Component == nil || *e.Component != "rgw" {
		return nil, false
	}

	cleaned := pythonByteStringJoinReplacer.Replace(e.Detail)

	jsonStart := strings.Index(cleaned, "{")
	jsonEnd := strings.LastIndex(cleaned, "}")
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		return nil, false
	}

	var rgwErr rgwErrorResponse
	if err := json.Unmarshal([]byte(cleaned[jsonStart:jsonEnd+1]), &rgwErr); err != nil {
		return nil, false
	}

	return &rgwErr, true
}

type Client struct {
	endpoint *url.URL
	token    string
	client   *http.Client

	tokenMu   sync.RWMutex
	refreshMu sync.Mutex
	// Credentials retained from Configure so an expired token can be
	// refreshed; both empty when a static token was provided.
	username    string
	password    string
	jwtSecret   string
	jwtUsername string
	jwtExpiry   time.Duration
}

func (c *Client) currentToken() string {
	c.tokenMu.RLock()
	defer c.tokenMu.RUnlock()
	return c.token
}

func (c *Client) setToken(token string) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.token = token
}

func (c *Client) setAuthHeader(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.currentToken())
}

// refreshTransport retries a request once with a freshly obtained token
// when the Ceph API rejects its bearer token, e.g. after the token expired
// mid-apply or an active mgr failover invalidated it.
type refreshTransport struct {
	client *Client
	base   http.RoundTripper
}

func (t *refreshTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("Authorization") == "" {
		return t.base.RoundTrip(req)
	}

	token := t.client.currentToken()
	authed := req.Clone(req.Context())
	authed.Header.Set("Authorization", "Bearer "+token)

	resp, err := t.base.RoundTrip(authed)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}
	if req.Body != nil && req.GetBody == nil {
		return resp, nil
	}

	freshToken, refreshErr := t.client.refreshToken(req.Context(), token)
	if refreshErr != nil {
		tflog.Warn(req.Context(), "Unable to refresh Ceph API token after 401 response", map[string]any{
			"error": refreshErr.Error(),
		})
		return resp, nil
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close() //nolint:errcheck

	retry := req.Clone(req.Context())
	if req.GetBody != nil {
		body, bodyErr := req.GetBody()
		if bodyErr != nil {
			return nil, bodyErr
		}
		retry.Body = body
	}
	retry.Header.Set("Authorization", "Bearer "+freshToken)
	return t.base.RoundTrip(retry)
}

func logAPIRequest(ctx context.Context, req *http.Request) func(*http.Response, error) {
	startTime := time.Now()
	requestURL := req.URL.String()
	host := req.URL.Host
	path := req.URL.Path

	return func(resp *http.Response, err error) {
		duration := time.Since(startTime)
		fields := map[string]any{
			"method":      req.Method,
			"url":         requestURL,
			"host":        host,
			"path":        path,
			"duration_ms": duration.Milliseconds(),
		}

		if resp != nil {
			fields["status"] = resp.StatusCode
		}

		if err != nil {
			fields["error"] = err.Error()
			tflog.Error(ctx, "Ceph API request failed", fields)
			return
		}

		tflog.Info(ctx, "Ceph API request completed", fields)
	}
}

func (c *Client) Token() string {
	return c.currentToken()
}
