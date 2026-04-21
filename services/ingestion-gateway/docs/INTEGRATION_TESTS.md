# Integration Tests Against Google Pub/Sub Emulator

Comprehensive integration tests for the queue publisher against the Google Cloud Pub/Sub emulator. These tests verify end-to-end webhook ingestion, message publishing, and observability.

## Overview

The integration test suite provides:

- **Unit + Integration Blend**: Tests at queue, handler, and full webhook flow levels
- **Pub/Sub Emulator**: Local Pub/Sub environment without credentials
- **Concurrent Testing**: Horizontal scalability and race condition detection
- **Docker Support**: Reproducible testing environment
- **CI/CD Ready**: Integrated into GitHub Actions workflow
- **Benchmarks**: Performance regression detection

## Architecture

```
Webhook (HTTP) → Handler → Signature Validation → Queue → Pub/Sub Emulator → Subscriber
                                                      ↓
                                          Observability (metrics, traces, logs)
```

## Test Files

### `internal/testhelpers/pubsubtest/emulator.go`

Helper utilities for Pub/Sub emulator management:

- `NewPubSubClient()` - Connect to emulator
- `CreateTopic()` / `CreateSubscription()` - Resource creation
- `ReceiveMessages()` - Message consumption with timeout
- `PurgeSubscription()` - Cleanup between tests
- `MessageCollector` - Background message collection

### `internal/queue/queue_integration_test.go`

Queue layer integration tests:

- `TestPubSubPublisherIntegration()` - Single and concurrent publishes
- `TestAsyncPublisherWithPubSub()` - Async wrapper behavior
- `TestPubSubPublisherWithoutTopic()` - Error handling for non-existent topics

### `internal/handlers/webhooks_pubsub_integration_test.go`

End-to-end webhook tests:

- `TestWebhookToPubSubIntegration()` - Slack, GitHub webhook flows
- `TestWebhookWithMiddleware()` - HTTP middleware integration
- `BenchmarkWebhookToPubSub()` - Performance baseline

## Prerequisites

### Local Development

```bash
# Install Docker
docker --version    # Docker 20.10+

# Install Go
go version         # Go 1.22+

# Install testify for assertions (automatic via go.mod)
```

### Environment Setup

The Pub/Sub emulator requires `PUBSUB_EMULATOR_HOST` environment variable:

```bash
export PUBSUB_EMULATOR_HOST=localhost:8085
export PUBSUB_PROJECT_ID=test-project
```

## Running Tests

### Quick Start (Local)

```bash
# Start Pub/Sub emulator in Docker
docker run -d \
  --name pubsub-emulator \
  -p 8085:8085 \
  google/cloud-sdk:emulators \
  gcloud beta emulators pubsub start --host-port=0.0.0.0:8085

# Wait for readiness
sleep 5

# Set environment
export PUBSUB_EMULATOR_HOST=localhost:8085

# Run integration tests from the project root
cd services/ingestion-gateway
go test -v -race ./internal/queue ./internal/handlers

# Stop emulator
docker stop pubsub-emulator
```

### Using Makefile

```bash
# Run contract and schema drift checks
make contract-test

# Run all integration tests
make integration-test

# Run only queue integration tests
make integration-test-queue

# Run only webhook integration tests
make integration-test-handlers

# Run benchmarks
make integration-bench

# Run with Docker Compose
make integration-docker-compose
```

### Using Docker Compose

```bash
# Start all services (emulator + service under test)
cd services/ingestion-gateway
docker-compose -f docker-compose.integration.yml up -d

# Service logs
docker-compose -f docker-compose.integration.yml logs -f ingestion-test

# Stop services
docker-compose -f docker-compose.integration.yml down
```

### Verbose Output

```bash
cd services/ingestion-gateway

# Very verbose with caller info
go test -v -race \
  -run TestPubSubPublisherIntegration \
  -count=1 \
  ./internal/queue

# With output buffering disabled
go test -v -race -p=1 ./internal/queue ./internal/handlers
```

## Test Cases

### Queue Layer Tests

#### `PublishMessage`

- Single message publish
- Attribute verification (source, schema, schema_version)
- Message envelope structure validation
- Header preservation

**Assertions:**

- HTTP 202 response (or async result)
- Message attributes present
- Message body contains valid JSON envelope
- Original headers preserved in envelope

