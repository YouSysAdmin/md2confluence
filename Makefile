.PHONY: all build install test vet tidy clean help

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

BIN := bin/md2confluence
PKG := ./cmd/md2confluence

help:
	@echo "Targets:"
	@echo "  build    build host binary into $(BIN)"
	@echo "  install  go install $(PKG) into \$$GOBIN"
	@echo "  test     run go test ./..."
	@echo "  vet      run go vet ./..."
	@echo "  tidy     run go mod tidy"
	@echo "  clean    remove bin/"

all: build

build:
	@mkdir -p bin
	go build $(LDFLAGS) -o $(BIN) $(PKG)

install:
	go install $(LDFLAGS) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf bin
