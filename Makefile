BINARY       ?= notip-simulator-backend
REPO         ?= notipswe/notip-infra
TAG          ?= main
FILE         ?= nats-contracts.yaml
SERVICE      ?= simulator-backend
OPENAPI_REPO ?= notipswe/notip-provisioning-service
OPENAPI_TAG  ?= main
OPENAPI_FILE ?= openapi.yaml
OPENAPI_AS   ?= provisioning-service-openapi.yaml

OPENAPI_AS_FLAG := $(if $(OPENAPI_AS),--as $(OPENAPI_AS),)

.PHONY: all build run clean fmt vet lint test test-race cover fetch-contracts fetch-openapi integration-test docker-build docker-run help

## Default

all: fmt lint test integration-test build

## Build

build:
	@echo "Building binary..."
	CGO_ENABLED=0 go build -o $(BINARY) .

run:
	@echo "Running..."
	go run .

clean:
	@echo "Cleaning..."
	rm -f $(BINARY) coverage.out

## Quality

fmt:
	@echo "Formatting code..."
	go fmt ./...

vet:
	@echo "Running go vet..."
	go vet ./...

lint:
	@echo "Running linter..."
	golangci-lint run

## Test

test:
	@echo "Running unit tests..."
	go test -coverprofile=coverage.out ./...

integration-test:
	@echo "Executing integration tests..."
	go test -tags=integration -timeout=5m ./tests/integration/...

cover:
	@echo "Running tests with coverage..."
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

## Contracts

fetch-contracts:
	@echo "Fetching contracts..."
	bash scripts/generate-asyncapi.sh --repo $(REPO) --tag $(TAG) --file $(FILE) --service $(SERVICE)

fetch-openapi:
	@echo "Fetching OpenAPI spec..."
	bash scripts/generate-openapi.sh --repo $(OPENAPI_REPO) --tag $(OPENAPI_TAG) --file $(OPENAPI_FILE) $(OPENAPI_AS_FLAG)

## Docker

docker-build:
	@echo "Building Docker image..."
	docker build --target prod -t $(BINARY) .

docker-run:
	@echo "Running Docker image..."
	docker run --rm $(BINARY)

## Help

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "  all              fmt → lint → test → integration-test → build"
	@echo "  build            compile the binary"
	@echo "  run              go run ."
	@echo "  clean            remove binary and coverage report"
	@echo "  fmt              go fmt ./..."
	@echo "  vet              go vet ./..."
	@echo "  lint             golangci-lint run"
	@echo "  test             run unit tests with coverage"
	@echo "  integration-test run integration tests (tag=integration, timeout=5m)"
	@echo "  cover            run tests and open HTML coverage report"
	@echo "  fetch-contracts  fetch and generate AsyncAPI contracts"
	@echo "  fetch-openapi    fetch OpenAPI spec"
	@echo "  docker-build     build production Docker image"
	@echo "  docker-run       run the Docker image"
