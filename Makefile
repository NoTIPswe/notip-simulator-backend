BINARY  ?= notip-app
TAG     ?= main
FILE    ?= nats-contracts.yaml
SERVICE ?= data-consumer

.PHONY: all build run clean fmt vet lint test test-race cover fetch-contracts docker-build docker-run help

## Default

all: fmt lint test build

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
	@echo "Running tests..."
	go test -race -coverprofile=coverage.out ./...

cover:
	@echo "Running tests with coverage..."
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

## Contracts

fetch-contracts:
	@echo "Fetching contracts..."
	bash scripts/generate-asyncapi.sh --tag $(TAG) --file $(FILE) --service $(SERVICE)

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
	@echo "  all            fmt → lint → test → build"
	@echo "  build          compile the binary"
	@echo "  run            go run ."
	@echo "  clean          remove binary and coverage report"
	@echo "  fmt            go fmt ./..."
	@echo "  vet            go vet ./..."
	@echo "  lint           golangci-lint run"
	@echo "  test           run all tests with -race"
	@echo "  cover          run tests and open HTML coverage report"
	@echo "  fetch-contracts fetch and generate AsyncAPI contracts"
	@echo "  docker-build   build production Docker image"
	@echo "  docker-run     run the Docker image"
