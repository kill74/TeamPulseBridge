SHELL := /bin/sh

TERRAFORM ?= terraform
CHECKOV ?= checkov
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck

.PHONY: help doctor env-init replay dev-setup dev-check precommit-install precommit-run verify ci-go ci-terraform ci-policy ci-smoke ci-local lint test race run up down logs tidy fmt docs-build docs-serve integration-test integration-test-queue integration-test-handlers integration-bench integration-docker integration-clean infra-help infra-init-backend-staging infra-init-backend-prod infra-plan-staging infra-plan-prod infra-deploy-staging infra-deploy-prod infra-destroy-staging infra-destroy-prod gitops-help gitops-render-staging gitops-render-prod gitops-render-argocd gitops-validate gitops-bootstrap

help:
	@echo "Application Targets:"
	@echo "  make doctor        - Verify local developer environment"
	@echo "  make env-init      - Create a local .env from .env.example if needed"
	@echo "  make replay FILE=<path>|EVENT_ID=<id> [REPLAY_ARGS='<args>'] - Replay a webhook payload"
	@echo "  make dev-setup     - Install local developer tooling and git hooks"
	@echo "  make dev-check     - Run fast local quality gates"
	@echo "  make ci-local      - Run local equivalents of push CI checks"
	@echo "  make precommit-install - Install pre-commit hooks"
	@echo "  make precommit-run - Run pre-commit hooks on all files"
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
	@echo "CI Parity Targets:"
	@echo "  make ci-go                    - Run local Go checks that mirror CI"
	@echo "  make ci-terraform             - Run terraform fmt/init/validate locally"
	@echo "  make ci-policy                - Run local Checkov policy checks"
	@echo "  make ci-smoke                 - Run the local docker compose smoke check"
	@echo "  make ci-local                 - Run ci-go + race + ci-terraform + ci-policy + ci-smoke"
	@echo ""
	@echo "Infrastructure Targets (see make infra-help for details):"
	@echo "  make infra-help              - Show infrastructure targets"
	@echo "  make infra-init-backend-*    - Initialize Terraform backend (*=staging|prod)"
	@echo "  make infra-plan-*            - Plan infrastructure deployment"
	@echo "  make infra-deploy-*          - Deploy infrastructure"
	@echo "  make infra-destroy-*         - Destroy infrastructure (⚠️ dangerous)"
	@echo ""
	@echo "GitOps Targets (see make gitops-help for details):"
	@echo "  make gitops-help             - Show GitOps targets"
	@echo "  make gitops-render-*         - Render kustomize overlays (*=staging|prod|argocd)"
	@echo "  make gitops-validate         - Validate all GitOps manifests"
	@echo "  make gitops-bootstrap        - Bootstrap Argo CD on GKE"

doctor:
	cd services/ingestion-gateway && GOCACHE="$$(pwd)/.gocache" GOTELEMETRY=off go run ./cmd/doctor

env-init:
	@if [ -f .env ]; then \
		echo ".env already exists"; \
	else \
		cp .env.example .env; \
		echo "Created .env from .env.example"; \
	fi

replay:
	@test -n "$(FILE)$(EVENT_ID)" || (echo "Either FILE or EVENT_ID is required. Example: make replay FILE=internal/handlers/testdata/contracts/github_pull_request_opened.json REPLAY_ARGS='-source github'" && exit 1)
	cd services/ingestion-gateway && GOCACHE="$$(pwd)/.gocache" GOTELEMETRY=off go run ./cmd/replay $(if $(FILE),-file "$(FILE)",) $(if $(EVENT_ID),-event-id "$(EVENT_ID)",) $(REPLAY_ARGS)

dev-setup:
	@command -v go >/dev/null 2>&1 || (echo "Go is required" && exit 1)
	@command -v python3 >/dev/null 2>&1 || (echo "python3 is required" && exit 1)
	@python3 -m pip install --upgrade pip pre-commit checkov==3.2.469
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@python3 -m pre_commit install --install-hooks
	@python3 -m pre_commit install -t pre-push
	@echo "Ensure $$(go env GOPATH)/bin is on your PATH for golangci-lint and govulncheck"
	@echo "Developer setup completed"

