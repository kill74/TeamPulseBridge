SHELL := /bin/sh

.PHONY: help verify lint test race run up down logs tidy fmt docs-build docs-serve integration-test integration-test-queue integration-test-handlers integration-bench integration-docker integration-clean infra-help infra-init-backend-staging infra-init-backend-prod infra-plan-staging infra-plan-prod infra-deploy-staging infra-deploy-prod infra-destroy-staging infra-destroy-prod

help:
	@echo "Application Targets:"
	@echo "  make fmt           - Format all Go code"
	@echo "  make lint          - Run golangci-lint"
	@echo "  make test          - Run unit tests"
	@echo "  make race          - Run race detector tests"
	@echo "  make verify        - fmt + lint + test + race"
	@echo "  make run           - Run ingestion gateway locally"
	@echo "  make up            - Start local docker stack"
	@echo "  make down          - Stop local docker stack"
	@echo "  make logs          - Tail docker compose logs"
	@echo "  make tidy          - Run go mod tidy"
	@echo "  make docs-build    - Build MkDocs documentation"
	@echo "  make docs-serve    - Serve documentation locally"
	@echo ""
	@echo "Integration Testing Targets:"
	@echo "  make integration-test         - Run all integration tests"
	@echo "  make integration-test-queue   - Run queue integration tests"
	@echo "  make integration-test-handlers - Run webhooks integration tests"
	@echo "  make integration-bench        - Run integration benchmarks"
	@echo "  make integration-docker       - Run integration tests via docker-compose"
	@echo "  make integration-clean        - Clean integration test resources"
	@echo ""
	@echo "Infrastructure Targets (see make infra-help for details):"
	@echo "  make infra-help              - Show infrastructure targets"
	@echo "  make infra-init-backend-*    - Initialize Terraform backend (*=staging|prod)"
	@echo "  make infra-plan-*            - Plan infrastructure deployment"
	@echo "  make infra-deploy-*          - Deploy infrastructure"
	@echo "  make infra-destroy-*         - Destroy infrastructure (⚠️ dangerous)"

fmt:
	cd services/ingestion-gateway && gofmt -w ./cmd ./internal

lint:
	cd services/ingestion-gateway && golangci-lint run ./...

test:
	cd services/ingestion-gateway && go test ./...

race:
	cd services/ingestion-gateway && go test -race ./...

verify: fmt lint test race

run:
	cd services/ingestion-gateway && go run ./cmd/server

up:
	docker compose up -d --build

down:
	docker compose down -v

logs:
	docker compose logs -f --tail=100

tidy:
	cd services/ingestion-gateway && go mod tidy

docs-build:
	cd site-docs && pip install -r requirements.txt && mkdocs build --strict

docs-serve:
	cd site-docs && pip install -r requirements.txt && mkdocs serve

# Integration Testing Targets

integration-test:
	@echo "Starting Pub/Sub Emulator..."
	@set -e; \
	cleanup() { \
		docker stop pubsub-emulator-make > /dev/null 2>&1 || true; \
		docker rm pubsub-emulator-make > /dev/null 2>&1 || true; \
	}; \
	trap cleanup EXIT; \
	docker rm -f pubsub-emulator-make > /dev/null 2>&1 || true; \
	docker run -d --name pubsub-emulator-make -p 8085:8085 google/cloud-sdk:emulators gcloud beta emulators pubsub start --host-port=0.0.0.0:8085 > /dev/null; \
	sleep 3; \
	cd services/ingestion-gateway; \
	PUBSUB_EMULATOR_HOST=localhost:8085 PUBSUB_PROJECT_ID=test-project go test -v -race -coverpkg=./... -coverprofile=coverage.out -timeout 30s ./...

integration-test-queue:
	@echo "Starting Pub/Sub Emulator..."
	@set -e; \
	cleanup() { \
		docker stop pubsub-emulator-queue > /dev/null 2>&1 || true; \
		docker rm pubsub-emulator-queue > /dev/null 2>&1 || true; \
	}; \
	trap cleanup EXIT; \
	docker rm -f pubsub-emulator-queue > /dev/null 2>&1 || true; \
	docker run -d --name pubsub-emulator-queue -p 8085:8085 google/cloud-sdk:emulators gcloud beta emulators pubsub start --host-port=0.0.0.0:8085 > /dev/null; \
	sleep 3; \
	cd services/ingestion-gateway; \
	PUBSUB_EMULATOR_HOST=localhost:8085 PUBSUB_PROJECT_ID=test-project go test -v -race -timeout 30s ./internal/queue

