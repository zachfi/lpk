

LD_FLAGS=-ldflags " \
    -X main.goos=$(shell go env GOOS) \
    -X main.goarch=$(shell go env GOARCH) \
    -X main.gitCommit=$(shell git rev-parse HEAD) \
    -X main.buildDate=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ') \
    "

.PHONY: build
build:
	@go build $(LD_FLAGS) -o bin/lpk cmd/lpk/main.go

.PHONY: drone
drone:
	@drone jsonnet --stream --format
	@drone lint

.PHONY: lint
lint:
	@golangci-lint run
