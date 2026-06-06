.PHONY: all clean pre_build api gitlab api-linux runner-linux docker-api docker-runner

BUILD_DIR=bin

pre_build:
	rm -rf $(BUILD_DIR)/*
	mkdir -p $(BUILD_DIR)

# macOS builds
api:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BUILD_DIR)/neutron-api cmd/api/main.go
	chmod a+x $(BUILD_DIR)/neutron-api
gitlab:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BUILD_DIR)/neutron-gitlab-runner cmd/gitlab-runner/*.go
	chmod a+x $(BUILD_DIR)/neutron-gitlab-runner

# Linux cross-compile (for Docker images)
api-linux:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BUILD_DIR)/neutron-api-linux cmd/api/main.go
runner-linux:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BUILD_DIR)/neutron-gitlab-runner-linux cmd/gitlab-runner/*.go

# Docker images
docker-api: api-linux
	docker build -t neutron-api:local -f Dockerfile .
docker-runner: runner-linux
	docker build -t neutron-runner:local -f Dockerfile.runner .

# Load images into kind
kind-load: docker-api docker-runner
	kind load docker-image neutron-api:local --name neutron
	kind load docker-image neutron-runner:local --name neutron

clean:
	rm -rf $(BUILD_DIR)/*
