.PHONY: run test build lint tidy

build:
	go build -o bin/server ./cmd/server

run:
	go run ./cmd/server

test:
	go test ./... -v -race

lint:
	go vet ./...

tidy:
	go mod tidy
