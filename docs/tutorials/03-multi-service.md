# Tutorial 3: Multi-Service Chaos — Simulating Correlated Failures

## Goal

Simulate a real-world scenario where multiple downstream dependencies fail at the same
time (e.g., during a deploy-window or a regional outage) and verify your SUT's fallback
behavior.

## Prerequisites

- Go 1.22+ installed
- 15 minutes

## The Scenario

Your SUT depends on three services:

| Service | Normal behavior | Chaos to inject |
|---------|----------------|-----------------|
| **Payment** | 200 OK, `{"status":"ok"}` | 50% error rate for 10 seconds |
| **Inventory** | 200 OK, `{"stock":100}` | 2-second delay for 10 seconds |
| **Shipping** | 200 OK, `{"status":"shipped"}` | No chaos (control group) |

During the chaos window (0-10 seconds), Payment and Inventory should both fail
or slow down. After 10 seconds, all services recover.

## Step 1: Start gmock

```bash
docker run -p 8080:8080 ghcr.io/sunny809/gochaos:latest
```

## Step 2: Register stubs

### Payment stub (50% error rate)

```bash
curl -X POST http://localhost:8080/__admin/mappings \
  -H "Content-Type: application/json" \
  -d '{
    "name": "payment-stub",
    "request": {"method": "POST", "urlPath": "/api/payments"},
    "response": {
      "status": 200,
      "body": "{\"status\":\"ok\"}",
      "fault": {
        "type": "error",
        "activation": {
          "probability": 0.5,
          "activeBetween": [{"startMs": 0, "endMs": 10000}]
        }
      }
    }
  }'
```

### Inventory stub (2-second delay)

```bash
curl -X POST http://localhost:8080/__admin/mappings \
  -H "Content-Type: application/json" \
  -d '{
    "name": "inventory-stub",
    "request": {"method": "GET", "urlPath": "/api/inventory"},
    "response": {
      "status": 200,
      "body": "{\"stock\":100}",
      "delay": {
        "type": "fixed",
        "fixedMs": 2000
      },
      "fault": {
        "activation": {
          "activeBetween": [{"startMs": 0, "endMs": 10000}]
        }
      }
    }
  }'
```

### Shipping stub (control — no chaos)

```bash
curl -X POST http://localhost:8080/__admin/mappings \
  -H "Content-Type: application/json" \
  -d '{
    "name": "shipping-stub",
    "request": {"method": "GET", "urlPath": "/api/shipping"},
    "response": {
      "status": 200,
      "body": "{\"status\":\"shipped\"}"
    }
  }'
```

## Step 3: Verify using the Go library

See the [Go example](./examples/tutorial/03-multi-service/main_test.go) for a complete,
reproducible test.

## What to Verify

1. **Fault log**: Check that Payment had ~50% error rate during the chaos window
2. **Fault log**: Check that Inventory had delays injected
3. **Fault log**: Check that Shipping had NO faults injected (control group)
4. **SUT behavior**: Verify the SUT used circuit breakers, fallbacks, or retries