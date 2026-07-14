package main

import (
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

// Terraform applies the user and bucket quota resources of one RGW user in
// parallel; both PUTs modify the same user metadata object and RGW rejects
// the losing writer with 409 ConcurrentModification.
func TestAccRestAPIClient_concurrentQuotaUpdates(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-quota-race")
	createTestRGWUser(t, testUID, "Quota Race User")

	endpoint, err := url.Parse(testDashboardURL)
	if err != nil {
		t.Fatalf("Failed to parse test dashboard URL: %v", err)
	}

	client := &restapi.Client{}
	if err := client.Configure(t.Context(), []*url.URL{endpoint}, "admin", "password", "", "", "", time.Hour); err != nil {
		t.Fatalf("Failed to configure client: %v", err)
	}

	for i := range 20 {
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, quotaType := range []string{"user", "bucket"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := client.RGWSetUserQuota(t.Context(), testUID, quotaType, true, int64(1024+i), -1); err != nil {
					errs <- fmt.Errorf("%s quota (round %d): %w", quotaType, i, err)
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
	}
}
