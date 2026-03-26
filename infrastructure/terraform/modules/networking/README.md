# Networking Module

Creates the network foundation for runtime and data-plane resources.

## Responsibility

- Creates the VPC used by workloads.
- Provisions GKE and optional database subnets.
- Configures Cloud Router and Cloud NAT for egress.
- Applies ingress firewall rules for internal traffic, health checks, and HTTP/HTTPS.
- Creates private service networking connection for Cloud SQL private IP.
- Creates Cloud Armor policy with baseline SQLi and rate-limit protections.

## Key Inputs

- `network_name`, `region`, `environment`
- `gke_subnet_cidr`, `pods_cidr`, `services_cidr`
- `create_db_subnet`, `db_subnet_cidr`
- `enable_ssh_access`, `ssh_source_ranges`

See `variables.tf` for full input contract.

## Outputs

Exposes network and subnet identifiers consumed by cluster and database modules.

See `outputs.tf` for complete output contract.

## Operational Notes

- Restrict `ssh_source_ranges` to approved bastion or corporate CIDRs.
- Keep CIDR planning consistent across regions to avoid overlap.
- Review Cloud Armor policy changes with security owners before production rollout.
