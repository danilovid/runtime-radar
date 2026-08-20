# MCP Server

MCP Server exposes Runtime Radar's data to AI agents over the
[Model Context Protocol](https://modelcontextprotocol.io), **read-only**. It is a thin wrapper over the product's own
gRPC APIs: it never writes anything, never blocks a process and never kills a pod. Whatever a model concludes from it
is advisory and has to be confirmed by a human.

It is built on the official [Go SDK](https://github.com/modelcontextprotocol/go-sdk) and speaks two transports:
Streamable HTTP (the one to deploy) and stdio (for local debugging).

## Tools

| Tool | What it does | Permission required |
| --- | --- | --- |
| `search_runtime_events` | Finds runtime events by namespace, pod, binary, detector and time window. Returns compact summaries, not raw Tetragon payloads. | `events: read` |
| `get_runtime_event` | Reads one event in full: process and parent, probe arguments, stack traces, threats and detect errors. | `events: read` |
| `get_process_context` | Returns what else the same process, its parent and its children did around an event. | `events: read` |
| `list_detectors` | Lists the detectors installed in Event Processor and what they look for. | `system_settings: read` |
| `get_runtime_stats` | Counts events in a time window, with a breakdown by event type. | `events: read` |
| `search_docs` | Full-text search over the product documentation shipped inside the image. | a valid token |

Every response is bounded on purpose:

- `search_runtime_events` returns at most 50 events and cuts process arguments at 512 bytes;
- `get_runtime_event` cuts arguments at 4 KB, probe arguments at 16 × 512 bytes and stack traces at 20 frames;
- `get_process_context` returns at most 20 events;
- `search_docs` returns at most 10 fragments of 2 KB each.

Credential-shaped text (bearer tokens, `--password` values, PEM blocks, long base64 blobs) is masked in everything
that leaves the service, the same way `notifier/pkg/ai` masks it before sending an event to an LLM provider.

`list_detectors` reports no severity: severity is not a property of a detector, it is assigned per detection and comes
back with every threat in `search_runtime_events` and `get_runtime_event`. MITRE ATT&CK identifiers are listed only
when a detector's author put them in its description, since the product carries no ATT&CK mapping of its own.

## Untrusted data

Everything the tools return is telemetry produced by workloads, and an attacker controls much of it: process
arguments, file paths, pod and image names. The description of every tool, and the server instructions handed to the
client on initialize, say so explicitly, so that a model treats the result as data to analyse and never as
instructions addressed to itself.

## Configuration

Configuration comes from environment variables and flags, as in every other service (see `pkg/config`).

| Variable | Flag | Default | Meaning |
| --- | --- | --- | --- |
| `LISTEN_HTTP_ADDR` | `-listenHTTPAddr` | `:9000` | Address the Streamable HTTP transport listens on. |
| `LISTEN_INSTRUMENTATION_ADDR` | `-listenInstrumentationAddr` | `:9090` | Probes (`/live`, `/ready`) and Prometheus metrics (`/metrics`). |
| `LISTEN_GOPS_ADDR` | `-listenGopsAddr` | `127.0.0.1:7000` | gops agent. |
| `STDIO` | `-stdio` | `false` | Serve over stdin/stdout instead of HTTP. Local debugging only. |
| `HISTORY_API_GRPC_ADDR` | `-historyAPIGRPCAddr` | `127.0.0.1:8000` | History API gRPC address. |
| `EVENT_PROCESSOR_GRPC_ADDR` | `-eventProcessorGRPCAddr` | `127.0.0.1:8000` | Event Processor gRPC address. |
| `AUTH` | `-auth` | `false` | Verify JWT tokens of MCP clients. |
| `TOKEN_KEY` | `-tokenKey` | | Hex-encoded key the tokens are signed with, the same one the other services use. |
| `AUTH_TOKEN` | `-authToken` | | Token for outgoing calls when the client presents none of its own: every call in stdio mode, and HTTP calls when auth is disabled. |
| `TLS` | `-tls` | `false` | Serve HTTPS and dial the gRPC services over TLS, using `ca.pem`, `cert.pem` and `key.pem`. |
| `DOCS_DIR` | `-docsDir` | `docs` (`/docs` in the image) | Directory `search_docs` indexes `*.md` from. |
| `LOG_LEVEL` | `-logLevel` | `TRACE` | Log level. In stdio mode logs go to stderr, since stdout carries the protocol. |

### Auth

With `AUTH=true` every request must carry `Authorization: Bearer <token>` — a Runtime Radar access token, the same one
the UI uses. The server verifies it, checks the permission the tool needs, and **forwards that very token** to History
API and Event Processor, so RBAC and audit see the person behind the agent rather than this service. A request without
a usable token is rejected with `401` before it reaches the protocol layer.

Get a token with:

```bash
curl -s -X POST https://runtime-radar.example/api/v1/signin -H 'Content-Type: application/json' -d '{"username":"analyst","password":"..."}'
```

The `access_token` field of the response is what goes into the client configuration below.

With `AUTH=false` there is no authentication at all and every client gets full read access to runtime events. Only do
that on a local deployment.

## Connecting a client

The server is routed at `/mcp` by the reverse proxy, so the endpoint of a normal deployment is
`https://<runtime-radar-host>/mcp`.

### Claude Code

```bash
claude mcp add --transport http runtime-radar https://runtime-radar.example/mcp --header "Authorization: Bearer <access_token>"
```

Or, in `.mcp.json` of a project:

```json
{
  "mcpServers": {
    "runtime-radar": {
      "type": "http",
      "url": "https://runtime-radar.example/mcp",
      "headers": {
        "Authorization": "Bearer <access_token>"
      }
    }
  }
}
```

### Claude Desktop

In `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "runtime-radar": {
      "type": "http",
      "url": "https://runtime-radar.example/mcp",
      "headers": {
        "Authorization": "Bearer <access_token>"
      }
    }
  }
}
```

### stdio, for local debugging

```bash
task run -- -historyAPIGRPCAddr localhost:8003 -eventProcessorGRPCAddr localhost:8002
```

The same binary as an MCP server of a local client:

```json
{
  "mcpServers": {
    "runtime-radar": {
      "command": "/path/to/runtime-radar/mcp-server/cmd/mcp-server/mcp-server",
      "args": ["-stdio", "-docsDir", "/path/to/runtime-radar/docs",
               "-historyAPIGRPCAddr", "localhost:8003",
               "-eventProcessorGRPCAddr", "localhost:8002"]
    }
  }
}
```

## Observability

Every tool call is logged with zerolog (caller, tool, arguments with values longer than 128 bytes cut, duration) and
counted in Prometheus:

- `mcp_server_tool_calls_total{tool}`
- `mcp_server_tool_errors_total{tool,code}`
- `mcp_server_tool_duration_seconds{tool}`

Tokens are never logged, and the shared HTTP logging middleware — which dumps request headers at debug level — is
deliberately not used on the MCP endpoint.

## Development

```bash
task build     # build the binary
task test      # unit tests
task lint      # golangci-lint
task run       # run over stdio against a local deployment
```

The image is built with the **repository root** as the Docker build context, because it ships `docs/**/*.md` for
`search_docs`; `task docker-build` and the `docker-compose.yaml` service already do that. A plain `docker build .`
inside this directory will not work.
