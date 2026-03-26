# Terraform Root

This directory contains the Terraform root module and reusable modules for TeamPulse Bridge infrastructure.

## Layout

- `main.tf`, `variables.tf`, `outputs.tf`: root composition.
- `backend.tf`: remote state backend declaration (GCS).
- `providers.tf`: provider configuration.
- `environments/`: environment-specific backend and variable files.
- `modules/`: reusable infrastructure modules.

Module guides:

- `modules/networking/README.md`
- `modules/gke_cluster/README.md`
- `modules/security/README.md`
- `modules/storage/README.md`
- `modules/database/README.md`
- `modules/monitoring/README.md`

## Recommended Workflow

1. Select environment backend config from `environments/<env>/backend.conf`.
2. Initialize Terraform backend:
   - `terraform init -backend-config=environments/<env>/backend.conf`
3. Plan with environment vars:
   - `terraform plan -var-file=environments/<env>/terraform.tfvars`
4. Apply after review:
   - `terraform apply -var-file=environments/<env>/terraform.tfvars`

## Safety Rules

- Never commit real secrets in backend or tfvars files.
- Keep environment drift low by using module inputs instead of ad-hoc edits.
- Run `terraform fmt -recursive` before opening a pull request.
