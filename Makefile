.PHONY: run test test-unit build lint tidy

build:
	go build -o bin/server .

run:
	go run .

# Unit tests only — no database required.
test-unit:
	go test -run "TestValidate|TestRequireAuth" -v ./...

# Full test suite — requires a running Postgres instance.
# export TEST_DATABASE_URL=postgres://user:pass@localhost:5432/catalog_test?sslmode=disable
test:
	go test ./... -v -race

lint:
	go vet ./...

tidy:
	go mod tidy
