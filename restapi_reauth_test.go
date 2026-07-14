package main

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

func TestAccRestAPIClient_reauthAfterJWTExpiry(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	endpoint, err := url.Parse(testDashboardURL)
	if err != nil {
		t.Fatalf("Failed to parse test dashboard URL: %v", err)
	}

	// The dashboard only mints its JWT secret on first use, so log in once
	// before reading it.
	bootstrap := &restapi.Client{}
	if err := bootstrap.Configure(t.Context(), []*url.URL{endpoint}, "admin", "password", "", "", "", time.Hour); err != nil {
		t.Fatalf("Failed to configure bootstrap client: %v", err)
	}

	jwtSecret, err := cephTestClusterCLI.ConfigKeyGet(t.Context(), "mgr/dashboard/jwt_secret")
	if err != nil {
		t.Fatalf("Failed to get JWT secret: %v", err)
	}

	client := &restapi.Client{}
	if err := client.Configure(t.Context(), []*url.URL{endpoint}, "", "", "", jwtSecret, "admin", 1*time.Second); err != nil {
		t.Fatalf("Failed to configure client: %v", err)
	}

	time.Sleep(3 * time.Second)

	if _, err := client.ListPools(t.Context()); err != nil {
		t.Fatalf("Expected request after token expiry to re-authenticate, got: %v", err)
	}
}

func TestAccRestAPIClient_reauthAfterPasswordTokenExpiry(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	endpoint, err := url.Parse(testDashboardURL)
	if err != nil {
		t.Fatalf("Failed to parse test dashboard URL: %v", err)
	}

	if err := cephTestClusterCLI.ConfigSet(t.Context(), "mgr", "mgr/dashboard/jwt_token_ttl", "2"); err != nil {
		t.Fatalf("Failed to set jwt_token_ttl: %v", err)
	}
	testCleanup(t, func(ctx context.Context) {
		if err := cephTestClusterCLI.ConfigRemove(ctx, "mgr", "mgr/dashboard/jwt_token_ttl"); err != nil {
			t.Logf("Warning: failed to reset jwt_token_ttl: %v", err)
		}
	})

	client := &restapi.Client{}
	if err := client.Configure(t.Context(), []*url.URL{endpoint}, "admin", "password", "", "", "", time.Hour); err != nil {
		t.Fatalf("Failed to configure client: %v", err)
	}

	time.Sleep(4 * time.Second)

	if _, err := client.ListPools(t.Context()); err != nil {
		t.Fatalf("Expected request after token expiry to re-authenticate, got: %v", err)
	}
}
