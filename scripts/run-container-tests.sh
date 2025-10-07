#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

if command -v container >/dev/null 2>&1; then
  set -o xtrace
  container build --file Dockerfile-dev --tag terraform-provider-ceph:latest .
  container run --rm --name terraform-provider-ceph --env TF_ACC=1 terraform-provider-ceph:latest go test -v
elif command -v docker >/dev/null 2>&1; then
  set -o xtrace
  docker build --file Dockerfile-dev --tag terraform-provider-ceph:latest .
  docker run --rm --name terraform-provider-ceph --env TF_ACC=1 terraform-provider-ceph:latest go test -v
else
  echo "error: no container runtime available" >&2
  exit 1
fi
