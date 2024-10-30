

LD_FLAGS=-ldflags " \
    -X main.goos=$(shell go env GOOS) \
    -X main.goarch=$(shell go env GOARCH) \
    -X main.gitCommit=$(shell git rev-parse HEAD) \
    -X main.buildDate=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ') \
    "

.PHONY: build
build:
	@go build $(LD_FLAGS) -o bin/lpk cmd/lpk/main.go

.PHONY: lint
lint:
	@golangci-lint run

.PHONY: drone drone-signature
drone:
	@drone jsonnet --stream --format
	@drone lint --trusted

drone-signature:
ifndef DRONE_TOKEN
	$(error DRONE_TOKEN is not set, visit https://drone.zach.fi/account)
endif
	@DRONE_SERVER=https://drone.zach.fi drone sign --save zachfi/iotcontroller .drone.yml
