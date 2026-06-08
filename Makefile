.PHONY: all clean pre_build api gitlab codeup api-linux gitlab-linux codeup-linux docker-api docker-runner docker-checkout

BUILD_DIR=bin

pre_build:
	rm -rf $(BUILD_DIR)/*
	mkdir -p $(BUILD_DIR)

# macOS builds
api:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BUILD_DIR)/neutron-api cmd/api/main.go
	chmod a+x $(BUILD_DIR)/neutron-api
gitlab:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BUILD_DIR)/neutron-gitlab-runner cmd/gitlab-runner/*.go
	chmod a+x $(BUILD_DIR)/neutron-gitlab-runner
codeup:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BUILD_DIR)/neutron-codeup-runner cmd/codeup-runner/*.go
	chmod a+x $(BUILD_DIR)/neutron-codeup-runner

# Linux cross-compile (for Docker images)
api-linux:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BUILD_DIR)/neutron-api-linux cmd/api/main.go
gitlab-linux:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BUILD_DIR)/neutron-gitlab-runner-linux cmd/gitlab-runner/*.go
codeup-linux:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BUILD_DIR)/neutron-codeup-runner-linux cmd/codeup-runner/*.go

# Docker images
docker-api: api-linux
	docker build -t neutron-api:local -f Dockerfile .
docker-runner: gitlab-linux codeup-linux
	docker build -t neutron-runner:local -f Dockerfile.runner .
docker-checkout:
	docker build -t neutron-checkout:local -f Dockerfile.checkout .

# Load images into kind
kind-load: docker-api docker-runner docker-checkout
	kind load docker-image neutron-api:local --name neutron
	kind load docker-image neutron-runner:local --name neutron
	kind load docker-image neutron-checkout:local --name neutron

clean:
	rm -rf $(BUILD_DIR)/*
