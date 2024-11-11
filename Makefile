build: go-tidy go-build go-test
quick: go-build
test: go-test
testcov: go-test-cov

go-tidy:
	@echo "  >  Tidying go.mod ..."
	go mod tidy

go-build:
	@echo "  >  Building ..."
	go build ./...

go-test:
	@echo "  >  Validating code..."
	go vet ./...
	go test ./...

go-test-cov:
	@echo "Running tests and generating coverage output"
	@go test ./... -coverprofile coverage.out -covermode count
	@echo "Current test coverage : $(shell go tool cover -func=coverage.out | grep total | grep -Eo '[0-9]+\.[0-9]+') %"

