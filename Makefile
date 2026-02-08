REGISTRY ?= ghcr.io/rhwendt/helios
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
SERVICES := flow-enricher runbook-operator target-generator
PLATFORMS ?= linux/amd64,linux/arm64

.PHONY: all lint test build docker-build helm-lint helm-unittest proto-gen

all: lint test build

## Lint all Go services
lint:
	@for svc in $(SERVICES); do \
		echo "==> Linting $$svc"; \
		golangci-lint run ./services/$$svc/...; \
	done

## Run tests for all Go services
test:
	@for svc in $(SERVICES); do \
		echo "==> Testing $$svc"; \
		go test ./services/$$svc/... -v -race -coverprofile=coverage-$$svc.out; \
	done

## Build all Go service binaries
build:
	@for svc in $(SERVICES); do \
		echo "==> Building $$svc"; \
		CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(VERSION)" \
			-o bin/$$svc ./services/$$svc/cmd/...; \
	done

## Build multi-arch Docker images for all services
docker-build:
	@for svc in $(SERVICES); do \
		echo "==> Building Docker image for $$svc"; \
		docker buildx build \
			--platform $(PLATFORMS) \
			--tag $(REGISTRY)/$$svc:$(VERSION) \
			--file services/$$svc/Dockerfile \
			services/$$svc; \
	done

## Lint Helm umbrella chart and all sub-charts
helm-lint:
	helm lint helm/helios
	@for chart in helm/helios/charts/*/; do \
		echo "==> Linting $$chart"; \
		helm lint $$chart; \
	done

## Run Helm unittest on all chart test directories
helm-unittest:
	@for chart in helm/helios/charts/*/; do \
		if [ -d "$$chart/tests" ]; then \
			echo "==> Running unittests for $$chart"; \
			helm unittest $$chart; \
		fi; \
	done

## Generate Go code from proto/flow.proto
proto-gen:
	protoc --go_out=. \
		--go_opt=module=github.com/rhwendt/helios \
		proto/flow.proto
