VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test lint clean install test-integration

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bolt-cowork ./cmd/bolt-cowork

install:
	go install -ldflags "-X main.version=$(VERSION)" ./cmd/bolt-cowork

test:
	go test -v -race ./...

lint:
	golangci-lint run ./...

test-integration:
	go test ./... -tags=integration -v -count=1

clean:
	rm -f bolt-cowork
