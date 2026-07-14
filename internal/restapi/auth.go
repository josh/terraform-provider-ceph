package restapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

func signJWTToken(secret, username string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":      "ceph-dashboard",
		"jti":      uuid.New().String(),
		"iat":      now.Unix(),
		"exp":      now.Add(expiry).Unix(),
		"username": username,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func (c *Client) Configure(ctx context.Context, endpoints []*url.URL, username, password, token, jwtSecret, jwtUsername string, jwtExpiry time.Duration) error {
	endpoint, err := queryEndpoints(ctx, endpoints)
	if err != nil {
		return fmt.Errorf("unable to query endpoints: %w", err)
	}

	c.endpoint = endpoint
	tflog.Info(ctx, "Using ceph mgr endpoint", map[string]any{
		"endpoint": endpoint.String(),
	})

	if c.client == nil {
		c.client = &http.Client{}
	}
	if _, ok := c.client.Transport.(*refreshTransport); !ok {
		base := c.client.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		c.client.Transport = &refreshTransport{client: c, base: base}
	}

	if token != "" {
		c.setToken(token)

		valid, err := c.AuthCheck(ctx)
		if err != nil {
			return fmt.Errorf("failed to validate token: %w", err)
		} else if !valid {
			return fmt.Errorf("provided token is invalid or expired")
		}
	} else if jwtSecret != "" {
		signedToken, err := signJWTToken(jwtSecret, jwtUsername, jwtExpiry)
		if err != nil {
			return fmt.Errorf("failed to sign JWT token: %w", err)
		}
		c.setToken(signedToken)
		c.jwtSecret = jwtSecret
		c.jwtUsername = jwtUsername
		c.jwtExpiry = jwtExpiry

		valid, err := c.AuthCheck(ctx)
		if err != nil {
			return fmt.Errorf("failed to validate signed JWT token: %w", err)
		} else if !valid {
			return fmt.Errorf("signed JWT token is invalid - check jwt_secret and jwt_username")
		}
	} else if username != "" && password != "" {
		authToken, err := c.Auth(ctx, username, password)
		if err != nil {
			return fmt.Errorf("failed to authenticate with credentials: %w", err)
		}

		c.setToken(authToken)
		c.username = username
		c.password = password
	} else {
		return fmt.Errorf("either token, jwt_secret, or username/password must be provided")
	}

	return nil
}

// refreshToken obtains a replacement for a token the API rejected. Requests
// racing on the same expiry refresh once: the first caller re-authenticates
// and later callers reuse its token.
func (c *Client) refreshToken(ctx context.Context, staleToken string) (string, error) {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	if current := c.currentToken(); current != staleToken {
		return current, nil
	}

	switch {
	case c.jwtSecret != "":
		token, err := signJWTToken(c.jwtSecret, c.jwtUsername, c.jwtExpiry)
		if err != nil {
			return "", fmt.Errorf("failed to sign JWT token: %w", err)
		}
		c.setToken(token)
		return token, nil
	case c.username != "" && c.password != "":
		token, err := c.Auth(ctx, c.username, c.password)
		if err != nil {
			return "", err
		}
		c.setToken(token)
		return token, nil
	}

	return "", errors.New("statically configured token cannot be refreshed")
}

func queryEndpoints(ctx context.Context, endpoints []*url.URL) (*url.URL, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	for _, endpoint := range endpoints {
		httpReq, err := http.NewRequestWithContext(ctx, "GET", endpoint.String(), nil)
		if err != nil {
			continue
		}

		done := logAPIRequest(ctx, httpReq)
		httpResp, err := client.Do(httpReq)
		done(httpResp, err)
		if err != nil {
			continue
		}
		httpResp.Body.Close() //nolint:errcheck

		if httpResp.StatusCode == http.StatusServiceUnavailable {
			continue
		}

		if httpResp.StatusCode == http.StatusSeeOther {
			continue
		}

		return endpoint, nil
	}

	return nil, errors.New("no available endpoints found")
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-auth-check>

type authCheckResponse struct {
	Username string `json:"username"`
}

func (c *Client) AuthCheck(ctx context.Context) (bool, error) {
	token := c.currentToken()
	url := c.endpoint.JoinPath("/api/auth/check").String() + "?token=" + token
	ctx = tflog.MaskLogStrings(ctx, token)
	jsonPayload := []byte("{}")

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return false, fmt.Errorf("unable to create check request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")

	done := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	done(httpResp, err)
	if err != nil {
		return false, fmt.Errorf("unable to make check request: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	switch httpResp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		body, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return false, fmt.Errorf("unable to read check response: %w", err)
		}

		tflog.Trace(ctx, "Ceph API response body", map[string]any{
			"response_body": string(body),
			"status_code":   httpResp.StatusCode,
		})

		var checkResp authCheckResponse
		if err := json.Unmarshal(body, &checkResp); err != nil {
			return false, fmt.Errorf("unable to decode check response: %w", err)
		}
		return checkResp.Username != "", nil
	case http.StatusUnauthorized:
		return false, fmt.Errorf("token is invalid or expired")
	default:
		body, _ := io.ReadAll(httpResp.Body)
		return false, fmt.Errorf("unknown error [%d]: %s", httpResp.StatusCode, string(body))
	}
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-auth>

type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string `json:"token"`
}

func (c *Client) Auth(ctx context.Context, username string, password string) (string, error) {
	ctx = tflog.MaskLogStrings(ctx, password)

	requestBody := authRequest{
		Username: username,
		Password: password,
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("unable to encode authentication request: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/auth").String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("unable to create authentication request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")

	done := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	done(httpResp, err)
	if err != nil {
		return "", fmt.Errorf("unable to make authentication request: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(httpResp.Body)
		return "", fmt.Errorf("authentication failed with status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read authentication response: %w", err)
	}

	var authResp authResponse
	err = json.Unmarshal(body, &authResp)
	if err != nil {
		return "", fmt.Errorf("unable to decode authentication response: %w", err)
	}

	if authResp.Token == "" {
		return "", fmt.Errorf("authentication response did not contain a token")
	}

	ctx = tflog.MaskLogStrings(ctx, authResp.Token)

	tflog.Trace(ctx, "Ceph API response body", map[string]any{
		"response_body": string(body),
		"status_code":   httpResp.StatusCode,
	})

	return authResp.Token, nil
}
