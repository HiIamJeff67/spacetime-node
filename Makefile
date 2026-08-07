KRATOS_THIRD_PARTY := $(shell go list -f '{{.Dir}}' -m github.com/go-kratos/kratos/v3)/third_party
PROTO_FILES := api/proto/spacetime_node/v1/common.proto api/proto/spacetime_node/v1/errors.proto api/proto/spacetime_node/v1/journey.proto api/proto/spacetime_node/v1/redemption.proto

.PHONY: openapi proto

proto:
	PATH="$(shell go env GOPATH)/bin:$(PATH)" protoc -I . -I $(KRATOS_THIRD_PARTY) \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		--go-http_out=paths=source_relative:. \
		--go-errors_out=paths=source_relative:. \
		$(PROTO_FILES)

openapi:
	protoc -I . -I $(KRATOS_THIRD_PARTY) \
		--openapi_out="title=Spacetime Node API,version=0.1.0,description=Demo API contract for the Spacetime Node backend,naming=proto,enum_type=string:api/openapi" \
		$(PROTO_FILES)
