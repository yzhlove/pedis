SHELL=/bin/bash

# Application Configuration
App:=pedis
Timezone:=Asia/Shanghai
Tags:=develop
Dockerfile:=Dockerfile
Network:=pedis_net
ServerPort:=6399
UnixSocket:=/tmp/pedis.sock

# Repo root directory (works regardless of where `make` is invoked from)
RepoRoot:=$(abspath $(dir $(lastword $(MAKEFILE_LIST))))

# Build flags with embedded metadata
BuildDate:=$(shell date '+%F_%T')
GitCommit:=$(shell git describe --tags --always --dirty=-dev)
GoVersion:=$(shell go version | cut -d' ' -f3)
AppVersion:=$(GitCommit)

Flags:="-X main.buildDate=$(BuildDate) -X main.goVersion=$(GoVersion) -X main.appVersion=$(AppVersion)"

# Build targets
build:
	@ echo -e "\n🔨 Building $(App) application..."
	@ echo "   Build Date: $(BuildDate)"
	@ echo "   Git Commit: $(GitCommit)"
	@ echo "   Go Version: $(GoVersion)"
	@ CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags $(Tags) -o $(App) -ldflags $(Flags) ./cmd/pedis/
	@ echo -e "\n🐳 Building Docker image..."
	@ docker rmi $(App) 2>/dev/null || true
	@ docker build --platform linux/amd64 --no-cache -t $(App) -f $(Dockerfile) .
	@ rm -f $(App)
	@ echo "✅ Build completed successfully!"

# Development build (local binary)
build-local:
	@ echo -e "\n🔨 Building local $(App) binary..."
	@ go build -tags $(Tags) -o $(App) -ldflags $(Flags) ./cmd/pedis/
	@ echo "✅ Local build completed: ./$(App)"

run:
	@ go run ./cmd/pedis/

test:
	@ go test ./...

stop:
	@ echo -e "\n🛑 Stopping $(App) container..."
	@ docker stop $(App) 2>/dev/null || true
	@ docker rm $(App) 2>/dev/null || true

clean: stop
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
