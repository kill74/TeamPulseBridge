# Multi-Region Active-Active Runbook

## Scope

This runbook covers rollout and operations for active-active deployment across two GCP regions for ingestion-gateway.

## Preconditions

- Terraform changes for multi-region are applied.
- Argo CD is bootstrapped on both clusters.
- Health checks and alerting are active in both regions.
- Traffic routing strategy is defined (global LB or DNS-based failover/weighted routing).

## Terraform Parameters

Set in environment tfvars:

- enable_multi_region = true
- region = "us-central1"
- secondary_region = "us-east1"
- secondary_gke_subnet_cidr
- secondary_pods_cidr
- secondary_services_cidr
- secondary_db_subnet_cidr
- secondary_app_domain (optional)

## Rollout Sequence

1. Apply infrastructure with multi-region enabled.
2. Validate both clusters are healthy and reachable.
3. Deploy application manifests to both regions.
4. Route a small percentage of traffic to secondary region.
5. Observe SLOs, error budget, and latency for 24h.
6. Increase traffic progressively to target split.

## Validation Checklist

- Both regions serve health endpoints successfully.
- Error ratio and burn rate remain within thresholds.
- Queue and downstream dependencies are healthy in both regions.
- Alerting detects regional degradations correctly.

## Incident Handling

### Regional Degradation

1. Shift traffic away from impacted region.
2. Keep healthy region serving full traffic.
3. Mitigate root cause in degraded region.
4. Restore gradually and rebalance traffic.

### Full Region Loss

1. Force traffic to surviving region.
2. Confirm capacity headroom and autoscaling behavior.
3. Monitor SLO burn and queue backpressure.
4. Start recovery work for lost region.

## Data Consistency Guidance

For write-heavy workflows, do not enable unrestricted dual-region writes unless your datastore model supports it.

Preferred patterns:

- Single-writer database with controlled failover.
- Conflict-safe multi-writer database.
- Idempotent event processing with replay support.

## Recovery Drills

Run quarterly game days:

1. Simulate region brownout.
2. Execute traffic shift runbook.
3. Measure RTO and SLO impact.
4. Capture lessons learned and update this runbook.
