#!/bin/bash
set -e

TAG="${1:-local}"
BUILD_DIR="bin"

mkdir -p "$BUILD_DIR"

# Cross-compile for Linux arm64 (kind on Apple Silicon)
echo "Building gitlab-runner binary..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o "${BUILD_DIR}/neutron-gitlab-runner-linux" ./cmd/gitlab-runner

echo "Building codeup-runner binary..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o "${BUILD_DIR}/neutron-codeup-runner-linux" ./cmd/codeup-runner

echo "Building API server binary..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o "${BUILD_DIR}/neutron-api-linux" ./cmd/api

echo "Building Docker images..."
docker build -t "neutron-api:${TAG}" -f Dockerfile .
docker build -t "neutron-runner:${TAG}" -f Dockerfile.runner .

echo "Done. Images:"
docker images | grep "neutron-"
