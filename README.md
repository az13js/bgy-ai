# bgy-ai

A lightweight CLI gateway that turns MCP (Model Context Protocol) and REST/HTTP services into a discoverable command-line tool — auto-generating subcommands, flags, and output formatting from YAML configuration files.

> `bgy-ai` stands for **B**inary, **G**ood, and eas**Y** AI application interface. It reflects the tool's core philosophy: turning complex service integrations into a single, dependency-free binary that provides an effortless command-line interface.

## Features

- **Zero-code service integration** — add a YAML file, get a CLI subcommand
- **Dual protocol support** — MCP (JSON-RPC 2.0 + SSE) and plain HTTP/REST
- **Auto-generated CLI** — tools and their parameters automatically become `--flag-name` flags
- **Template variables** — path and POST body support `{{.param}}` substitution
- **Response extraction** — `response_path` dot-path to pull values from JSON responses
- **MCP session persistence** — sessions cached locally, re-initialized on expiry
- **Multiple output formats** — plain text (default) or structured JSON
- **External scripting** — shell commands can generate per-request headers or take over full HTTP request building
- **Minimal dependencies** — only Cobra and yaml.v3; zero runtime overhead

## Installation

```bash
# Build from source
git clone https://github.com/az13js/bgy-ai.git
cd bgy-ai
go build -o bgy-ai ./cmd/bgyai/
sudo mv bgy-ai /usr/local/bin/

# Or install to $GOPATH/bin
go install ./cmd/bgyai/
```

Requirements: **Go 1.24+**

## Quick Start

```bash
# 1. Create config directory
mkdir -p ~/.bgy-ai/tools.d

# 2. Add a service config
cat > ~/.bgy-ai/tools.d/my-api.yaml << 'EOF'
name: my-api
description: My REST API
type: http
base_url: https://httpbin.org
tools:
  - name: get-anything
    description: Send a GET request to /anything
    method: GET
    path: /anything
    parameters:
      foo:
        type: string
        description: A query parameter
        default: bar
    response_path: args
EOF

# 3. Verify it's loaded
bgy-ai list

# 4. Call the service
bgy-ai my-api get-anything --foo hello

# 5. JSON output mode
bgy-ai my-api get-anything --foo hello -o json
```

## Configuration

Services are defined as YAML files under `~/.bgy-ai/tools.d/` (or a custom directory via `--mcp-dir`).

### MCP Service

```yaml
name: my-mcp-service
description: An MCP tool server
type: mcp
url: https://my-server.example.com/mcp
headers:
  Authorization: Bearer <token>
```

MCP tools are **discovered automatically** via the `tools/list` RPC — no manual tool definitions needed.

### HTTP Service

```yaml
name: my-http-service
description: A REST API
type: http
base_url: https://api.example.com
headers:
  accept: application/json
tools:
  - name: search
    description: Search resources
    method: GET
    path: /v1/search
    parameters:
      keyword:
        type: string
        description: Search keyword
      page:
        type: number
        description: Page number
        default: 1
    response_path: data.items

  - name: create
    description: Create a resource
    method: POST
    path: /v1/resources
    body_template: |
      {"name": "{{.name}}", "tags": {{.tags}}}
    parameters:
      name:
        type: string
        description: Resource name
      tags:
        type: array
        description: Tag list
```

### Parameter Types

| Type | CLI Flag Type | Example |
|------|---------------|---------|
| `string` | `--flag string` | `--keyword hello` |
| `number` | `--flag int` | `--page 3` |
| `integer` | `--flag int` | `--count 10` |
| `boolean` | `--flag bool` | `--verbose` |
| `array` | `--flag stringSlice` | `--tags a --tags b` |

Parameters **without a `default` value** are automatically marked as required in the CLI.

### Path Template Variables

Use `{{.param_name}}` to substitute parameter values into URL paths:

```yaml
path: /api/projects/{{.project}}/repos/{{.repo}}/files
```

