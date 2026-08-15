FROM mcr.microsoft.com/devcontainers/base:ubuntu-22.04

ARG CEPH_RELEASE=squid

RUN curl -fsSL https://download.ceph.com/keys/release.asc | gpg --dearmor -o /usr/share/keyrings/ceph-archive-keyring.gpg && \
    echo "deb [signed-by=/usr/share/keyrings/ceph-archive-keyring.gpg] https://download.ceph.com/debian-${CEPH_RELEASE}/ jammy main" > /etc/apt/sources.list.d/ceph.list

RUN apt-get update && apt-get install --yes \
    ceph \
    ceph-mgr-dashboard \
    python3-setuptools \
    radosgw \
    unzip \
    wget

ARG GO_VERSION=1.26.5
RUN curl --fail --silent --show-error --location https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz | \
    tar --extract --gzip --directory /usr/local && \
    ln --symbolic /usr/local/go/bin/go /usr/local/bin/go && \
    ln --symbolic /usr/local/go/bin/gofmt /usr/local/bin/gofmt

ARG TERRAFORM_VERSION=1.15.8
RUN wget --quiet https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    mv terraform /usr/local/bin/ && \
    rm terraform_${TERRAFORM_VERSION}_linux_amd64.zip

ARG GOLANGCI_LINT_VERSION=2.12.2
RUN curl --silent --show-error --fail --location \
    https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz | \
    tar --extract --gzip --directory /usr/local/bin --strip-components=1 golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64/golangci-lint

USER vscode
WORKDIR /workspace