integration-test-handlers:
	@echo "Starting Pub/Sub Emulator..."
	@set -e; \
	cleanup() { \
		docker stop pubsub-emulator-handlers > /dev/null 2>&1 || true; \
		docker rm pubsub-emulator-handlers > /dev/null 2>&1 || true; \
	}; \
	trap cleanup EXIT; \
	docker rm -f pubsub-emulator-handlers > /dev/null 2>&1 || true; \
	docker run -d --name pubsub-emulator-handlers -p 8085:8085 google/cloud-sdk:emulators gcloud beta emulators pubsub start --host-port=0.0.0.0:8085 > /dev/null; \
	sleep 3; \
	cd services/ingestion-gateway; \
	PUBSUB_EMULATOR_HOST=localhost:8085 PUBSUB_PROJECT_ID=test-project go test -v -race -timeout 30s ./internal/handlers

integration-bench:
	@echo "Starting Pub/Sub Emulator..."
	@set -e; \
	cleanup() { \
		docker stop pubsub-emulator-bench > /dev/null 2>&1 || true; \
		docker rm pubsub-emulator-bench > /dev/null 2>&1 || true; \
	}; \
	trap cleanup EXIT; \
	docker rm -f pubsub-emulator-bench > /dev/null 2>&1 || true; \
	docker run -d --name pubsub-emulator-bench -p 8085:8085 google/cloud-sdk:emulators gcloud beta emulators pubsub start --host-port=0.0.0.0:8085 > /dev/null; \
	sleep 3; \
	cd services/ingestion-gateway; \
	PUBSUB_EMULATOR_HOST=localhost:8085 PUBSUB_PROJECT_ID=test-project go test -v -run=^$ -bench=. -benchmem -benchtime=5s ./internal/handlers ./internal/queue

integration-docker:
	cd services/ingestion-gateway && docker compose -f docker-compose.integration.yml up -d --build
	@sleep 5
	@echo "Services running. Health checks:"
	cd services/ingestion-gateway && docker compose -f docker-compose.integration.yml ps
	@curl -f http://localhost:8080/healthz || echo "Gateway not ready yet"
	@curl -f http://localhost:8085/v1/projects/test-project || echo "Pub/Sub emulator check failed"
	@echo "Integration environment ready. Run 'make integration-clean' to stop."

integration-clean:
	cd services/ingestion-gateway && docker compose -f docker-compose.integration.yml down -v
	@docker stop pubsub-emulator-make pubsub-emulator-queue pubsub-emulator-handlers pubsub-emulator-bench 2>/dev/null || true
	@docker rm pubsub-emulator-make pubsub-emulator-queue pubsub-emulator-handlers pubsub-emulator-bench 2>/dev/null || true

# Infrastructure Targets

infra-help:
	@echo "Infrastructure as Code (Terraform) Targets:"
	@echo ""
	@echo "Setup:"
	@echo "  make infra-init-backend-staging  - Initialize Terraform backend for staging"
	@echo "  make infra-init-backend-prod     - Initialize Terraform backend for production"
	@echo ""
	@echo "Planning:"
	@echo "  make infra-plan-staging          - Plan staging infrastructure changes"
	@echo "  make infra-plan-prod             - Plan production infrastructure changes"
	@echo ""
	@echo "Deployment:"
	@echo "  make infra-deploy-staging        - Deploy staging infrastructure"
	@echo "  make infra-deploy-prod           - Deploy production infrastructure (requires confirmation)"
	@echo ""
	@echo "Destruction (⚠️ WARNING: Deletes all resources and data):"
	@echo "  make infra-destroy-staging       - Destroy staging infrastructure"
	@echo "  make infra-destroy-prod          - Destroy production infrastructure"
	@echo ""
	@echo "Documentation:"
	@echo "  Complete guide: infrastructure/docs/README.md"

infra-init-backend-staging:
	cd infrastructure/scripts && bash init-backend.sh staging

infra-init-backend-prod:
	cd infrastructure/scripts && bash init-backend.sh prod

infra-plan-staging:
	cd infrastructure/terraform && terraform plan -var-file=environments/staging/terraform.tfvars

infra-plan-prod:
	cd infrastructure/terraform && terraform plan -var-file=environments/prod/terraform.tfvars

infra-deploy-staging:
	cd infrastructure/scripts && bash deploy.sh staging

infra-deploy-prod:
	cd infrastructure/scripts && bash deploy.sh prod

infra-destroy-staging:
	cd infrastructure/scripts && bash destroy.sh staging

infra-destroy-prod:
	cd infrastructure/scripts && bash destroy.sh prod
