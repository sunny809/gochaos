# Tutorial 1: 10-Minute Chaos Experiment

## Goal

Make your microservice survive intermittent failures from a downstream dependency.

## Prerequisites

- Go 1.22+ installed
- 10 minutes

## Step 1: Start gmock (Docker)

```bash
docker run -p 8080:8080 ghcr.io/sunny809/gochaos:latest
```

## Step 2: Register a stub with probabilistic fault

```bash
curl -X POST http://localhost:8080/__admin/mappings \
  -H "Content-Type: application/json" \
  -d '{
    "request": {"method": "GET", "urlPath": "/api/inventory"},
    "response": {
      "status": 200,
      "body": "{\"stock\":100}",
      "fault": {
        "type": "error",
        "activation": {"probability": 0.3}
      }
    }
  }'
```

## Step 3: Verify your SUT handles 30% errors

Run your test client against the SUT. The SUT should experience ~30% errors from the inventory endpoint.

## Step 4: Check the fault log

```bash
curl http://localhost:8080/__admin/fault-log
```

You should see entries showing how many faults were actually injected.

## What's next?

- Try different fault types: `empty`, `connection_reset`, `malformed`, `random_data`, `slow_close`
- Combine with delay distributions: `fixed`, `random`, `lognormal`, `dribble`
- See the Go example in `examples/tutorial/01-quick-start/` for the programmatic equivalent