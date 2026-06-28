.PHONY: run test build lint tidy

build:
	go build -o bin/server .

run:
	go run .

test:
	go test ./... -v -race

lint:
	go vet ./...

tidy:
	go mod tidy
