# Agents Guide

This project is written in Go and uses Go modules for dependency management. The repository assumes Go 1.24 or newer.

## Setup

1. Install Go 1.24 or later.
2. Download dependencies with:

```sh
go mod download
```

## Formatting

Format code with:

```sh
go fmt ./...
```

## Code Quality

Run vet and static analysis tools before committing:

```sh
go vet ./...
```

Optionally run `golangci-lint` for additional checks:

```sh
golangci-lint run ./...
```

## Testing

Default to acceptance tests using `terraform-plugin-testing` and the embedded Ceph harness. Reach for unit tests only for tight, algorithmic helpers (for example the keyring parser) where they provide clear value.

The harness starts a throwaway cluster, so `ceph-mon`, `ceph-osd`, `ceph-mgr` with the dashboard module, `radosgw`, and `terraform` must be on `PATH`. CI runs the suite on the Ceph runner images:

```sh
TF_ACC=1 go test -cover -timeout 90m
```

For detailed debugging of API interactions, enable TRACE-level logging:

```sh
TF_LOG=TRACE TF_ACC=1 go test -v -run TestAccCephPoolResource
```

This will output raw JSON request and response bodies for every Ceph API call, which can be helpful for diagnosing unexpected behavior or API changes.

## Building

Build the project with:

```sh
go build ./...
```

## Comments

Keep comments concise. Only add them when they clarify non-obvious logic. Avoid inline comments—prefer short comment blocks immediately above the code they explain when they are necessary.

## Best Practices

- Validate provider and resource inputs early and surface problems via diagnostics instead of panicking.
- Reuse `restapi.Client` (`internal/restapi`, the Ceph RESTful API client) for Ceph API interactions to keep headers, timeouts, and error handling consistent across resources and data sources.
- Propagate `context.Context` from Terraform entrypoints through every API call so operations respect cancellations and deadlines.
- Use helpers like `mapAttrToCephCaps` and `cephCapsToMapValue` to convert between Terraform types and Go structs, guarding against unknown or null values before calling Ceph.
- When tests shell out to `ceph`, `rbd`, or `radosgw-admin`, reuse `cephcli.CLI` (`internal/cephcli`), wrap calls with context timeouts, and rely on the shared harness to spin up and tear down the ephemeral cluster.
