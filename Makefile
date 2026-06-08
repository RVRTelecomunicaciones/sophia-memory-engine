.PHONY: test test-unit test-integration openapi-validate lint modernize

test:
	go test ./...

test-unit:
	go test ./internal/domain/... ./internal/application/... ./internal/adapters/...

test-integration:
	go test ./test/integration/... ./internal/adapters/... -tags=integration -count=1

openapi-validate:
	go test ./test/openapi/... -count=1

lint:
	golangci-lint run ./...

modernize:
	go fix ./...