#### `PublishMultipleMessages`

- Sequential publishes from different sources
- Concurrent publishes (go routines)
- Message ordering verification

**Assertions:**

- All messages received in subscription
- Sources match (Slack, GitHub, GitLab)
- No messages lost
- Order preserved for same source

#### `PublishWithLargePayload`

- ~100 event messages in single webhook
- Size limits handling
- Message fragmentation (if applicable)

**Assertions:**

- Large payloads transmitted intact
- No truncation
- Memory efficiency

#### `AsyncPublisherQueueFull`

- Fill buffer with max capacity
- Overflow attempt
- Error signaling

**Assertions:**

- `ErrQueueFull` returned when buffer exceeds capacity
- Graceful error handling
- No data loss for queued messages

### Handler Layer Tests

#### `SlackWebhookToPubSub`

- Slack signature validation (HMAC-SHA256)
- Challenge token handling
- URL verification request

**Assertions:**

- HTTP 200 for URL verification
- Message attributes: `source=slack`, `schema=raw-webhook-envelope`
- Slack payload preserved in envelope
- Headers included

#### `GitHubWebhookToPubSub`

- GitHub signature validation (SHA256)
- Pull request payload
- Different webhook actions

**Assertions:**

- HTTP 202 for valid webhooks
- Message attributes correct
- Original payload preserved
- Multiple actions handled

#### `WebhookSignatureValidation`

- Invalid signature rejection
- Tampered payload detection
- Missing signature handling

**Assertions:**

- HTTP 403 for invalid signatures
- No message published
- Error logged

#### `MultipleWebhooksOrdered`

- Send 5 sequential webhooks
- Verify order preservation
- Check for lost messages

**Assertions:**

- All 5 messages received
- Message numbering sequential
- No gaps

#### `WebhookWithAsyncPublisher`

- Async wrapper with actual publishing
- Buffer utilization
- Latency impact

**Assertions:**

- All messages eventually published
- Acceptable latency (<1s)
- No buffer overflow

### Middleware Tests

#### `WebhookWithRequestID`

- Request ID generation
- Panic recovery
- Middleware ordering

**Assertions:**

- Request ID propagated (if captured)
- No panics swallowed silently
- Proper error handling

## Performance Benchmarks

Run during CI/CD to detect regressions:

```bash
cd services/ingestion-gateway
go test -bench=BenchmarkWebhookToPubSub -benchmem -benchtime=5s ./internal/handlers
```

Expected benchmarks (baseline):

- `BenchmarkWebhookToPubSub-8`: ~500-1000 ns/op, ~1-2 KB alloc/op

## Troubleshooting

### Emulator Won't Start

```bash
# Check Docker status
docker ps -a | grep pubsub

# View emulator logs
docker logs pubsub-emulator

# Force remove and restart
docker rm -f pubsub-emulator
docker run ... (restart command)

# Verify port availability
lsof -i :8085  # macOS/Linux
netstat -ano | findstr :8085  # Windows
```

### Tests Timeout

```bash
# Increase timeout
go test -timeout 60s ./...

# Run single test with verbose output
go test -v -timeout 10s -run TestPubSubPublisherIntegration/PublishMessage ./internal/queue

# Check emulator connectivity
curl -v http://localhost:8085/v1/projects/test-project/topics
```

### Message Not Received

```bash
# Verify subscription exists
docker exec pubsub-emulator gcloud pubsub subscriptions list --project=test-project

# Manually publish test message
docker exec pubsub-emulator gcloud pubsub topics publish test-webhook-topic \
  --message='{"test":"data"}' \
  --project=test-project

# Consume from subscription
docker exec pubsub-emulator gcloud pubsub subscriptions pull test-sub \
  --auto-ack \
  --limit=10 \
  --project=test-project
```

### PUBSUB_EMULATOR_HOST Not Set

```bash
# For bash/zsh
export PUBSUB_EMULATOR_HOST=localhost:8085

# For PowerShell
$env:PUBSUB_EMULATOR_HOST='localhost:8085'

# For Windows CMD
set PUBSUB_EMULATOR_HOST=localhost:8085

# Verify
echo $PUBSUB_EMULATOR_HOST  # bash
echo $env:PUBSUB_EMULATOR_HOST  # PowerShell
```

