# Security Incident Triage

## Objective

Respond quickly when the ingestion gateway starts rejecting traffic for security reasons or when admin endpoint abuse is suspected.

This runbook complements the standard SLO runbook with a security-specific workflow:

- dashboard: `deploy/monitoring/grafana/dashboards/security-operations.json`
- rules: `deploy/monitoring/prometheus-rules.yml`
- audit API: `GET /admin/events/security-audit`

## Primary Signals

- `IngestionGatewaySecurityRejectionFastBurn`
- `IngestionGatewaySecurityRejectionSlowBurn`
- `IngestionGatewaySecurityRejectionsBurst`
- `IngestionGatewayPotentialAdminAbuse`

Derived metrics to inspect first:

- `teampulse:security_rejections:rate5m`
- `teampulse:security_rejections_ratio:5m`
- `teampulse:security_rejections_ratio:1h`
- `teampulse:security_rejection_budget_burn:5m`
- `teampulse:security_rejection_budget_burn:1h`
- `teampulse:security_rejection_budget_burn:6h`

The anomaly budget assumes security rejections should stay below roughly `0.2%` of handled webhook traffic over time. That is intentionally stricter than pure availability telemetry, but still tolerant of occasional secret drift and provider retry noise.

## First 10 Minutes

1. Open the Grafana Security Operations dashboard and confirm whether the issue is source-specific, path-specific, or global.
2. Check the alert timeline panel to see whether the event is a short spike, a sustained burn, or isolated to admin abuse.
3. Pull recent security audit records and identify top client IPs, actors, reasons, and paths.
4. Validate whether the pattern matches a rollout/config regression or a hostile probe.
5. If the issue targets `/admin/*`, tighten access first and investigate second.

## Automation Snippets

### Bash: fetch the most recent security audit records

```bash
export TPB_BASE_URL="http://127.0.0.1:8080"
export TPB_ADMIN_TOKEN="replace-me"

curl -fsS \
  -H "Authorization: Bearer ${TPB_ADMIN_TOKEN}" \
  "${TPB_BASE_URL}/admin/events/security-audit?limit=100"
```

### Bash: summarize top rejection reasons

```bash
curl -fsS \
  -H "Authorization: Bearer ${TPB_ADMIN_TOKEN}" \
  "${TPB_BASE_URL}/admin/events/security-audit?limit=200" \
| jq -r '.records[] | .reason' \
| sort | uniq -c | sort -nr
```

### Bash: summarize top client IPs and actors

```bash
curl -fsS \
  -H "Authorization: Bearer ${TPB_ADMIN_TOKEN}" \
  "${TPB_BASE_URL}/admin/events/security-audit?limit=200" \
| jq -r '.records[] | [.client_ip // "unknown", .actor // "unknown", .reason, .path] | @tsv' \
| sort | uniq -c | sort -nr | head -20
```

### Bash: isolate the last 15 minutes of admin abuse

```bash
cutoff="$(date -u -d '15 minutes ago' +%Y-%m-%dT%H:%M:%SZ)"

curl -fsS \
  -H "Authorization: Bearer ${TPB_ADMIN_TOKEN}" \
  "${TPB_BASE_URL}/admin/events/security-audit?limit=300" \
| jq --arg cutoff "$cutoff" '
  .records[]
  | select(.occurred_at >= $cutoff)
  | select(.path | startswith("/admin"))
  | {occurred_at, client_ip, actor, reason, path, http_status, request_id}
'
```

### PowerShell: fetch and group recent offenders

```powershell
$baseUrl = "http://127.0.0.1:8080"
$token = "replace-me"

$response = Invoke-RestMethod `
  -Headers @{ Authorization = "Bearer $token" } `
  -Uri "$baseUrl/admin/events/security-audit?limit=200"

$response.records |
  Group-Object client_ip, reason |
  Sort-Object Count -Descending |
  Select-Object -First 15
```

### PowerShell: isolate admin endpoint rejections

```powershell
$baseUrl = "http://127.0.0.1:8080"
$token = "replace-me"

$records = (Invoke-RestMethod `
  -Headers @{ Authorization = "Bearer $token" } `
  -Uri "$baseUrl/admin/events/security-audit?limit=200").records

$records |
  Where-Object { $_.path -like "/admin*" } |
  Sort-Object occurred_at -Descending |
  Select-Object occurred_at, client_ip, actor, reason, path, http_status, request_id
```

## Investigation Notes

- Use Prometheus and Grafana for low-cardinality offender views such as `reason`, `source`, `path`, and `status`.
- Use the security audit API for high-cardinality triage such as `client_ip`, `actor`, and `request_id`.
- Do not add `client_ip` or `actor` as Prometheus metric labels. That would create cardinality pressure exactly when the service is under attack.

## Response Guidance

If the event looks like config drift:

- verify secret rotation or ingress/header changes
- confirm trusted proxy CIDRs and admin JWT settings
- compare staging vs prod overlays for unintended auth differences

If the event looks like abuse:

- confirm admin CIDR allowlist coverage
- tighten rate limits or upstream edge controls if needed
- preserve the security audit stream for evidence before pruning or manual cleanup

If the event is provider-specific:

- check whether one provider secret is stale
- inspect the top reject path and reason in the dashboard
- validate against webhook smoke tooling only after restoring the correct auth boundary
