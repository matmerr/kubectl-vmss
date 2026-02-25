MODULE       := github.com/matmerr/kubectl-vmss
VERSION      ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GIT_COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE   := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS      := -s -w \
	-X $(MODULE)/pkg/version.Version=$(VERSION) \
	-X $(MODULE)/pkg/version.GitCommit=$(GIT_COMMIT) \
	-X $(MODULE)/pkg/version.BuildDate=$(BUILD_DATE)

BIN          := kubectl-vmss
CMD          := ./cmd/kubectl-vmss

.PHONY: build test clean install dist

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BIN) $(CMD)

test:
	go test ./...

install: build
	mv $(BIN) $(shell go env GOPATH)/bin/$(BIN)

clean:
	rm -rf $(BIN) dist/

dist:
	@mkdir -p dist
	@for os_arch in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do \
		os=$${os_arch%/*}; \
		arch=$${os_arch#*/}; \
		ext=""; \
		[ "$$os" = "windows" ] && ext=".exe"; \
		echo "Building $$os/$$arch..."; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags="$(LDFLAGS)" \
			-o "dist/$(BIN)-$$os-$$arch$$ext" $(CMD); \
	done
