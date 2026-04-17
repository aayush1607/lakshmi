BINARY  ?= lakshmi
PKG     := github.com/aayush1607/lakshmi
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X $(PKG)/internal/version.Version=$(VERSION)

.PHONY: build test lint run clean install tidy vet

build:
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/lakshmi

test:
	go test ./...

vet:
	go vet ./...

lint: vet
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed; skipping"

run: build
	./bin/$(BINARY)

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/lakshmi

tidy:
	go mod tidy

clean:
	rm -rf bin
