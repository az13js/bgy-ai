# AGENTS.md — bgy-ai

## Build & Run

```
go build -o bgy-ai ./cmd/bgyai/
go vet ./...
```

No test framework, linter, formatter, or CI are configured. `go test ./...` runs unit tests in `converter`, `provider`, and `registry` packages.

## Architecture

- **Module**: `bgy-ai` (go 1.24+)
- **Entrypoint**: `cmd/bgyai/main.go` → `internal/cli` (cobra auto-generation)
- **Only 2 dependencies**: `cobra` + `yaml.v3` (see `go.mod`)

| Directory | Role |
|---|---|
| `internal/cli/` | Root command, subcommand builder, executor |
| `internal/provider/` | `ToolProvider` interface + `MCPProvider` + `HTTPProvider` |
| `internal/registry/` | YAML loading, service registry (Load/Reload/Close) |
| `internal/converter/` | JSONSchema / HTTP params → cobra flag definitions |
| `internal/renderer/` | Text and JSON output formatting |

## Config

- YAML files read from `~/.bgy-ai/tools.d/*.yaml` (also `*.yml`)
- Custom directory: `--mcp-dir` flag
- MCP sessions cached to `~/.bgy-ai/sessions/<name>.json` (24h expiry)
- Session dir set in `cli/root.go:27` via `provider.SetSessionDir`

### Request Signing

HTTP services that require per-request signatures use `header_command`:

```yaml
header_command: "python3 /path/to/sign.py"
```

The command receives request context as stdin JSON (`{"method":"GET","path":"/api/skills?...","timestamp":"1782...","body":""}`) and must output header mapping as stdout JSON (`{"x-nonce":"...","x-signature":"...","x-timestamp":"..."}`). The `timestamp` in stdin matches the value used in URL query params — scripts must use it directly, not generate a new timestamp.

### External Tool Command

For tools that need full control over request building (URL, headers, body), use `command` at the tool level:

```yaml
tools:
  - name: my-tool
    description: Complex API call
    command: "python3 /path/to/build-request.py"
    parameters:
      keyword:
        type: string
        description: Search keyword
    response_path: data
```

**Stdin** (all CLI flags collected into params, plus context):
```json
{"method":"GET","path_template":"/api/skills","base_url":"https://...","params":{"keyword":"test","limit":20},"body_template":"...","timestamp_ms":"1782..."}
```

**Stdout** (the complete request specification):
```json
{"url":"https://.../api/skills?keyword=test&limit=20&timestamp=1782...","headers":{"x-nonce":"...","x-signature":"..."},"body":""}
```

bgy-ai sends the request with the returned `url`, merges `headers` with server-level headers, and uses `body` as-is. Response formatting still respects `response_path`. Timeout: 30s.

Tool-level `command` takes priority over all built-in path/body/param/sign logic.

### Shell

Commands (`header_command`, `command`) are executed through a shell. The default shell is auto-detected: `sh -c` on Linux/macOS, `cmd /c` on Windows. Override per service:

```yaml
shell: "bash -c"          # unix-specific
# shell: "powershell -Command"  # windows-specific
```

## Key Conventions

- **Tool name → CLI name**: Underscores replaced with hyphens (`search_wiki` → `search-wiki`)
- **CLI flag → param name**: Hyphens in flag names converted back to underscores (`--my-flag` → `my_flag`)
- **Provider type defaults**: empty/omitted → `mcp`; `streamable-http` also routes to MCP
- **HTTP method defaults**: empty/omitted → `GET`
- **Time templates**: `now`, `now-1h`, `now-6h`, `now-24h` are resolved to Unix millisecond timestamps automatically in HTTP provider
- **Required params**: Parameters without a `default` value in YAML are marked required on the CLI

## Prohibited

- Do **not** add `go.sum` changes to commits unless explicitly requested
- Do **not** add new dependencies without clear justification (the tool deliberately minimizes deps)
- Do **not** add test files or test frameworks unless asked
