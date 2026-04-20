# Regional Failover Drill

This drill validates that the production multi-region topology can fail over within a bounded recovery time objective.

## What It Covers

- pre-flight health validation for the primary and secondary regions
- execution of a failover action you choose for the environment
- measurement of failover recovery time against an explicit RTO SLO
- optional primary-region recovery validation after the drill

The repository already supports multi-region infrastructure in Terraform. This drill is the operational proof that the regions can actually recover under stress.

## Script

Use [regional-failover-drill.sh](../scripts/regional-failover-drill.sh).

Required environment variables:

- `PRIMARY_HEALTH_URL`
- `SECONDARY_HEALTH_URL`

Optional environment variables:

- `FAILOVER_HEALTH_URL`
- `FAIL_ACTION`
- `RECOVERY_ACTION`
- `RTO_SLO_SECONDS`
- `RECOVERY_SLO_SECONDS`
- `POLL_INTERVAL_SECONDS`
- `FAILOVER_TIMEOUT_SECONDS`
- `RECOVERY_TIMEOUT_SECONDS`

## Example

```bash
PRIMARY_HEALTH_URL="https://api.example.com/healthz" \
SECONDARY_HEALTH_URL="https://api-us-east1.example.com/healthz" \
FAILOVER_HEALTH_URL="https://api.example.com/healthz" \
FAIL_ACTION="kubectl --context gke_prod_us_central1 scale rollout ingestion-gateway -n ingestion-gateway-prod --replicas=0" \
RECOVERY_ACTION="kubectl --context gke_prod_us_central1 scale rollout ingestion-gateway -n ingestion-gateway-prod --replicas=3" \
RTO_SLO_SECONDS=300 \
bash infrastructure/scripts/regional-failover-drill.sh
```

## Recommended Drill Cadence

- run in staging after every meaningful failover-path change
- run in production on a scheduled basis with change control
- capture the measured RTO in the incident or resilience review

## Success Criteria

- secondary path remains healthy before the drill starts
- failover target recovers within the agreed RTO
- primary region recovers within the agreed recovery SLO after rollback
- operator notes include observed bottlenecks and manual steps that should be automated later
