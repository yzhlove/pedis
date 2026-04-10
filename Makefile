SHELL=/bin/bash

proto:
	@echo "Generating protobuf Go stubs..."
	@protoc -I proto \
		--go_out=paths=source_relative:proto/pb \
		proto/*.proto
	@echo "Proto generation completed"

build:
	go build ./...

test:
	go test ./...

.PHONY: proto build test
