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

Run the suite through `scripts/run-container-tests.sh` when possible; it builds the dev image and executes `go test` inside the container with `TF_ACC=1`. The script mounts the repository's `./tmp` directory at `/tmp/host` in the container so you can collect artifacts like coverage reports.

To capture coverage, write the profile into the mounted directory, for example:

```sh
scripts/run-container-tests.sh -cover -coverprofile=/tmp/host/coverage.out ./...
```

## Building

Build the project with:

```sh
go build ./...
```

## Comments

Keep comments concise. Only add them when they clarify non-obvious logic. Avoid inline comments—prefer short comment blocks immediately above the code they explain when they are necessary.

## Best Practices

- Validate provider and resource inputs early and surface problems via diagnostics instead of panicking.
- Reuse `CephAPIClient` for Ceph API interactions to keep headers, timeouts, and error handling consistent across resources and data sources.
- Propagate `context.Context` from Terraform entrypoints through every API call so operations respect cancellations and deadlines.
- Use helpers like `mapAttrToCephCaps` and `cephCapsToMapValue` to convert between Terraform types and Go structs, guarding against unknown or null values before calling Ceph.
- When tests shell out to `ceph`, wrap calls with context timeouts and rely on the shared harness to spin up and tear down the ephemeral cluster.
