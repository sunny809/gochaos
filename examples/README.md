# Examples

This directory contains runnable examples demonstrating gmock usage.

## Running Examples

Each example is a complete, runnable Go program. You can run them with:

```bash
# Run a specific example
cd examples/basic && go run .
cd examples/cors && go run .
cd examples/delay && go run .

# Or run the verification example as a test
cd examples/verification && go test -v
```

## Examples

- `basic/` — Starting a server and creating stubs
- `cors/` — CORS-enabled server with custom configuration
- `delay/` — Simulating response delays
- `verification/` — Using the verification API in tests

## Contributing

When adding a new example, ensure it compiles and runs. Each example should be a complete, self-contained program.
