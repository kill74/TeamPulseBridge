# Kubernetes Manifests

Kustomize layout for ingestion-gateway.

## Structure

- `base`: common manifests for all environments
- `overlays/staging`: staging-specific scaling and image tag
- `overlays/prod`: production-specific scaling, limits, and image tag
- Namespaces: `ingestion-gateway-staging` and `ingestion-gateway-prod`

## Render

```bash
kubectl kustomize deploy/k8s/overlays/staging
kubectl kustomize deploy/k8s/overlays/prod
```

CI renders both overlays before policy checks run, so guardrails apply to the final manifests that Argo Rollouts and the HPA will actually consume rather than only to the raw patch files.

## Release Flow

- Update image tag in staging overlay, validate, merge
- After validation, update image tag in prod overlay and merge
- Argo CD reconciles based on each application's sync policy
- Rollouts use canary steps plus Prometheus analysis gates before full promotion

## Rollout Model

- the base workload is an Argo Rollout instead of a plain Deployment
- staging uses a faster 25% then 60% promotion path
- production uses 10% then 25% then 50% promotion with analysis between steps
- rollback is automatic when the Prometheus canary gates fail on error budget or queue resilience
