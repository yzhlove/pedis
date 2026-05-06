SHELL=/bin/bash

# Application Configuration
App:=pedis
Timezone:=Asia/Shanghai
Tags:=develop
Dockerfile:=Dockerfile
Network:=mynet
ServerPort:=6399
UnixContainerDir:=/tmp
UnixLocalDir:=/tmp
UnixSocket:=$(UnixContainerDir)/pedis.sock
Env:=dev
ClientName:=yurisa

# Repo root directory (works regardless of where `make` is invoked from)
RepoRoot:=$(abspath $(dir $(lastword $(MAKEFILE_LIST))))

# Build flags with embedded metadata
BuildDate:=$(shell date '+%F_%T')
GitCommit:=$(shell git describe --tags --always --dirty=-dev)
GoVersion:=$(shell go version | cut -d' ' -f3)
AppVersion:=$(GitCommit)

Flags:="-X main.buildDate=$(BuildDate) -X main.goVersion=$(GoVersion) -X main.appVersion=$(AppVersion)"

ClientCmd:--rm=true -it \
	--network=$(Network) \
	--name=$(App)-client \
	-h $(App)-client \
	-e TZ=$(Timezone) \
	-e PEDIS_ENV=$(Env) \
	-e PEDIS_ROLE=client \
	-e PEDIS_CLI_NAME=$(ClientName) \
	-e PEDIS_UNIX_SOCKET=$(UnixSocket) \
	-v $(UnixContainerDir):$(UnixLocalDir)

ServerCmd:=--rm=true -it \
	--network=$(Network) \
    --name=$(App)-server \
    -h $(App)-server \
    -e TZ=$(Timezone) \
    -e PEDIS_ENV=$(Env) \
    -e PEDIS_ROLE=server \
    -e PEDIS_SERVER_PORT=$(ServerPort) \
    -e PEDIS_UNIX_SOCKET=$(UnixSocket) \
    -v $(UnixContainerDir):$(UnixLocalDir)

# Build targets
build:
	@ echo -e "\n🔨 Building $(App) application..."
	@ echo "   Build Date: $(BuildDate)"
	@ echo "   Git Commit: $(GitCommit)"
	@ echo "   Go Version: $(GoVersion)"
	@ CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags $(Tags) -o $(App) -ldflags $(Flags) .
	@ echo -e "\n🐳 Building Docker image..."
	@ docker rmi $(App) 2>/dev/null || true
	@ docker build --platform linux/arm64 --no-cache -t $(App) -f $(Dockerfile) .
	@ rm -f $(App)
	@ echo "✅ Build completed successfully!"

start-s:
	@ echo -e "\n🚀 Starting $(App)-server container..."
	@ echo -e "docker run $(ServerCmd) $(App)"
	@ docker run $(ServerCmd) $(App)

start-c:
	@ echo -e "\n🚀 Starting $(App)-client container..."
	@ docker run $(ClientCmd) $(App)

stop:
	@ echo -e "\n🛑 Stopping $(App) containers..."

clean:
	@ echo -e "\n🧹 Cleaning up..."
	@ docker rmi $(App) 2>/dev/null || true
	@ docker network rm $(Network) 2>/dev/null || true
	@ rm -f $(App)
	@ echo "✅ Cleanup completed!"

proto:
	@ echo -e "\n📦 Generating protobuf Go stubs..."
	@ protoc -I proto \
		--go_out=paths=source_relative:proto/pb \
		proto/*.proto
	@ echo "✅ Proto generation completed"

help:
	@ echo -e "\n📖 Available targets:"
	@ echo "  build         - Build cross-compiled binary and Docker image"
	@ echo "  build-local   - Build local binary for development"
	@ echo "  run           - Run the application with go run"
	@ echo "  test          - Run all tests"
	@ echo "  stop          - Stop running container"
	@ echo "  clean         - Stop container and clean up resources"
	@ echo "  proto         - Regenerate protobuf Go stubs"
	@ echo "  help          - Show this help message"
	@ echo ""
	@ echo "📋 Configuration:"
	@ echo "  Server Port:  $(ServerPort)"
	@ echo "  Unix Socket:  $(UnixSocket)"
	@ echo "  Network:      $(Network)"
	@ echo ""

.PHONY: build build-local run test stop clean proto help
