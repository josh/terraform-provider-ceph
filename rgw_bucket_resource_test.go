package main

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephRGWBucketResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-bucket-owner")
	testBucket := acctest.RandomWithPrefix("test-bucket")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWBucketDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Bucket Test User"
					}

					resource "ceph_rgw_s3_key" "test" {
					  user_id = ceph_rgw_user.test.user_id
					}

					resource "ceph_rgw_bucket" "test" {
					  bucket = %q
					  owner  = ceph_rgw_user.test.user_id
					  depends_on = [ceph_rgw_s3_key.test]
					}
				`, testUID, testBucket),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWBucketExists(t, testBucket),
					resource.TestCheckResourceAttr("ceph_rgw_bucket.test", "bucket", testBucket),
					resource.TestCheckResourceAttr("ceph_rgw_bucket.test", "owner", testUID),
					resource.TestCheckResourceAttrSet("ceph_rgw_bucket.test", "id"),
					resource.TestCheckResourceAttrSet("ceph_rgw_bucket.test", "creation_time"),
				),
			},
		},
	})
}

func TestAccCephRGWBucketResource_zonegroupReadOnly(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-bucket-zg-owner")
	testBucket := acctest.RandomWithPrefix("test-bucket-zg")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWBucketDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Bucket Zonegroup Test User"
					}

					resource "ceph_rgw_s3_key" "test" {
					  user_id = ceph_rgw_user.test.user_id
					}

					resource "ceph_rgw_bucket" "test" {
					  bucket    = %q
					  owner     = ceph_rgw_user.test.user_id
					  zonegroup = "bogus-zonegroup"
					  depends_on = [ceph_rgw_s3_key.test]
					}
				`, testUID, testBucket),
				ExpectError: regexp.MustCompile(`(?i)read-only`),
			},
		},
	})
}

func TestAccCephRGWBucketResourceImport(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-bucket-import-owner")
	testBucket := acctest.RandomWithPrefix("test-bucket-import")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWBucketDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Bucket Import Test User"
					}

					resource "ceph_rgw_s3_key" "test" {
					  user_id = ceph_rgw_user.test.user_id
					}

					resource "ceph_rgw_bucket" "test" {
					  bucket = %q
					  owner  = ceph_rgw_user.test.user_id
					  depends_on = [ceph_rgw_s3_key.test]
					}
				`, testUID, testBucket),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Bucket Import Test User"
					}

					resource "ceph_rgw_s3_key" "test" {
					  user_id = ceph_rgw_user.test.user_id
					}

					resource "ceph_rgw_bucket" "test" {
					  bucket = %q
					  owner  = ceph_rgw_user.test.user_id
					  depends_on = [ceph_rgw_s3_key.test]
					}
				`, testUID, testBucket),
				ResourceName:                         "ceph_rgw_bucket.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "bucket",
				ImportStateId:                        testBucket,
			},
		},
	})
}

