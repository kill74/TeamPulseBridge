# Testing Gap Analysis & Improvement Plan

This document outlines identified testing gaps in the `ingestion-gateway` service and provides a roadmap for increasing coverage in critical components.

## Current State
- **Critical components with zero or low coverage:**
  - `internal/retry`: Core retry logic is currently untested.
  - `internal/observability`: Telemetry and logging configurations lack unit tests.
  - `internal/schema`: Data validation logic is untested.

## Proposed Improvements

### 1. `internal/retry` (High Priority)
- **Goal:** Unit test `Scheduler` backoff and retry scheduling.
- **Tasks:**
  - Create `services/ingestion-gateway/internal/retry/scheduler_test.go`.
  - Mock `failstore.Store` and `queue.Publisher` interfaces.
  - Implement tests for:
    - Backoff calculation (check jitter distribution).
    - Max retry limits.
    - Graceful shutdown behavior.

### 2. `internal/observability` (Medium Priority)
- **Goal:** Verify telemetry exporter initialization and logging setup.
- **Tasks:**
  - Create `services/ingestion-gateway/internal/observability/telemetry_test.go`.
  - Mock the OTLP exporter and verify environment variable handling.

### 3. `internal/schema` (Medium Priority)
- **Goal:** Ensure schema validation correctly handles various input formats and errors.
- **Tasks:**
  - Create `services/ingestion-gateway/internal/schema/validator_test.go`.
  - Test valid/invalid JSON payloads against common schemas.

## Next Steps
- Begin implementation of unit tests for `internal/retry`.
