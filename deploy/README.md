# Deployment Layout

This directory contains deployment assets for Kubernetes runtime and GitOps operations.

## Structure

- `k8s/`: Kustomize base and environment overlays used to render workloads.
- `gitops/`: Argo CD application definitions and bootstrap manifests.
- `monitoring/`: Prometheus and Grafana configuration for local and cluster observability.

## Usage

- Validate manifests before publishing changes:
  - `make gitops-validate`
- Bootstrap Argo CD after cluster provisioning:
  - `make gitops-bootstrap PROJECT_ID=<project> CLUSTER=<cluster> REGION=<region>`

## Change Rules

- Keep base manifests environment-agnostic.
- Apply environment-specific changes only in overlays.
- Prefer additive patches over replacing full resources.
