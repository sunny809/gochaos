package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStubsFromFileJSON(t *testing.T) {
	// Create a temp JSON file with a single stub
	dir := t.TempDir()
	path := filepath.Join(dir, "stub.json")
	if err := os.WriteFile(path, []byte(`{
		"request": {"method": "GET", "urlPath": "/api/test"},
		"response": {"status": 200, "body": "ok"}
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	stubs, err := LoadStubsFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stubs) != 1 {
		t.Fatalf("expected 1 stub, got %d", len(stubs))
	}
	if stubs[0].Request.Method != "GET" {
		t.Errorf("expected GET, got %s", stubs[0].Request.Method)
	}
	if stubs[0].Request.URLPath != "/api/test" {
		t.Errorf("expected /api/test, got %s", stubs[0].Request.URLPath)
	}
	if stubs[0].Response.Status != 200 {
		t.Errorf("expected 200, got %d", stubs[0].Response.Status)
	}
}

func TestLoadStubsFromFileYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stub.yaml")
	if err := os.WriteFile(path, []byte(`
request:
  method: POST
  urlPath: /api/create
response:
  status: 201
  body: '{"id":1}'
`), 0644); err != nil {
		t.Fatal(err)
	}

	stubs, err := LoadStubsFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stubs) != 1 {
		t.Fatalf("expected 1 stub, got %d", len(stubs))
	}
	if stubs[0].Request.Method != "POST" {
		t.Errorf("expected POST, got %s", stubs[0].Request.Method)
	}
	if stubs[0].Response.Status != 201 {
		t.Errorf("expected 201, got %d", stubs[0].Response.Status)
	}
}

func TestLoadStubsFromFileMultiMapping(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mappings.json")
	if err := os.WriteFile(path, []byte(`{
		"mappings": [
			{
				"request": {"method": "GET", "urlPath": "/a"},
				"response": {"status": 200, "body": "a"}
			},
			{
				"request": {"method": "GET", "urlPath": "/b"},
				"response": {"status": 200, "body": "b"}
			}
		]
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	stubs, err := LoadStubsFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stubs) != 2 {
		t.Fatalf("expected 2 stubs, got %d", len(stubs))
	}
}

func TestLoadStubsFromFileMultiYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mappings.yaml")
	if err := os.WriteFile(path, []byte(`
mappings:
  - request:
      method: PUT
      urlPath: /a
    response:
      status: 200
  - request:
      method: DELETE
      urlPath: /b
    response:
      status: 204
`), 0644); err != nil {
		t.Fatal(err)
	}

	stubs, err := LoadStubsFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stubs) != 2 {
		t.Fatalf("expected 2 stubs, got %d", len(stubs))
	}
}

func TestLoadStubsFromFileJSONArray(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "array.json")
	if err := os.WriteFile(path, []byte(`[
		{"request": {"method": "GET", "urlPath": "/x"}, "response": {"status": 200}},
		{"request": {"method": "POST", "urlPath": "/y"}, "response": {"status": 201}}
	]`), 0644); err != nil {
		t.Fatal(err)
	}

	stubs, err := LoadStubsFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stubs) != 2 {
		t.Fatalf("expected 2 stubs, got %d", len(stubs))
	}
}

func TestLoadStubsFromFileNotFound(t *testing.T) {
	_, err := LoadStubsFromFile("/nonexistent/file.json")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestLoadStubsFromFilesMultiple(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.json")
	p2 := filepath.Join(dir, "b.json")
	os.WriteFile(p1, []byte(`{"request":{"method":"GET","urlPath":"/a"},"response":{"status":200}}`), 0644)
	os.WriteFile(p2, []byte(`{"request":{"method":"POST","urlPath":"/b"},"response":{"status":201}}`), 0644)

	stubs, err := LoadStubsFromFiles([]string{p1, p2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stubs) != 2 {
		t.Fatalf("expected 2 stubs, got %d", len(stubs))
	}
}

func TestLoadStubsFromFileYmlExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stubs.yml")
	os.WriteFile(path, []byte(`{"request":{"method":"GET","urlPath":"/test"},"response":{"status":200}}`), 0644)

	stubs, err := LoadStubsFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stubs) != 1 {
		t.Fatalf("expected 1 stub, got %d", len(stubs))
	}
}

func TestLoadStubsFromFileUnknownExt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stubs.txt")
	os.WriteFile(path, []byte(`{"request":{"method":"GET","urlPath":"/test"},"response":{"status":200}}`), 0644)

	// Should try JSON first, fall back to YAML
	stubs, err := LoadStubsFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stubs) != 1 {
		t.Fatalf("expected 1 stub, got %d", len(stubs))
	}
}

func TestLoadStubsFromFileEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	os.WriteFile(path, []byte(``), 0644)

	_, err := LoadStubsFromFile(path)
	if err == nil {
		t.Error("expected error for empty file")
	}
}
