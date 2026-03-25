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

## Release Flow

- Update image tag in staging overlay, validate, merge
- After validation, update image tag in prod overlay and merge
- Argo CD reconciles based on each application's sync policy
