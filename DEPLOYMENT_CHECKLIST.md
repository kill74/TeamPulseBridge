# Security Upgrade Checklist & Deployment Guide

## Pre-Deployment Security Checklist

### Verification Steps (Complete Before Deploying)

#### 1. Build & Test
- [ ] Run `go build ./...` to verify compilation
- [ ] Run `go test ./...` for unit tests
- [ ] Run integration tests with Pub/Sub
- [ ] Run e2e tests if available

#### 2. Security Scanning
```bash
# Run vulnerability check
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
# Expected: "No vulnerabilities found."
```
- [ ] govulncheck passes with zero vulnerabilities
- [ ] No warnings or deprecation notices

#### 3. Dependency Verification
```bash
# Verify go.mod is clean
go mod verify
# Expected: "all modules verified"

# Check for unused dependencies
go mod tidy
# Commit the result if changed
```
- [ ] `go mod verify` passes
- [ ] `go mod tidy` shows no changes needed

#### 4. gRPC Functionality Tests
Since gRPC is a core dependency, verify:
- [ ] Can establish gRPC connections to upstream services
- [ ] Can create/close Pub/Sub client connections
- [ ] Can publish messages to Pub/Sub
- [ ] Can subscribe and receive messages from Pub/Sub
- [ ] Authorization rules still work as expected

#### 5. OpenTelemetry Tests
- [ ] Tracing exports are being sent correctly
- [ ] No errors in trace collection
- [ ] Spans are properly formatted
- [ ] Metrics are being collected

#### 6. Documentation Review
- [ ] cloud.google.com/go/pubsub migration guide reviewed
- [ ] Any API breaking changes documented
- [ ] Team notified of changes

---

## Deployment Strategy

### Option 1: Production Deployment (Recommended)
**Timeline:** Proceed immediately after checklist

```
1. Merge upgraded dependencies to main branch
2. Deploy to staging environment
3. Run integration tests for 24 hours
4. Deploy to production during low-traffic window
5. Monitor error rates and logs
6. Keep rollback plan ready for 48 hours
```

### Option 2: Canary Deployment
**Timeline:** Safer for high-traffic systems

```
1. Deploy to 5-10% of traffic
2. Monitor for 24 hours
3. If stable, increase to 25%
4. Monitor for 24 hours
5. If stable, increase to 100%
```

### Option 3: Blue-Green Deployment
**Timeline:** Zero-downtime deployment

```
1. Deploy new version to separate environment
2. Run parallel tests with both versions
3. Switch load balancer to new version
4. Keep old version running for 24-48 hours
5. Decommission old version when stable
```

---

## Post-Deployment Monitoring

### Immediate (First 24 Hours)
- [ ] Monitor error rates and logs
- [ ] Check for any gRPC connection issues
- [ ] Verify Pub/Sub messages are flowing
- [ ] Monitor CPU and memory usage
- [ ] Check for any authorization failures

### Short-term (First Week)
- [ ] Review security logs for any anomalies
- [ ] Verify all integration points working
- [ ] Check performance metrics
- [ ] Get team feedback on stability

### Ongoing
- [ ] Monthly security update review
- [ ] Quarterly dependency updates
- [ ] Semi-annual penetration testing
- [ ] Annual security audit

---

## Rollback Plan

If any critical issues emerge post-deployment:

### Immediate Rollback
```bash
# Revert dependency changes
git revert <commit-hash>

# Verify old dependencies
go mod tidy
govulncheck ./...

# Redeploy
# Follow normal deployment procedure
```

### Rollback Triggers
- [ ] Deployment fails build/tests
- [ ] Pub/Sub connections failing
- [ ] gRPC authorization failures
- [ ] Significant performance degradation (>20% increase in latency)
- [ ] Memory/CPU usage spike (>30% increase)
- [ ] Error rate spike (>100% increase)

### Communication Plan
1. Alert on-call engineer immediately
2. Notify team leads
3. Notify stakeholders
4. Post incident review within 24 hours
5. Document lessons learned

---

## Security Maintenance Schedule

### Weekly
- [ ] Check GitHub for critical security alerts

### Monthly
- [ ] Review new CVEs in Go ecosystem
- [ ] Run `go get -u` to check for updates
- [ ] Review dependabot alerts if enabled

### Quarterly
- [ ] Full dependency audit
- [ ] Update all non-breaking dependencies
- [ ] Security training/review with team

### Annually
- [ ] External security audit
- [ ] Penetration testing
- [ ] Supply chain security review

---

## Team Communication Template

