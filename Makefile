# Variables
BINARY_NAME := k8s-iperf
DOCKER_IMAGE := dariomader/iperf3
DOCKER_TAG := latest

# Go build flags
GO_BUILD_FLAGS := -ldflags="-s -w"

.PHONY: all build docker clean lint

all: lint build docker push

lint:
	@echo "Running golangci-lint..."
	golangci-lint run ./...

build: lint
	@echo "Building Go binary..."
	go build $(GO_BUILD_FLAGS) -o $(BINARY_NAME) ./cmd/main.go

docker: build
	@echo "Building multi-platform Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

clean:
	@echo "Cleaning up..."
	rm -f $(BINARY_NAME)
	docker rmi $(DOCKER_IMAGE):$(DOCKER_TAG)

push: docker
	@echo "Pushing multi-platform Docker image..."
	docker buildx build --platform linux/amd64,linux/arm64 -t $(DOCKER_IMAGE):$(DOCKER_TAG) --push .