Parameters used in the path are **automatically excluded** from query strings.

### POST Body Templates

Use `{{.param_name}}` in JSON body templates:

```yaml
body_template: |
  {"query": "{{.keyword}}", "limit": {{.limit}}}
```

Values are JSON-encoded and the body is validated before sending.

### Response Path

Extract a sub-value from the JSON response using dot-path notation:

```
response_path: data.list
response_path: results.*.frames.0.data.values.2
```

- `key` — object property access
- `0`, `1`, `...` — array index
- `*` — wildcard (first key of an object)

### Time Defaults

The values `now`, `now-1h`, `now-6h`, and `now-24h` are automatically replaced with Unix millisecond timestamps at runtime.

### Per-Request Headers via External Command

For APIs that require dynamic per-request headers (signatures, tokens, nonces), use `header_command`:

```yaml
name: my-api
type: http
base_url: https://api.example.com
headers:
  accept: application/json
header_command: "python3 /path/to/sign.py"
tools:
  - name: search
    method: GET
    path: /v1/search
    parameters:
      keyword:
        type: string
```

The command receives request context as stdin JSON and must output header key-value pairs as stdout JSON:

```
stdin:  {"method":"GET","path":"/v1/search?keyword=test&timestamp=1782...","timestamp":"1782...","body":""}
stdout: {"x-nonce":"abc123","x-signature":"7f3a...","x-timestamp":"1782..."}
```

### Full Request Control via External Command

When a tool needs complete control over URL construction, headers, and body, use `command` at the tool level:

```yaml
tools:
  - name: complex-api
    description: Advanced API call
    command: "python3 /path/to/build-request.py"
    parameters:
      keyword:
        type: string
    response_path: data
```

The script receives all CLI parameters plus service context, and returns the complete request specification:

```
stdin:  {"method":"GET","path_template":"/v1/search","base_url":"https://...","params":{"keyword":"test"},"body_template":"...","timestamp_ms":"1782..."}
stdout: {"url":"https://.../v1/search?keyword=test","headers":{"x-nonce":"..."},"body":""}
```

Tool-level `command` takes priority over `method`/`path`/`body_template` — the script handles everything.

### Shell

Commands (`header_command`, `command`) run through a shell. Default is auto-detected: `sh -c` on Linux/macOS, `cmd /c` on Windows. Override per service:

```yaml
shell: "bash -c"
```

## CLI Reference

```
bgy-ai                              # Show available services
bgy-ai list                         # List all services and their tools
bgy-ai reload                       # Reload YAML configs without restarting
bgy-ai <server>                     # Show server help and available tools
bgy-ai <server> <tool> [flags]      # Execute a tool

bgy-ai <server> <tool> --help       # Show tool-specific flags

Flags:
  -o, --output string    Output format: text (default) or json
  --mcp-dir string       Config directory (default: ~/.bgy-ai/tools.d)
  --config string        Config root (default: ~/.bgy-ai)
  -v, --verbose          Enable debug logging
```

## Project Structure

```
bgy-ai/
├── cmd/bgyai/main.go              # Entrypoint
└── internal/
    ├── cli/                       # Cobra CLI: root, builder, executor
    │   ├── root.go
    │   ├── builder.go
    │   └── executor.go
    ├── converter/
    │   └── schema.go              # JSON Schema → CLI flags
    ├── provider/                  # Service provider implementations
    │   ├── interface.go           # ToolProvider interface
    │   ├── types.go               # ToolDef, CallResult, etc.
    │   ├── factory.go             # Provider dispatcher
    │   ├── http.go                # HTTP/REST provider
    │   └── mcp.go                 # MCP JSON-RPC provider
    ├── registry/                  # Config loading & service registry
    │   ├── loader.go              # Registry: Load/Reload/Get/Close
    │   └── manifest.go            # YAML manifest parsing
    └── renderer/
        └── renderer.go            # Text and JSON output formatting
```

## License

MIT — see [LICENSE](LICENSE) for details.
