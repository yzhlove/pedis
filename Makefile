SHELL=/bin/bash

# Application Configuration
App        := pedis
Timezone   := Asia/Shanghai
Tags       := develop
Dockerfile := Dockerfile
Network    := mynet
ServerPort := 6399
UnixContainerDir := /tmp
UnixLocalDir     := /tmp
UnixSocket := $(UnixContainerDir)/pedis.sock
Env        := dev
ClientName := yurisa

# Repo root directory (works regardless of where `make` is invoked from)
RepoRoot := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))

# Build flags with embedded metadata
BuildDate  := $(shell date '+%F_%T')
GitCommit  := $(shell git describe --tags --always --dirty=-dev)
GoVersion  := $(shell go version | cut -d' ' -f3)
AppVersion := $(GitCommit)
HostArch   := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

Flags := -X main.buildDate=$(BuildDate) -X main.goVersion=$(GoVersion) -X main.appVersion=$(AppVersion)

ClientCmd := --rm=true -it \
	--network=$(Network) \
	--name=$(App)-client \
	-h $(App)-client \
	-e TZ=$(Timezone) \
	-e PEDIS_ENV=$(Env) \
	-e PEDIS_ROLE=client \
	-e PEDIS_CLI_NAME=$(ClientName) \
	-e PEDIS_CLI_REDIS_HOST=redis \
	-e PEDIS_CLI_REDIS_PORT=6379 \
	-e PEDIS_UNIX_SOCKET=$(UnixSocket) \
	-v $(UnixContainerDir):$(UnixLocalDir)

ServerCmd := --rm=true -it \
	--network=$(Network) \
	--name=$(App)-server \
	-h $(App)-server \
	-e TZ=$(Timezone) \
	-e PEDIS_ENV=$(Env) \
	-e PEDIS_ROLE=server \
	-e PEDIS_SERVER_PORT=$(ServerPort) \
	-e PEDIS_UNIX_SOCKET=$(UnixSocket) \
	-p $(ServerPort):$(ServerPort) \
	-v $(UnixContainerDir):$(UnixLocalDir)

# Build targets

## build: Cross-compile binary and build Docker image for linux/HOSTARCH
build:
	@echo -e "\n🔨 Building $(App) application..."
	@echo "   Build Date: $(BuildDate)"
	@echo "   Git Commit: $(GitCommit)"
	@echo "   Go Version: $(GoVersion)"
	@echo "   Target Arch: $(HostArch)"
	@CGO_ENABLED=0 GOOS=linux GOARCH=$(HostArch) go build -tags $(Tags) -o $(App) -ldflags '$(Flags)' .
	@echo -e "\n🐳 Building Docker image..."
	@docker rmi $(App) 2>/dev/null || true
	@docker build --platform linux/$(HostArch) --no-cache -t $(App) -f $(Dockerfile) .
	@rm -f $(App)
	@echo "✅ Build completed successfully!"

## build-local: Build native binary for local development
build-local:
	@echo -e "\n🔨 Building $(App) locally..."
	@go build -tags $(Tags) -o $(App) -ldflags '$(Flags)' .
	@echo "✅ Local build completed: ./$(App)"

## run: Run the application with go run
run:
	@go run -tags $(Tags) .

## test: Run all tests
test:
	@echo -e "\n🧪 Running tests..."
	@go test -v ./...
	@echo "✅ Tests completed!"

## lint: Run golangci-lint
lint:
	@golangci-lint run

## network: Create Docker bridge network
network:
	@docker network inspect $(Network) >/dev/null 2>&1 || \
		(echo "Creating network $(Network)..." && docker network create $(Network))

## start-s: Start server container (creates network if missing)
start-s: network
	@echo -e "\n🚀 Starting $(App)-server container..."
	@docker run $(ServerCmd) $(App)

## start-c: Start client container (creates network if missing)
start-c: network
	@echo -e "\n🚀 Starting $(App)-client container..."
	@docker run $(ClientCmd) $(App)

## stop: Stop running containers
stop:
	@echo -e "\n🛑 Stopping $(App) containers..."
	@docker stop $(App)-server $(App)-client 2>/dev/null || true

## clean: Remove binary, Docker image, and network
clean:
	@echo -e "\n🧹 Cleaning up..."
	@docker rmi $(App) 2>/dev/null || true
	@docker network rm $(Network) 2>/dev/null || true
	@rm -f $(App)
	@echo "✅ Cleanup completed!"

## proto: Regenerate protobuf Go stubs
proto:
	@echo -e "\n📦 Generating protobuf Go stubs..."
	@protoc -I proto \
		--go_out=paths=source_relative:proto/pb \
		proto/*.proto
	@echo "✅ Proto generation completed"

## help: Show this help message
help:
	@echo -e "\n📖 Available targets:"
	@grep -E '^## ' Makefile | sed 's/## /  /'
	@echo ""
	@echo "📋 Configuration:"
	@echo "  Server Port:  $(ServerPort)"
	@echo "  Unix Socket:  $(UnixSocket)"
	@echo "  Network:      $(Network)"
	@echo "  Target Arch:  $(HostArch)"
	@echo ""

.PHONY: build build-local run test lint network start-s start-c stop clean proto help
