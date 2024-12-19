.PHONY: all clean

BUILD_DIR=bin

pre_build:
	rm -rf $(BUILD_DIR)/*
	mkdir -p $(BUILD_DIR)

api:
	CGO_ENABLED=0 go build -o $(BUILD_DIR)/neutron-api cmd/api/main.go
	chmod a+x $(BUILD_DIR)/neutron-api
gitlab:
	CGO_ENABLED=0 go build -o $(BUILD_DIR)/neutron-gitlab-runner cmd/gitlab-runner/*.go
	chmod a+x $(BUILD_DIR)/neutron-gitlab-runner
clean:
	rm -rf $(BUILD_DIR)/*
