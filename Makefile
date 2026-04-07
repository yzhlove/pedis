SHELL=/bin/bash

proto:
	@echo -e "\n📦 Generating protobuf Go stubs..."
	@protoc -I app/proto/pbfiles \
		--go_out=paths=source_relative:app/proto/pb \
		app/proto/pbfiles/*.proto
	@echo "✅ Proto generation completed"


.PHONY: proto