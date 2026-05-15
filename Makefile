.PHONY: build test lint clean install test-integration release

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

clean:
	go run ./scripts/build.go clean
