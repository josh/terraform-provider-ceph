FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

RUN apt-get update && apt-get install --yes \
    ceph \
    ceph-mgr-dashboard \
    python3-setuptools \
    radosgw \
    unzip \
    wget

ARG GO_VERSION=1.24.9
RUN curl --fail --silent --show-error --location https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz | \
    tar --extract --gzip --directory /usr/local && \
    ln --symbolic /usr/local/go/bin/go /usr/local/bin/go && \
    ln --symbolic /usr/local/go/bin/gofmt /usr/local/bin/gofmt

ARG TERRAFORM_VERSION=1.13.3
RUN wget --quiet https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    mv terraform /usr/local/bin/ && \
    rm terraform_${TERRAFORM_VERSION}_linux_amd64.zip

ARG GOLANGCI_LINT_VERSION=2.1.6
RUN curl --silent --show-error --fail --location \
    https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz | \
    tar --extract --gzip --directory /usr/local/bin --strip-components=1 golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64/golangci-lint

USER vscode
WORKDIR /workspace
