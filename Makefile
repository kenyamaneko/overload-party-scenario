.PHONY: build test test-integration vet fmt run tidy down generate-types help

APP := overload-party-scenario

build: ## Build Docker image
	docker build -t $(APP) .

test: ## Run unit tests
	go test ./... -count=1 -race

test-integration: ## Run unit + integration tests (Testcontainers で Postgres / Pub/Sub emulator を起動するので Docker 必須。FIRESTORE_EMULATOR_HOST 設定時は Firestore も検証)
	go test -tags=integration ./... -count=1 -race

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy dependencies
	go mod tidy

fmt: ## Format code
	gofmt -s -w .

generate-types: ## Re-generate packages/api-scenario/{openapi,asyncapi}_gen.go from data/{openapi,asyncapi}.yaml (requires oapi-codegen and asyncapi-codegen on PATH)
	scripts/generate_types.sh

down: ## Stop the local stack and remove volumes
	HOST_GOMODCACHE=$$(go env GOMODCACHE) docker compose down -v

run: ## Run the full local stack (app + infra) in compose; edit source and restart `scenario` to reload
	GOWORK=off GOPRIVATE=github.com/kenyamaneko/* go mod download
	HOST_GOMODCACHE=$$(go env GOMODCACHE) docker compose up

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
