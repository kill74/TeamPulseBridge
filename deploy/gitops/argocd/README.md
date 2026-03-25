# Argo CD GitOps Bootstrap

This folder contains the Argo CD control-plane declarations for TeamPulse Bridge.

## Resources

- `project.yaml`: AppProject boundaries and allowed sources/destinations
- `root-app.yaml`: app-of-apps that discovers child applications in `apps/`
- `apps/staging.yaml`: staging app (auto-sync)
- `apps/prod.yaml`: production app (manual sync gate)

## Apply

```bash
kubectl apply -k deploy/gitops/argocd
```

Use `infrastructure/scripts/bootstrap-gitops-argocd.sh` for complete cluster bootstrap.
