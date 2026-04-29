.PHONY: build test test-integration test-all

build:
	go build -o shrine .

test:
	go test ./...

test-integration:
	go test -tags integration -v -timeout 5m ./tests/integration/...

test-all: test test-integration
