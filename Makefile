# Development tasks for github.com/sojoudian/color.
BINARY  := color
IMAGE   ?= ghcr.io/sojoudian/color
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

# Tool versions — keep in sync with .github/workflows/ci.yaml.
GOVULNCHECK_VERSION := v1.3.0
GOLANGCI_LINT_VERSION := v2.12   # CI uses golangci-lint-action with this

.PHONY: all build run test lint vet vuln fmt cover docker docker-run clean

all: fmt vet lint test build

build: ## Build the release binary into ./bin
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/$(BINARY)

run: ## Run locally on :8080
	go run ./cmd/$(BINARY)

test: ## Run tests with the race detector and coverage
	go test -race -shuffle=on -coverprofile=coverage.out ./...

cover: test ## Open the HTML coverage report
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html

fmt: ## Verify formatting (same formatters CI enforces: gofmt + goimports)
	golangci-lint fmt --diff

vet:
	go vet ./...

lint: ## Run golangci-lint (requires $(GOLANGCI_LINT_VERSION).x — see .golangci.yml)
	golangci-lint run

vuln: ## Scan for known vulnerabilities
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

docker: ## Build the container image for the host platform
	docker build -t $(IMAGE):dev --build-arg VERSION=$(VERSION) .

docker-run: docker ## Run the container on :8080
	docker run --rm -p 8080:8080 $(IMAGE):dev

clean:
	rm -rf bin coverage.out coverage.html
