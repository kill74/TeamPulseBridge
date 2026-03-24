# Architecture Overview

```mermaid
flowchart LR
  A[External Platforms] --> B[Ingestion Gateway]
  B --> C[Queue Backend]
  C --> D[Normalizer and Workers]
  B --> E[Metrics and Traces]
  E --> F[Prometheus]
  E --> G[Trace Backend]
  F --> H[Grafana]
```

## Design Principles

- Stateless horizontal scaling
- Signature-verified ingestion
- Async processing and backpressure handling
- Strong observability as default
- Clear operational contracts and runbooks
