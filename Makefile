BINARY  := sshmanager
CMD     := ./cmd/sshmanager
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.DEFAULT_GOAL := help

## build: Compile the binary (output: ./sshmanager)
.PHONY: build
build:
	go build $(LDFLAGS) -o $(BINARY) $(CMD)

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

## clean: Remove build artifacts
.PHONY: clean
clean:
	rm -f $(BINARY)

## help: Show this help
.PHONY: help
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
