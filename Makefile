.PHONY: fmt check build

fmt:
	gofmt -w cmd internal site/test

check:
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal; echo "run make fmt"; exit 1)
	go vet ./cmd/... ./internal/...
	go test -race ./cmd/... ./internal/...

build:
	go build -trimpath -o gopro-yank ./cmd/gopro-yank
