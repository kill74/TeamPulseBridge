# GKE Cluster Module

Creates and manages a production-grade GKE cluster and workload node pool.

## Responsibility

- Provisions a VPC-native GKE cluster with autoscaling.
- Configures secure node settings (Shielded VMs and workload metadata mode).
- Creates a dedicated workload node pool for application scheduling.
- Optionally manages bootstrap Kubernetes resources.

## Key Inputs

- `cluster_name`, `region`, `environment`
- `network_name`, `subnetwork_name`
- `pods_secondary_range_name`, `services_secondary_range_name`
- Node pool and autoscaling settings (`initial/min/max`, machine types, disk sizes)
- `deletion_protection`

See `variables.tf` for full input contract.

## Outputs

Exposes cluster identity and access metadata for downstream modules.

See `outputs.tf` for complete output contract.

## Operational Notes

- Keep `deletion_protection=true` for production environments.
- Validate autoscaling limits against quota before rollout.
- Treat node pool machine type changes as capacity-impacting operations.
