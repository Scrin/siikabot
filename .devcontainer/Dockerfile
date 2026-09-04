FROM mcr.microsoft.com/devcontainers/javascript-node:26

# Native build dependencies for the Go backend.
#
# The bot links against libolm for Matrix E2EE (see matrix/crypto_store.go), which
# means `go build`/`go test ./...` need the olm headers, not just the runtime .so —
# the same reason the release Dockerfile installs libolm-dev in its builder stage.
# Keep this in sync with the builder stage of the repo-root Dockerfile.
#
# build-essential and pkg-config already ship in the base image (node-gyp needs
# them); they are named explicitly so a base-image change cannot silently break
# cgo builds here.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libolm-dev \
        build-essential \
        pkg-config && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Seed ownership of the persistent volumes mounted at these paths in
# devcontainer.json. A fresh named volume inherits the image directory's
# ownership on first use, so this makes them writable by the non-root `node`
# user. GOMODCACHE is pinned to this path in devcontainer.json rather than left
# to derive from GOPATH, which the Go feature owns and could change.
RUN mkdir -p /home/node/.claude /home/node/go/pkg/mod && \
    chown -R node:node /home/node/.claude /home/node/go
