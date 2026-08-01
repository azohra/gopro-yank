.PHONY: fmt check build snapshot release

fmt:
	gofmt -w cmd internal

check:
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal; echo "run make fmt"; exit 1)
	go vet ./...
	go test -race ./...

build:
	go build -trimpath -o gopro-yank ./cmd/gopro-yank

snapshot:
	./scripts/build-release.sh dev

release:
	@test -n "$(VERSION)" || (echo "usage: make release VERSION=1.0.0" && exit 2)
	./scripts/build-release.sh "$(VERSION)"
