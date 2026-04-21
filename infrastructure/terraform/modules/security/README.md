# Security Module

Defines workload identity, IAM bindings, and optional Kubernetes security controls.

## Responsibility

- Creates service accounts for nodes, workloads, database access, and Pub/Sub.
- Configures Workload Identity bindings between Kubernetes and GCP identities.
- Grants least-privilege IAM roles required by runtime services.
- Optionally applies Kubernetes RBAC, network policies, and pod security policy resources.

## Key Inputs

- `cluster_name`, `app_name`, `environment`, `gcp_project`
- `namespace`, `ksa_name`
- `pubsub_role`, `permissions`
- `enable_network_policies`, `enable_pod_security_policy`
- `https_egress_cidrs`, `db_egress_cidrs`

See `variables.tf` for full input contract.

## Outputs

Exposes service-account identifiers and security artifacts used by other modules.

See `outputs.tf` for complete output contract.

## Operational Notes

- Prefer Workload Identity over long-lived service account keys.
- Keep `permissions` minimal and review every non-default role.
- Production IAM exceptions are time-bounded automatically from the expiry date embedded in `production_iam_exception_justification`.
- The workload no longer receives default `roles/iam.serviceAccountUser` impersonation over the Pub/Sub service account.
- Treat network policy changes as availability-sensitive.
