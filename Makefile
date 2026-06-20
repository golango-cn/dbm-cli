BINARY   := dbm-cli
MODULE    := github.com/golango-cn/dbm-cli
MAIN      := $(MODULE)/cmd/dbm-cli
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS   := -s -w -X '$(MODULE)/internal/buildinfo.Version=$(VERSION)' -X '$(MODULE)/internal/buildinfo.Commit=$(COMMIT)'

GOFLAGS   := -trimpath
CGO_ENABLED := 0

.PHONY: all build run tidy vet test fmt lint clean dist help

all: build

## build: compile the dbm-cli binary into ./bin
build:
	CGO_ENABLED=$(CGO_ENABLED) go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(MAIN)

## run: build and run with given args, e.g. make run ARGS="--help"
run: build
	./bin/$(BINARY) $(ARGS)

## tidy: go mod tidy
tidy:
	go mod tidy

## vet: go vet
vet:
	go vet ./...

## test: run unit tests
test:
	go test ./...

## fmt: gofmt + simplify
fmt:
	gofmt -s -w .

## lint: placeholder for golangci-lint if installed
lint:
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

## clean: remove build artifacts
clean:
	rm -rf bin dist

## dist: cross-compile static binaries for linux/darwin/windows (amd64+arm64)
dist:
	@mkdir -p dist
	@for os in linux darwin windows; do \
	  for arch in amd64 arm64; do \
	    ext=""; [ $$os = windows ] && ext=".exe"; \
	    echo "  -> $$os/$$arch"; \
	    GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
	      -o dist/$(BINARY)-$$os-$$arch$$ext $(MAIN); \
	  done; \
	done

help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'
