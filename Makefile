BINARY_NAME := kubectl-pvc-usage
MODULE := github.com/herveleclerc/pvc-usage
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.1.0")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -w -s \
	-X $(MODULE)/pkg/version.Version=$(VERSION) \
	-X $(MODULE)/pkg/version.GitCommit=$(GIT_COMMIT) \
	-X $(MODULE)/pkg/version.BuildDate=$(BUILD_DATE)

PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64

.PHONY: all build build-all test lint clean install

all: test build

build:
	@mkdir -p bin
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/kubectl-pvc-usage

build-all:
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		OS=$${platform%/*}; \
		ARCH=$${platform#*/}; \
		OUTPUT=dist/$(BINARY_NAME)-$${OS}-$${ARCH}; \
		if [ "$${OS}" = "windows" ]; then \
			OUTPUT="$${OUTPUT}.exe"; \
		fi; \
		echo "Building for $${OS}/$${ARCH}..."; \
		GOOS=$${OS} GOARCH=$${ARCH} CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o "$${OUTPUT}" ./cmd/kubectl-pvc-usage || exit 1; \
	done; \
	echo "Multi-arch build completed in ./dist"

test:
	go test -v ./...

test-race:
	go test -v -race ./...

lint:
	go vet ./...

install: build
	@INSTALL_DIR=$$(go env GOPATH)/bin; \
	mkdir -p "$${INSTALL_DIR}"; \
	cp bin/$(BINARY_NAME) "$${INSTALL_DIR}/$(BINARY_NAME)"; \
	echo "Installed $(BINARY_NAME) to $${INSTALL_DIR}"

clean:
	rm -rf bin dist
