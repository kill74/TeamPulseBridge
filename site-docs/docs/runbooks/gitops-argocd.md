# GitOps with Argo CD

This runbook defines how TeamPulse Bridge is deployed to Kubernetes using Argo CD.

## Objectives

- Keep cluster state converged with Git state
- Separate release velocity between staging and production
- Eliminate manual kubectl drift in day-2 operations

## Repository Layout

- `deploy/k8s/base`: shared Kubernetes manifests
- `deploy/k8s/overlays/staging`: staging-specific changes
- `deploy/k8s/overlays/prod`: production-specific changes
- `deploy/gitops/argocd`: Argo CD project and app-of-apps bootstrap

## Prerequisites

- Existing GKE cluster (staging or production)
- `gcloud` authenticated with cluster-admin level bootstrap permissions
- `kubectl` installed
- Access to the Git repository

## Bootstrap Argo CD

```bash
cd infrastructure/scripts
./bootstrap-gitops-argocd.sh <gcp-project-id> <gke-cluster-name> <gke-region>
```

What this script does:

1. Configures kubectl context for the target cluster
2. Installs Argo CD into namespace `argocd`
3. Applies TeamPulse `AppProject` and root application
4. Registers environment applications (`staging` and `prod`)

## Sync Strategy

- Staging application (`ingestion-gateway-staging`): automated sync enabled
- Production application (`ingestion-gateway-prod`): manual sync gate
- Namespace targets: `ingestion-gateway-staging` and `ingestion-gateway-prod`

This model gives fast feedback in staging while protecting production from accidental rollouts.

## Promote a New Release

1. Update image tag in staging overlay at `deploy/k8s/overlays/staging/kustomization.yaml`
2. Validate manifests locally:

   ```bash
   make gitops-validate
   ```

3. Commit and merge
4. Verify staging health and metrics
5. Update production tag at `deploy/k8s/overlays/prod/kustomization.yaml`
6. Commit and merge
7. Trigger production sync in Argo CD after approval

## Operational Guardrails

- Never edit Kubernetes resources directly in production namespaces
- Keep all config and scaling changes in Git overlays
- Rotate secret values via secret management workflow and GitOps-safe mechanism
- Require peer review for production overlay changes

## Verification Commands

```bash
# Render manifests
make gitops-render-staging
make gitops-render-prod
make gitops-render-argocd

# Argo CD app status
kubectl -n argocd get applications

# Workload health
kubectl -n ingestion-gateway-staging get deploy,po,svc,hpa,pdb
kubectl -n ingestion-gateway-prod get deploy,po,svc,hpa,pdb
```
