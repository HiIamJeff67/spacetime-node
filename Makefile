KRATOS_THIRD_PARTY := $(shell go list -f '{{.Dir}}' -m github.com/go-kratos/kratos/v3)/third_party
COMPOSE_FILE ?= deploy/compose/compose.yaml
ENV_FILE ?= .env
MIGRATION_START ?= 000010
CATALOG_MIGRATIONS := $(shell find migrations/postgres -maxdepth 1 -type f -name '[0-9][0-9][0-9][0-9][0-9][0-9]_*.sql' -exec basename {} \; | sort | awk -F_ '$$1 >= "$(MIGRATION_START)"')
PROTO_FILES := api/proto/spacetime_node/v1/common.proto api/proto/spacetime_node/v1/errors.proto api/proto/spacetime_node/v1/journey.proto api/proto/spacetime_node/v1/redemption.proto api/proto/spacetime_node/v1/user.proto api/proto/spacetime_node/v1/notification.proto api/proto/spacetime_node/v1/mobility.proto

.PHONY: check compose-config migrate module-check openapi proto test vet

check: module-check test vet compose-config

module-check:
	go mod tidy -diff

test:
	go test ./...

vet:
	go vet ./...

compose-config:
	docker compose --env-file .env.example -f deploy/compose/compose.yaml config --quiet

migrate:
	@for migration in $(CATALOG_MIGRATIONS); do \
		docker compose --env-file $(ENV_FILE) -f $(COMPOSE_FILE) exec -T postgres \
			sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -f "/docker-entrypoint-initdb.d/$$1"' sh "$$migration"; \
	done
	@docker compose --env-file $(ENV_FILE) -f $(COMPOSE_FILE) restart embedding-indexer

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
