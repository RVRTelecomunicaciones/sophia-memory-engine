.PHONY: test test-unit test-integration lint modernize

test:
	go test ./...

test-unit:
	go test ./internal/domain/... ./internal/application/...

test-integration:
	go test ./test/integration/... -tags=integration -count=1

lint:
	golangci-lint run ./...

modernize:
	go fix ./...
