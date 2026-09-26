.PHONY: build run test test-race vet fmt docker-build docker-test

GO ?= go

build:
	$(GO) build ./...

run:
	$(GO) run ./cmd/server

test:
	$(GO) test ./... -count=1

test-race:
	$(GO) test -race ./... -count=1

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

docker-build:
	docker build -t rocalc:latest .

docker-test:
	docker build --target build -t rocalc:build .