## Test Patterns

### Creating a Local Test

```go
package queue_test

import (
    "context"
    "testing"
    "time"

    "cloud.google.com/go/pubsub"
    "github.com/stretchr/testify/require"
    "github.com/guilhermesales/TeamPulseBridge/services/ingestion-gateway/internal/testhelpers/pubsubtest"
)

func TestMyFeature(t *testing.T) {
    // Setup
    cfg := pubsubtest.DefaultEmulatorConfig()
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    client, err := pubsubtest.NewPubSubClient(ctx, cfg)
    require.NoError(t, err)
    defer client.Close()

    topic, err := pubsubtest.CreateTopic(ctx, client, "my-topic")
    require.NoError(t, err)

    sub, err := pubsubtest.CreateSubscription(ctx, client, "my-sub", "my-topic")
    require.NoError(t, err)

    // Test logic
    // ...

    // Verify
    messages, err := pubsubtest.ReceiveMessages(ctx, sub, 1, 3*time.Second)
    require.NoError(t, err)
    require.Len(t, messages, 1)
}
```

### Testing Error Paths

```go
func TestPublishError(t *testing.T) {
    // Use non-existent topic to trigger error
    topic := client.Topic("does-not-exist")
    publisher := queue.NewPubSubPublisherDirect(client, topic, logger)

    err := publisher.Publish(ctx, "github", []byte(`{}`), map[string]string{})
    assert.Error(t, err)
}
```

### Concurrent Test Pattern

```go
func TestConcurrentPublish(t *testing.T) {
    done := make(chan error, 10)

    for i := 0; i < 10; i++ {
        go func(id int) {
            err := publisher.Publish(ctx, "github", data, headers)
            done <- err
        }(i)
    }

    for i := 0; i < 10; i++ {
        err := <-done
        require.NoError(t, err)
    }
}
```

## CI/CD Integration

### GitHub Actions Workflow

The `.github/workflows/integration.yml` workflow runs:

1. **Integration Tests** - Full suite against emulator
2. **Docker Compose Test** - Service startup and health checks
3. **Benchmarks** - Performance regression detection

**Triggers:**

- On push to `main` branch
- On pull requests to `main`
- Changes to service code or workflow file

**Concurrency:**

- Cancels previous runs on new push
- Parallel job execution

**Artifacts:**

- Code coverage report (uploaded to Codecov)
- Test results
- Benchmark baselines

### Local Pre-Commit

```bash
#!/bin/bash
# .git/hooks/pre-commit

cd services/ingestion-gateway
go test -race -count=1 ./internal/queue ./internal/handlers || exit 1
```

## Contract and Data Quality

The ingress contract suite is intentionally separate from Pub/Sub emulator integration coverage.

Use it when you change webhook fixtures, queue schema shape, or provider-specific parsing assumptions:

```bash
make contract-test
```

That target verifies:

- the versioned fixture catalog is complete and internally consistent
- publishable fixtures still fit the versioned raw webhook envelope schema
- negative fixtures capture common degraded-but-authenticated provider payloads
- the compatibility matrix stays in sync with the executable catalog

## Best Practices

✅ **Do:**

- Clean up resources in `defer` and `t.Cleanup()`
- Use `testing.Short()` to skip in `-short` mode
- Set reasonable timeouts (contexts)
- Use helper functions to avoid duplication
- Test both success and error paths
- Run with `-race` to catch data races

❌ **Don't:**

- Share state between tests
- Hardcode timestamps (use fixed `time.Time` for reproducibility)
- Assume port availability
- Leave Docker containers running
- Write to disk in tests
- Ignore cleanup errors

## Future Enhancements

- [ ] Add stress tests (high throughput)
- [ ] Add chaos tests (emulator failures)
- [ ] Add multi-region emulator testing
- [ ] Add dead-letter queue testing
- [ ] Generate test coverage reports with HTML output
- [ ] Add property-based testing with quicktest

## Resources

- [Google Cloud Pub/Sub Emulator Docs](https://cloud.google.com/pubsub/docs/emulator)
- [Go Testing Package](https://golang.org/pkg/testing/)
- [Testify Documentation](https://github.com/stretchr/testify)
- [Go Concurrency Patterns](https://go.dev/blog/pipelines)
