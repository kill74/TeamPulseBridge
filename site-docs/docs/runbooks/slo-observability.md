# SLO and Observability Runbook

## Objective

Operate Ingestion Gateway with production-grade SLO monitoring focused on availability and error budget consumption.

## SLO Definition

- Service: ingestion-gateway
- SLI: successful webhook ingestion responses (non-5xx)
- SLO target: 99.9% monthly availability (30d rolling window)
- Error budget: 0.1% failed requests (5xx)

Formula:

- Availability = 1 - (5xx / total)

## Key Metrics

Prometheus recording rules are defined in:

- deploy/monitoring/prometheus-rules.yml

Primary derived series:

- teampulse:webhook_error_ratio:5m
- teampulse:webhook_error_ratio:1h
- teampulse:webhook_error_ratio:6h
- teampulse:webhook_error_budget_burn:5m
- teampulse:webhook_error_budget_burn:1h
- teampulse:webhook_error_budget_burn:6h
- teampulse:webhook_availability:30d
- teampulse:webhook_error_budget_consumed:30d
- teampulse:security_rejections_ratio:5m
- teampulse:security_rejection_budget_burn:5m
- teampulse:security_rejection_budget_burn:1h
- teampulse:security_rejection_budget_burn:6h

## Alerts

- IngestionGatewaySLOErrorBudgetFastBurn (critical)
  - Condition: burn rate above 14.4 on 5m and 1h windows
  - Action: immediate investigation and mitigation

- IngestionGatewaySLOErrorBudgetSlowBurn (warning)
  - Condition: burn rate above 3 on 1h and 6h windows
  - Action: create incident ticket and prioritize corrective actions

- IngestionGatewaySecurityRejectionFastBurn (critical)
  - Condition: security rejection anomaly budget burn above 14.4 on 5m and 1h windows
  - Action: switch to security triage mode and inspect the Security Operations dashboard immediately

- IngestionGatewaySecurityRejectionSlowBurn (warning)
  - Condition: security rejection anomaly budget burn above 6 on 1h and 6h windows
  - Action: investigate source/path/reason drift before the issue turns into a sustained incident

## Dashboard

Use Grafana dashboard:

- deploy/monitoring/grafana/dashboards/ingestion-slo.json
- deploy/monitoring/grafana/dashboards/security-operations.json

Look at:

1. Availability 30d
2. Error Budget Consumed 30d
3. Burn Rate 5m and 1h
4. Error ratio trend across 5m/1h/6h
5. Security reject ratio and burn trend overlays when security alerts are firing

## Triage Guide

1. Check whether errors are global or source-specific (github/slack/gitlab/teams).
2. Correlate 5xx spikes with deploys and dependency health.
3. Verify queue backend behavior and publish latency/failures.
4. If fast burn is active, enforce mitigation first, root-cause second.
5. If security rejection burn is active, pivot to the Security Incident Triage runbook before changing auth settings under pressure.

## Local Validation

1. Start stack:

   docker compose up -d ingestion-gateway prometheus grafana

2. Open Grafana:

   http://localhost:3000

3. Confirm dashboard metrics are populated and alerts evaluate.

## Operational Notes

- Dashboards use a fixed Prometheus datasource UID: prometheus.
- Keep thresholds aligned with SLO target if target changes.
- Revisit burn-rate thresholds after at least two production incident cycles.
- Security offender analysis should stay low-cardinality in Prometheus; use the admin security audit API for client IP and actor drill-down.