func TestAccCephRGWBucketResourceImport_nonExistent(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testBucket := acctest.RandomWithPrefix("test-bucket-nonexist")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_bucket" "nonexistent" {
					  bucket = %q
					  owner  = "nonexistent-user"
					}
				`, testBucket),
				ResourceName:  "ceph_rgw_bucket.nonexistent",
				ImportState:   true,
				ImportStateId: testBucket,
				ExpectError:   regexp.MustCompile(`(?i)cannot import non-existent remote object`),
			},
		},
	})
}

func testAccCheckCephRGWBucketDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_rgw_bucket" {
				continue
			}

			bucketName := rs.Primary.Attributes["bucket"]

			_, err := cephTestClusterCLI.RgwBucketInfo(ctx, bucketName)
			if err == nil {
				return fmt.Errorf("ceph_rgw_bucket resource %s still exists", bucketName)
			}
		}
		return nil
	}
}

func checkCephRGWBucketExists(t *testing.T, bucketName string) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		bucket, err := cephTestClusterCLI.RgwBucketInfo(t.Context(), bucketName)
		if err != nil {
			return fmt.Errorf("RGW bucket %s does not exist: %w", bucketName, err)
		}

		t.Logf("Verified RGW bucket %s exists with owner: %s", bucketName, bucket.Owner)
		return nil
	}
}

func TestAccCephRGWBucketResource_OutOfBandDeletion(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-bucket-oob-owner")
	testBucket := acctest.RandomWithPrefix("test-bucket-oob")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWBucketDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Bucket OOB Delete Test User"
					}

					resource "ceph_rgw_s3_key" "test" {
					  user_id = ceph_rgw_user.test.user_id
					}

					resource "ceph_rgw_bucket" "test" {
					  bucket = %q
					  owner  = ceph_rgw_user.test.user_id
					  depends_on = [ceph_rgw_s3_key.test]
					}
				`, testUID, testBucket),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWBucketExists(t, testBucket),
					resource.TestCheckResourceAttr("ceph_rgw_bucket.test", "bucket", testBucket),
				),
			},
			{
				PreConfig: func() {
					err := cephTestClusterCLI.RgwBucketRemove(t.Context(), testBucket, true)
					if err != nil {
						t.Fatalf("Failed to delete bucket out of band: %v", err)
					}
					t.Logf("Deleted bucket %s out of band", testBucket)
				},
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Bucket OOB Delete Test User"
					}

					resource "ceph_rgw_s3_key" "test" {
					  user_id = ceph_rgw_user.test.user_id
					}

					resource "ceph_rgw_bucket" "test" {
					  bucket = %q
					  owner  = ceph_rgw_user.test.user_id
					  depends_on = [ceph_rgw_s3_key.test]
					}
				`, testUID, testBucket),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rgw_bucket.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWBucketExists(t, testBucket),
					resource.TestCheckResourceAttr("ceph_rgw_bucket.test", "bucket", testBucket),
				),
			},
		},
	})
}

func TestAccCephRGWBucketResource_OutOfBandDeletionDestroy(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-bucket-oob-destroy-owner")
	testBucket := acctest.RandomWithPrefix("test-bucket-oob-destroy")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWBucketDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Bucket OOB Destroy Test User"
					}

					resource "ceph_rgw_s3_key" "test" {
					  user_id = ceph_rgw_user.test.user_id
					}

					resource "ceph_rgw_bucket" "test" {
					  bucket = %q
					  owner  = ceph_rgw_user.test.user_id
					  depends_on = [ceph_rgw_s3_key.test]
					}
				`, testUID, testBucket),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWBucketExists(t, testBucket),
				),
			},
			{
				PreConfig: func() {
					err := cephTestClusterCLI.RgwBucketRemove(t.Context(), testBucket, true)
					if err != nil {
						t.Fatalf("Failed to delete bucket out of band: %v", err)
					}
					t.Logf("Deleted bucket %s out of band", testBucket)
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccProviderConfigBlock,
			},
		},
	})
}

func TestAccCephRGWBucketResource_OwnerChange(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	ownerA := acctest.RandomWithPrefix("test-bucket-owner-a")
	ownerB := acctest.RandomWithPrefix("test-bucket-owner-b")
	testBucket := acctest.RandomWithPrefix("test-bucket-chown")

	config := func(owner string) string {
		return testAccProviderConfigBlock + fmt.Sprintf(`
			resource "ceph_rgw_user" "a" {
			  user_id      = %q
			  display_name = "Owner A"
			}

			resource "ceph_rgw_s3_key" "a" {
			  user_id = ceph_rgw_user.a.user_id
			}

			resource "ceph_rgw_user" "b" {
			  user_id      = %q
			  display_name = "Owner B"
			}

			resource "ceph_rgw_s3_key" "b" {
			  user_id = ceph_rgw_user.b.user_id
			}

			resource "ceph_rgw_bucket" "test" {
			  bucket     = %q
			  owner      = %q
			  depends_on = [ceph_rgw_s3_key.a, ceph_rgw_s3_key.b]
			}
		`, ownerA, ownerB, testBucket, owner)
	}

	idStable := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWBucketDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config(ownerA),
				ConfigStateChecks: []statecheck.StateCheck{
					idStable.AddStateValue("ceph_rgw_bucket.test", tfjsonpath.New("id")),
				},
				Check: checkCephRGWBucketOwner(t, testBucket, ownerA),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config(ownerB),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rgw_bucket.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					idStable.AddStateValue("ceph_rgw_bucket.test", tfjsonpath.New("id")),
				},
				Check: checkCephRGWBucketOwner(t, testBucket, ownerB),
			},
		},
	})
}