### Notify Team (Pre-Deployment)
```
Subject: Upcoming Dependency Upgrade - Security Review Complete

Team,

We're upgrading the following dependencies in the ingestion-gateway service:

1. cloud.google.com/go/pubsub: v1.50.2 → v2.6.0
2. google.golang.org/api: v0.274.0 → v0.277.0
3. google.golang.org/grpc: v1.80.0 (new direct dependency)
4. go.opentelemetry.io/.../otelgrpc: v0.67.0 → v0.68.0

Security Status: ✅ All packages verified safe
Notable: gRPC v1.80.0 includes authorization bypass security fix

Deployment scheduled for: [DATE]
Expected downtime: [DURATION, typically 0 for rolling deployment]

Questions? See attached security analysis documents.
```

### Notify Team (Post-Deployment)
```
Subject: Dependency Upgrade Complete - All Systems Operational

Team,

The ingestion-gateway dependency upgrades have been successfully deployed.

Status: ✅ Production deployment complete
Monitoring: Continuous for next 48 hours
Performance: [Include key metrics comparison]
Issues: [None found / Describe any issues if present]

No action required from your team. Continue monitoring your integration points.
```

---

## Long-term Security Strategy

### 1. Automated Vulnerability Scanning
Add to CI/CD:
```yaml
# .github/workflows/security.yml (GitHub Actions)
name: Security Scan

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
  schedule:
    - cron: '0 0 * * 0'  # Weekly on Sunday

jobs:
  govulncheck:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.25.0'
      - name: Run govulncheck
        run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./...
```

### 2. Dependabot Integration
```yaml
# .github/dependabot.yml
version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"
    schedule:
      interval: "weekly"
    security-updates-only: false
    open-pull-requests-limit: 10
    reviewers:
      - "security-team"
    allow:
      - dependency-type: "all"
```

### 3. Security Advisories Subscription
- [ ] Subscribe to Google Cloud security bulletins
- [ ] Watch gRPC-Go security releases
- [ ] Follow OpenTelemetry security advisories
- [ ] Set up GitHub security alerts

### 4. Internal Documentation
- [ ] Document your dependency upgrade process
- [ ] Create runbook for security incidents
- [ ] Maintain inventory of critical dependencies
- [ ] Document SLAs for security patches

---

## FAQ

### Q: Should we worry about the Pub/Sub v1→v2 change?
**A:** No, this is a planned migration. It's better to upgrade proactively than to wait until v1 is deprecated. The v1.50.2 version we were on had no known vulnerabilities, but v2 is the recommended path going forward.

### Q: Is the gRPC authorization fix critical?
**A:** It's important but likely low risk for your use case unless:
- You use grpc/authz interceptors with path-based deny rules
- You have untrusted clients sending crafted headers
If neither applies, it's still recommended as security hardening.

### Q: Could this break anything?
**A:** Unlikely. All upgrades maintain backward compatibility except the intentional v1→v2 Pub/Sub change, which has a documented migration guide.

### Q: How often should we update dependencies?
**A:** 
- Security updates: Immediately (within days)
- Minor updates: Quarterly
- Major updates: Evaluate case-by-case

### Q: What's the performance impact?
**A:** Likely positive. gRPC v1.80.0 includes memory optimizations through pooled write buffers.

---

## Success Criteria

Deployment is successful when:

- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] govulncheck shows zero vulnerabilities
- [ ] No error rate increase in production
- [ ] No latency increase > 5%
- [ ] No memory/CPU spike > 10%
- [ ] All Pub/Sub operations working normally
- [ ] Tracing/observability working
- [ ] Team reports no issues after 48 hours

---

## Escalation Matrix

### Level 1: Minor Issues (Handle immediately)
- Single transaction failures
- Occasional connection timeouts
- Non-critical logging errors

### Level 2: Significant Issues (Escalate within 1 hour)
- Pub/Sub subscription failures
- Authorization failures
- gRPC connection pool exhaustion

### Level 3: Critical Issues (Escalate immediately - consider rollback)
- Complete Pub/Sub failure
- Widespread authorization bypass
- Complete service unavailability

---

## Approval Sign-off

Security Analysis completed: ✅ May 3, 2026
Vulnerability scan status: ✅ PASSED (0 CVEs)
Recommendation: ✅ APPROVED FOR PRODUCTION

---

## Additional Resources

- [Go Vulnerability Database](https://pkg.go.dev/vuln/)
- [NIST NVD](https://nvd.nist.gov/)
- [GitHub Security Advisories](https://github.com/advisories)
- [gRPC Security Documentation](https://grpc.io/docs/guides/security/)
- [Google Cloud Security](https://cloud.google.com/security)
- [OpenTelemetry Security](https://opentelemetry.io/docs/community/security/)

---

**Document Version:** 1.0  
**Last Updated:** May 3, 2026  
**Created by:** Security Vulnerability Analysis  
**Status:** Final - Ready for Implementation
