.PHONY: build test lint clean install test-integration release changelog

build:
	go run ./scripts/build.go build

install:
	go run ./scripts/build.go install

test:
	go test -v -race ./...

lint:
	go run ./scripts/build.go lint

test-integration:
	go test ./... -tags=integration -v -count=1

release:
	go run ./scripts/build.go release

# Release prep, run on dev before the release PR: make changelog VERSION=v0.4.6
changelog:
	go run ./scripts/changelog.go cut $(VERSION)

clean:
	go run ./scripts/build.go clean