func TestAccCephRGWBucketResource_Versioning(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-bucket-vers-owner")
	testBucket := acctest.RandomWithPrefix("test-bucket-vers")

	config := func(state string) string {
		return testAccProviderConfigBlock + fmt.Sprintf(`
			resource "ceph_rgw_user" "test" {
			  user_id      = %q
			  display_name = "Versioning Test User"
			}

			resource "ceph_rgw_s3_key" "test" {
			  user_id = ceph_rgw_user.test.user_id
			}

			resource "ceph_rgw_bucket" "test" {
			  bucket           = %q
			  owner            = ceph_rgw_user.test.user_id
			  versioning_state = %q
			  depends_on       = [ceph_rgw_s3_key.test]
			}
		`, testUID, testBucket, state)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWBucketDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config("Enabled"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_bucket.test",
						tfjsonpath.New("versioning_state"),
						knownvalue.StringExact("Enabled"),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config("Suspended"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rgw_bucket.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_bucket.test",
						tfjsonpath.New("versioning_state"),
						knownvalue.StringExact("Suspended"),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config("Suspended"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccCephRGWBucketResource_ObjectLock(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-bucket-lock-owner")
	lockedBucket := acctest.RandomWithPrefix("test-bucket-locked")
	unlockedBucket := acctest.RandomWithPrefix("test-bucket-unlocked")

	userConfig := testAccProviderConfigBlock + fmt.Sprintf(`
		resource "ceph_rgw_user" "test" {
		  user_id      = %q
		  display_name = "Object Lock Test User"
		}

		resource "ceph_rgw_s3_key" "test" {
		  user_id = ceph_rgw_user.test.user_id
		}
	`, testUID)

	config := func(days int64) string {
		return userConfig + fmt.Sprintf(`
			resource "ceph_rgw_bucket" "locked" {
			  bucket                     = %q
			  owner                      = ceph_rgw_user.test.user_id
			  lock_enabled               = true
			  lock_mode                  = "GOVERNANCE"
			  lock_retention_period_days = %d
			  depends_on                 = [ceph_rgw_s3_key.test]
			}

			resource "ceph_rgw_bucket" "unlocked" {
			  bucket     = %q
			  owner      = ceph_rgw_user.test.user_id
			  depends_on = [ceph_rgw_s3_key.test]
			}
		`, lockedBucket, days, unlockedBucket)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWBucketDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config(1),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_bucket.locked",
						tfjsonpath.New("lock_enabled"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_bucket.locked",
						tfjsonpath.New("lock_mode"),
						knownvalue.StringExact("GOVERNANCE"),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_bucket.locked",
						tfjsonpath.New("lock_retention_period_days"),
						knownvalue.Int64Exact(1),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_bucket.locked",
						tfjsonpath.New("lock_retention_period_years"),
						knownvalue.Int64Exact(0),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_bucket.locked",
						tfjsonpath.New("versioning_state"),
						knownvalue.StringExact("Enabled"),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_bucket.unlocked",
						tfjsonpath.New("lock_enabled"),
						knownvalue.Bool(false),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_bucket.unlocked",
						tfjsonpath.New("lock_mode"),
						knownvalue.Null(),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_bucket.unlocked",
						tfjsonpath.New("lock_retention_period_days"),
						knownvalue.Null(),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config(2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rgw_bucket.locked", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_bucket.locked",
						tfjsonpath.New("lock_retention_period_days"),
						knownvalue.Int64Exact(2),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config(2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: userConfig + fmt.Sprintf(`
					resource "ceph_rgw_bucket" "both_periods" {
					  bucket                      = "%s-both"
					  owner                       = ceph_rgw_user.test.user_id
					  lock_enabled                = true
					  lock_mode                   = "GOVERNANCE"
					  lock_retention_period_days  = 1
					  lock_retention_period_years = 1
					  depends_on                  = [ceph_rgw_s3_key.test]
					}
				`, lockedBucket),
				ExpectError: regexp.MustCompile(`must be set, not both`),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: userConfig + fmt.Sprintf(`
					resource "ceph_rgw_bucket" "no_period" {
					  bucket       = "%s-none"
					  owner        = ceph_rgw_user.test.user_id
					  lock_enabled = true
					  lock_mode    = "GOVERNANCE"
					  depends_on   = [ceph_rgw_s3_key.test]
					}
				`, lockedBucket),
				ExpectError: regexp.MustCompile(`must be set when lock_enabled is true`),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: userConfig + fmt.Sprintf(`
					resource "ceph_rgw_bucket" "mode_only" {
					  bucket     = "%s-mode"
					  owner      = ceph_rgw_user.test.user_id
					  lock_mode  = "GOVERNANCE"
					  depends_on = [ceph_rgw_s3_key.test]
					}
				`, lockedBucket),
				ExpectError: regexp.MustCompile(`requires lock_enabled`),
			},
			{
				// The destroy after the last step reuses its config, so
				// it must be a valid one.
				ConfigVariables: testAccProviderConfig(),
				Config:          config(2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccCephRGWBucketResource_Tags(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-bucket-tags-owner")
	testBucket := acctest.RandomWithPrefix("test-bucket-tags")

	config := func(tags string) string {
		return testAccProviderConfigBlock + fmt.Sprintf(`
			resource "ceph_rgw_user" "test" {
			  user_id      = %q
			  display_name = "Tags Test User"
			}

			resource "ceph_rgw_s3_key" "test" {
			  user_id = ceph_rgw_user.test.user_id
			}

			resource "ceph_rgw_bucket" "test" {
			  bucket     = %q
			  owner      = ceph_rgw_user.test.user_id
			  tags       = %s
			  depends_on = [ceph_rgw_s3_key.test]
			}
		`, testUID, testBucket, tags)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWBucketDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config(`{ env = "test", team = "storage" }`),
				Check:           checkCephRGWBucketTags(t, testBucket, map[string]string{"env": "test", "team": "storage"}),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config(`{ env = "prod" }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rgw_bucket.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: checkCephRGWBucketTags(t, testBucket, map[string]string{"env": "prod"}),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config(`{}`),
				Check:           checkCephRGWBucketTags(t, testBucket, map[string]string{}),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config(`{}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccCephRGWBucketResource_Policy(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-bucket-pol-owner")
	testBucket := acctest.RandomWithPrefix("test-bucket-pol")

	config := func(policy string) string {
		return testAccProviderConfigBlock + fmt.Sprintf(`
			resource "ceph_rgw_user" "test" {
			  user_id      = %q
			  display_name = "Policy Test User"
			}

			resource "ceph_rgw_s3_key" "test" {
			  user_id = ceph_rgw_user.test.user_id
			}

			resource "ceph_rgw_bucket" "test" {
			  bucket        = %q
			  owner         = ceph_rgw_user.test.user_id
			  bucket_policy = %s
			  depends_on    = [ceph_rgw_s3_key.test]
			}
		`, testUID, testBucket, policy)
	}

	// RGW re-serializes stored policies in its normalized form, so the
	// config must use that shape (arrays for Action/Resource/Principal)
	// for plans to converge.
	compactPolicy := fmt.Sprintf(
		`"{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"AWS\":[\"arn:aws:iam:::user/%s\"]},\"Action\":[\"s3:GetObject\"],\"Resource\":[\"arn:aws:s3:::%s/*\"]}]}"`,
		testUID, testBucket)

	prettyPolicy := fmt.Sprintf(`jsonencode({
		Version = "2012-10-17"
		Statement = [{
		  Effect    = "Allow"
		  Principal = { AWS = ["arn:aws:iam:::user/%s"] }
		  Action    = ["s3:GetObject"]
		  Resource  = ["arn:aws:s3:::%s/*"]
		}]
	})`, testUID, testBucket)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWBucketDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config(compactPolicy),
			},
			{
				// The API re-serializes the stored policy; semantic
				// equality must keep the state on the configured form so
				// an unchanged config plans clean.
				ConfigVariables: testAccProviderConfig(),
				Config:          config(compactPolicy),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				// Reformatting the config plans a harmless in-place
				// update, since config-supplied values always become the
				// planned value.
				ConfigVariables: testAccProviderConfig(),
				Config:          config(prettyPolicy),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rgw_bucket.test", plancheck.ResourceActionUpdate),
					},
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config(prettyPolicy),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func checkCephRGWBucketOwner(t *testing.T, bucketName, wantOwner string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		bucket, err := cephTestClusterCLI.RgwBucketInfo(t.Context(), bucketName)
		if err != nil {
			return fmt.Errorf("failed to get bucket info: %w", err)
		}
		if bucket.Owner != wantOwner {
			return fmt.Errorf("bucket %q owner: expected %q, got %q", bucketName, wantOwner, bucket.Owner)
		}
		return nil
	}
}

func checkCephRGWBucketTags(t *testing.T, bucketName string, want map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		bucket, err := cephTestClusterCLI.RgwBucketInfo(t.Context(), bucketName)
		if err != nil {
			return fmt.Errorf("failed to get bucket info: %w", err)
		}
		if len(bucket.Tagset) != len(want) {
			return fmt.Errorf("bucket %q tags: expected %v, got %v", bucketName, want, bucket.Tagset)
		}
		for k, v := range want {
			if bucket.Tagset[k] != v {
				return fmt.Errorf("bucket %q tag %q: expected %q, got %q", bucketName, k, v, bucket.Tagset[k])
			}
		}
		return nil
	}
}
