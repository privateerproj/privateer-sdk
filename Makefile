build: tidy build test
quick: build
testcov: test-cov

tidy:
	@echo "  >  Tidying go.mod ..."
	go mod tidy

build:
	@echo "  >  Building ..."
	go build ./...

test:
	@echo "  >  Validating code..."
	go vet ./...
	go clean -testcache
	go test ./...

test-cov:
	@echo "Running tests and generating coverage output"
	@go test ./... -coverprofile coverage.out -covermode count
	@echo "Current test coverage : $(shell go tool cover -func=coverage.out | grep total | grep -Eo '[0-9]+\.[0-9]+') %"
