# YAML / JSON Stub Files

> gochaos can load stub definitions from YAML or JSON files. This is useful for
> sharing stub configurations across teams, managing stubs in version control,
> and setting up CI pipelines.

## Loading Stub Files

### CLI Mode

```bash
# Single file
gmock start --port 8080 --stubs ./stubs.yaml

# Multiple files
gmock start --port 8080 --stubs ./api-stubs.yaml --stubs ./fault-stubs.json
```

### Library Mode

```go
server := gmock.NewServer(
    gmocks.WithPort(0),
    gmocks.WithStubFiles("./stubs.yaml", "./fault-stubs.json"),
)
server.Start()
```

## YAML Format

```yaml
- name: list-users
  request:
    method: GET
    urlPath: /api/users
  response:
    status: 200
    headers:
      Content-Type: application/json
    body: '{"users":[{"id":1,"name":"Alice"}]}'

- name: get-user-by-id
  request:
    method: GET
    urlPathRegex: ^/api/users/\d+$
  response:
    status: 200
    headers:
      Content-Type: application/json
    body: '{"id":1,"name":"Alice"}'

- name: create-user
  request:
    method: POST
    urlPath: /api/users
    headers:
      Content-Type: application/json
  response:
    status: 201
    body: '{"id":2}'

- name: slow-endpoint
  request:
    method: GET
    urlPath: /api/slow
  response:
    status: 200
    delay:
      type: fixed
      value: 500

- name: error-fault
  request:
    method: GET
    urlPath: /api/unstable
  response:
    fault:
      type: error

- name: empty-fault
  request:
    method: GET
    urlPath: /api/empty
  response:
    fault:
      type: empty
```

## JSON Format

```json
[
  {
    "name": "get-users",
    "request": {
      "method": "GET",
      "urlPath": "/api/users"
    },
    "response": {
      "status": 200,
      "headers": {
        "Content-Type": "application/json"
      },
      "body": "{\"users\":[]}"
    }
  },
  {
    "name": "error-fault",
    "request": {
      "method": "GET",
      "urlPath": "/api/unstable"
    },
    "response": {
      "fault": {
        "type": "error"
      }
    }
  }
]
```

## Field Reference

See the [Admin API](../admin-api.md) for the complete field reference for
request patterns and response definitions.

## Example Files

See the test fixture files for more examples:

- [testdata/stubs.yaml](../testdata/stubs.yaml) — 8 stubs including fault injection
- [testdata/fault_stubs.json](../testdata/fault_stubs.json) — JSON format fault stubs

## Best Practices

1. **Name your stubs** — use the `name` field for readability
2. **One file per domain** — `api-stubs.yaml`, `fault-stubs.json`
3. **Version control your stubs** — keep them in the repo alongside tests
4. **Use YAML for readability** — YAML is easier to read and write
5. **Use JSON for programmatic generation** — JSON is easier to generate from scripts
