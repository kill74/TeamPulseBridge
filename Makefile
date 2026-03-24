SHELL := /bin/sh

.PHONY: help verify lint test race run up down logs tidy fmt docs-build docs-serve infra-help infra-init-backend-staging infra-init-backend-prod infra-plan-staging infra-plan-prod infra-deploy-staging infra-deploy-prod infra-destroy-staging infra-destroy-prod

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