precommit-install:
	@command -v python3 >/dev/null 2>&1 || (echo "python3 is required" && exit 1)
	@python3 -m pip install --upgrade pip pre-commit
	@python3 -m pre_commit install --install-hooks
	@python3 -m pre_commit install -t pre-push

precommit-run:
	@command -v python3 >/dev/null 2>&1 || (echo "python3 is required" && exit 1)
	@python3 -m pre_commit run --all-files

dev-check:
	@$(MAKE) precommit-run
	@$(MAKE) verify

fmt:
	cd services/ingestion-gateway && gofmt -w ./cmd ./internal

lint:
	cd services/ingestion-gateway && golangci-lint run ./...

test:
	cd services/ingestion-gateway && go test ./...

race:
	cd services/ingestion-gateway && go test -race ./...

verify: fmt lint test race

ci-go:
	cd services/ingestion-gateway && test -z "$$(gofmt -l ./cmd ./internal)"
	cd services/ingestion-gateway && go vet ./...
	cd services/ingestion-gateway && go test ./...
	cd services/ingestion-gateway && $(GOLANGCI_LINT) run --config ../../.golangci.yml ./...
	cd services/ingestion-gateway && $(GOVULNCHECK) -format text ./...

ci-terraform:
	@set -e; \
	cd infrastructure/terraform; \
	tf_data_dir="$$(pwd)/.terraform.ci-local"; \
	trap 'rm -rf "$$tf_data_dir"' EXIT; \
	$(TERRAFORM) fmt -check -recursive; \
	TF_DATA_DIR="$$tf_data_dir" $(TERRAFORM) init -backend=false; \
	TF_DATA_DIR="$$tf_data_dir" $(TERRAFORM) validate

ci-policy:
	$(CHECKOV) --config-file .checkov.yaml --framework terraform --directory infrastructure/terraform
	$(CHECKOV) --config-file .checkov.yaml --framework kubernetes --directory deploy/k8s --directory deploy/gitops/argocd

ci-smoke:
	@set -e; \
	cleanup() { docker compose down -v >/dev/null 2>&1 || true; }; \
	trap cleanup EXIT; \
	cleanup; \
	docker compose up -d --build; \
	i=0; \
	while [ "$$i" -lt 40 ]; do \
		if curl -fsS http://localhost:8080/healthz >/dev/null; then \
			echo "healthz is up"; \
			curl -fsS http://localhost:8080/metrics | head -n 20; \
			docker compose ps; \
			exit 0; \
		fi; \
		i=$$((i + 1)); \
		sleep 3; \
	done; \
	echo "healthz did not become ready in time"; \
	docker compose ps; \
	docker compose logs --no-color --tail=200; \
	exit 1

ci-local: ci-go race ci-terraform ci-policy ci-smoke

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

# GitOps Targets

gitops-help:
	@echo "GitOps (Argo CD) Targets:"
	@echo ""
	@echo "Validation:"
	@echo "  make gitops-render-staging   - Render staging overlay"
	@echo "  make gitops-render-prod      - Render production overlay"
	@echo "  make gitops-render-argocd    - Render Argo CD bootstrap manifests"
	@echo "  make gitops-validate         - Render all GitOps manifests"
	@echo ""
	@echo "Bootstrap:"
	@echo "  make gitops-bootstrap PROJECT_ID=<id> CLUSTER=<name> REGION=<region> [REPO_URL=<url>] [REVISION=<ref>]"

gitops-render-staging:
	kubectl kustomize deploy/k8s/overlays/staging

gitops-render-prod:
	kubectl kustomize deploy/k8s/overlays/prod

gitops-render-argocd:
	kubectl kustomize deploy/gitops/argocd

gitops-validate: gitops-render-staging gitops-render-prod gitops-render-argocd
	@echo "GitOps manifests rendered successfully"

gitops-bootstrap:
	@test -n "$(PROJECT_ID)" || (echo "PROJECT_ID is required" && exit 1)
	@test -n "$(CLUSTER)" || (echo "CLUSTER is required" && exit 1)
	@test -n "$(REGION)" || (echo "REGION is required" && exit 1)
	bash infrastructure/scripts/bootstrap-gitops-argocd.sh $(PROJECT_ID) $(CLUSTER) $(REGION) $(REPO_URL) $(REVISION)
