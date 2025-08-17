default: portquiz

RELEASE_DEPS = fmt lint
include release.mk

.PHONY: portquiz
portquiz: portquiz-client portquiz-server

.PHONY: deps
deps: go.mod
	GOPROXY=direct go mod download
	GOPROXY=direct go get -u all

.PHONY: update-deps
update-deps:
	go get -u
	go mod tidy

.PHONY: clean
clean:
	rm -rf portquiz-client portquiz-server dist/

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: lint
lint:
	golangci-lint run

portquiz-client: go.mod go.sum client/*go
	go build -o $@ client/*.go

portquiz-server: go.mod go.sum server/*go
	go build -o $@ server/*.go

.PHONY: goreleaser
goreleaser:
	goreleaser build --snapshot --clean
