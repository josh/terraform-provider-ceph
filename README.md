# terraform-provider-ceph

A Terraform provider for managing Ceph clusters via the Ceph Dashboard REST API (Ceph RESTful API), covering authentication, configuration, placement rules, and RGW users/buckets. The provider communicates with the manager's REST HTTP endpoint, so Terraform does not require direct access to the Ceph cluster.

## Examples

### Configure the provider

```terraform
provider "ceph" {
  endpoint = "https://ceph.example.com"
  token    = var.ceph_token
}
```

### Create a dashboard user with S3 credentials

```terraform
resource "ceph_rgw_user" "dashboard" {
  user_id      = "dashboard"
  display_name = "Ceph Dashboard"
  system       = true
}

resource "ceph_rgw_s3_key" "dashboard" {
  user_id = ceph_rgw_user.dashboard.user_id
}
```

### Manage a global Ceph configuration value

```terraform
resource "ceph_config" "global" {
  section = "global"
  config = {
    # Write objects to 3 replicas
    osd_pool_default_size = 3
    # Allow writing two copies in a degraded state
    osd_pool_default_min_size = 2
  }
}
```

### Create an admin auth key with full access

```terraform
resource "ceph_auth" "client_admin" {
  entity = "client.admin"
  caps = {
    "mds" = "allow *"
    "mgr" = "allow *"
    "mon" = "allow *"
    "osd" = "allow *"
  }
}
```
