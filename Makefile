.PHONY: build test lint clean

build:
	go build -o bolt-cowork ./cmd/bolt-cowork

test:
	go test -v -race ./...

lint:
	golangci-lint run ./...

clean:
	rm -f bolt-cowork
