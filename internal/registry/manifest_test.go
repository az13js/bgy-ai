package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestBasic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.yaml")
	data := `
name: test-svc
description: A test service
type: http
base_url: https://api.example.com
headers:
  accept: application/json
tools:
  - name: search
    description: Search stuff
    method: GET
    path: /v1/search
    parameters:
      q:
        type: string
        default: ""
    response_path: data.items
  - name: create
    description: Create item
    method: POST
    path: /v1/items
    body_template: '{"name":"{{.name}}"}'
    parameters:
      name:
        type: string
`
	if err := os.WriteFile(p, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadManifest(p)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}

	if cfg.Name != "test-svc" {
		t.Errorf("name: got %q", cfg.Name)
	}
	if cfg.Type != "http" {
		t.Errorf("type: got %q", cfg.Type)
	}
	if cfg.BaseURL != "https://api.example.com" {
		t.Errorf("base_url: got %q", cfg.BaseURL)
	}
	if cfg.Headers["accept"] != "application/json" {
		t.Errorf("headers: got %v", cfg.Headers)
	}

	if len(cfg.Tools) != 2 {
		t.Fatalf("tools: got %d", len(cfg.Tools))
	}

	t1 := cfg.Tools[0]
	if t1.Name != "search" || t1.Method != "GET" {
		t.Errorf("tool 0: %+v", t1)
	}
	if t1.Params["q"].Default != "" {
		t.Errorf("search q default: got %v", t1.Params["q"].Default)
	}
	if t1.ResponsePath != "data.items" {
		t.Errorf("response_path: got %q", t1.ResponsePath)
	}

	t2 := cfg.Tools[1]
	if t2.Name != "create" || t2.Method != "POST" {
		t.Errorf("tool 1: %+v", t2)
	}
	if t2.BodyTemplate != `{"name":"{{.name}}"}` {
		t.Errorf("body_template: got %q", t2.BodyTemplate)
	}
}

func TestLoadManifestDefaults(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "defaults.yaml")
	data := `
name: def
description: defaults test
`
	if err := os.WriteFile(p, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadManifest(p)
	if err != nil {
		t.Fatal(err)
	}
	// type defaults to mcp
	if cfg.Type != "mcp" {
		t.Errorf("default type: got %q, want mcp", cfg.Type)
	}
	// method defaults to GET
	if len(cfg.Tools) != 0 {
		t.Error("expected empty tools")
	}
}

func TestLoadManifestWithCommand(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cmd.yaml")
	data := `
name: cmd-svc
description: command test
type: http
base_url: https://api.example.com
header_command: "python3 sign.py"
shell: "bash -c"
tools:
  - name: custom
    description: custom tool
    command: "python3 builder.py"
    parameters:
      keyword:
        type: string
    response_path: data
`
	if err := os.WriteFile(p, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadManifest(p)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if cfg.HeaderCommand != "python3 sign.py" {
		t.Errorf("header_command: got %q", cfg.HeaderCommand)
	}
	if cfg.Shell != "bash -c" {
		t.Errorf("shell: got %q", cfg.Shell)
	}
	if len(cfg.Tools) != 1 {
		t.Fatalf("tools: got %d", len(cfg.Tools))
	}
	if cfg.Tools[0].Command != "python3 builder.py" {
		t.Errorf("tool command: got %q", cfg.Tools[0].Command)
	}
	if cfg.Tools[0].ResponsePath != "data" {
		t.Errorf("response_path: got %q", cfg.Tools[0].ResponsePath)
	}
}

func TestLoadManifestNonExistent(t *testing.T) {
	_, err := LoadManifest("/nonexistent/path.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadManifestInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "invalid.yaml")
	if err := os.WriteFile(p, []byte("bad: [[["), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadManifest(p)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}
