BINARY  := lazyssh
CMD     := ./cmd/lazyssh
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.DEFAULT_GOAL := help

## build: Compile the binary without embedded lss helpers (output: ./lazyssh)
.PHONY: build
build:
	go build $(LDFLAGS) -o $(BINARY) $(CMD)

## build-full: Build lss helpers then compile lazyssh with embedded helpers
.PHONY: build-full
build-full: lss-helpers
	go build $(LDFLAGS) -tags embed_lss -o $(BINARY) $(CMD)

## lss-helpers: Cross-compile lss for linux/amd64 and linux/arm64
.PHONY: lss-helpers
lss-helpers: internal/ssh/embed/lss-linux-amd64 internal/ssh/embed/lss-linux-arm64

internal/ssh/embed/lss-linux-amd64:
	mkdir -p internal/ssh/embed
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $@ ./cmd/lss

internal/ssh/embed/lss-linux-arm64:
	mkdir -p internal/ssh/embed
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $@ ./cmd/lss

## install: Install the binary to $GOPATH/bin
.PHONY: install
install:
	go install $(LDFLAGS) $(CMD)

## run: Build and run immediately
.PHONY: run
run: build
	./$(BINARY)

## test: Run the test suite
.PHONY: test
test:
	go test ./...

## fmt: Format all Go source files
.PHONY: fmt
fmt:
	gofmt -w .

## vet: Run go vet
.PHONY: vet
vet:
	go vet ./...

## tidy: Tidy go.mod / go.sum
.PHONY: tidy
tidy:
	go mod tidy

## tag: Create and push a release tag (usage: make tag V=1.2.3)
.PHONY: tag
tag:
	@test -n "$(V)" || (echo "Usage: make tag V=1.2.3"; exit 1)
	git tag -a v$(V) -m "Release v$(V)"
	git push origin v$(V)

## clean: Remove build artifacts
.PHONY: clean
clean:
	rm -f $(BINARY)

## help: Show this help
.PHONY: help
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
