# `make generate` regenerates the committed OpenAPI document from the API's
# route table (internal/api/routes.go) and its component schemas
# (internal/api/openapi.components.yaml). CI runs it and fails when the output
# differs from what is committed, so the published spec cannot drift from the
# served API.

.PHONY: generate test

generate:
	go run ./cmd/openapi

test:
	go test ./...
