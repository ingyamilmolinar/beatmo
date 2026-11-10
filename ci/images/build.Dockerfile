# Everything a non-browser CI job needs: C toolchain, Ebiten's cgo headers,
# xvfb + software GL for the real-Ebiten suite, Go, emsdk, golangci-lint.
ARG DEBIAN_IMAGE
FROM ${DEBIAN_IMAGE}

ARG GO_VERSION
ARG GO_SHA256
ARG EMSDK_VERSION
ARG GOLANGCI_LINT_VERSION
ARG GOLANGCI_LINT_SHA256
ARG GOVULNCHECK_VERSION

ENV DEBIAN_FRONTEND=noninteractive LANG=C.UTF-8

RUN apt-get update \
 && apt-get upgrade -y --no-install-recommends \
 && apt-get install -y --no-install-recommends \
      build-essential pkg-config ca-certificates curl git xz-utils python3 \
      cppcheck xvfb xauth \
      libasound2-dev libgl1-mesa-dev libgl1-mesa-dri libglx-mesa0 \
      libx11-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev libxxf86vm-dev \
 && rm -rf /var/lib/apt/lists/* \
 && git config --system --add safe.directory '*'

RUN curl -fsSLo /tmp/go.tgz "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
 && echo "${GO_SHA256}  /tmp/go.tgz" | sha256sum -c - \
 && tar -C /usr/local -xzf /tmp/go.tgz && rm /tmp/go.tgz

RUN curl -fsSLo /tmp/gl.tgz "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz" \
 && echo "${GOLANGCI_LINT_SHA256}  /tmp/gl.tgz" | sha256sum -c - \
 && tar -xzf /tmp/gl.tgz -C /tmp \
 && mv "/tmp/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64/golangci-lint" /usr/local/bin/ \
 && rm -rf /tmp/gl.tgz /tmp/golangci-lint-*

# The first emcc run builds its system libraries; doing it here keeps that out
# of every CI job. The cache stays writable so jobs may run as any uid.
RUN git clone --depth 1 --branch "${EMSDK_VERSION}" https://github.com/emscripten-core/emsdk.git /opt/emsdk \
 && cd /opt/emsdk && ./emsdk install "${EMSDK_VERSION}" && ./emsdk activate "${EMSDK_VERSION}" \
 && rm -rf /opt/emsdk/.git /opt/emsdk/downloads \
 && echo 'int main(void){return 0;}' > /tmp/w.c \
 && . /opt/emsdk/emsdk_env.sh && emcc -O3 /tmp/w.c -o /tmp/w.js && rm -f /tmp/w.* \
 && chmod -R a+rwX /opt/emsdk/upstream/emscripten/cache

# GOTOOLCHAIN=local: the image's Go is the toolchain; never download another.
ENV GOTOOLCHAIN=local \
    EMSDK=/opt/emsdk \
    EM_CONFIG=/opt/emsdk/.emscripten \
    GOLANGCI_LINT=/usr/local/bin/golangci-lint \
    GOCACHE=/cache/go-build GOMODCACHE=/cache/gomod npm_config_cache=/cache/npm \
    PATH=/usr/local/go/bin:/opt/emsdk:/opt/emsdk/upstream/emscripten:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
RUN mkdir -p /cache && chmod 1777 /cache

# Tools the tests (ffmpeg), actions/cache (zstd), `make lint` (shellcheck) and
# `make vulncheck` shell out to; last, so adding one keeps every layer above cached.
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg zstd shellcheck \
 && rm -rf /var/lib/apt/lists/*
RUN GOBIN=/usr/local/bin go install "golang.org/x/vuln/cmd/govulncheck@v${GOVULNCHECK_VERSION}" \
 && go clean -cache -modcache
