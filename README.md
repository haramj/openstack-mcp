# OpenStack MCP

An MCP (Model Context Protocol) server for OpenStack built with the official Go SDK.

This project provides an MCP server that exposes OpenStack resources as MCP tools, allowing AI assistants and MCP-compatible clients to interact with OpenStack environments through a standardized interface.

## Features

Currently implemented tools:

- `list_instances`
  - List all instances in the current OpenStack project.

- `get_instance`
  - Get detailed information about an instance by name or ID.

- `list_networks`
  - List available OpenStack networks.

- `list_images`
  - List available OpenStack images.

- `list_flavors`
  - List available OpenStack flavors.

Administrator tools:

- `admin_instance_action`
  - Run lifecycle actions on an instance: `start`, `stop`, `reboot`,
    `pause`, `unpause`, `suspend`, `resume`, `shelve`, `unshelve`, `lock`,
    or `unlock`.
  - For reboot, pass `reboot_type` as `soft` or `hard`.

- `create_instance`
  - Create a new instance from an image, flavor, and network.
  - Optional fields: `key_name`, `security_groups`, and `wait` (default false).
  - By default, creation returns after the API accepts the request. Poll `get_instance` by returned ID for build status; opt in with `wait=true` to wait for the build. The deprecated `no_wait` field remains accepted, but `no_wait=false` no longer enables waiting. Do not set both `wait` and `no_wait` to true.

- `delete_instance`
  - Delete an instance by name.
  - Requires `confirm_name` to exactly match `name` before deletion.

Agent workflow tools:

- `get_agent_memory`
  - Read remembered OpenStack agent defaults and policy.

- `plan_instance_operation`
  - Plan the MCP tool and arguments for an instance operation before execution.
  - Applies remembered defaults for create requests.
  - Blocks risky operations against protected instance patterns.
  - This operation is read-only and does not call OpenStack.

- `summarize_agent_activity`
  - Summarize recent administrator MCP audit activity.
  - Groups successful, failed, and rejected work.
  - Highlights destructive operations and events that need attention.
  - This operation is read-only and does not call OpenStack.

- `record_agent_memory`
  - Update non-secret operational memory such as default image/flavor/network
    and protected instance patterns.
  - This modifies local MCP memory and should use write approval.

For Codex, keep read-only tools auto-approved and run administrator tools with
write approval enabled. A typical MCP policy is:

```toml
default_tools_approval_mode = "writes"

[mcp_servers.openstack.tools.list_instances]
approval_mode = "approve"
```

## Audit Logging

Administrator tools append JSONL audit events for every OpenStack state-changing
operation. The log records the operation, target instance, OpenStack command
arguments, success/failure status, and duration. Command output and OpenStack
credentials are not written to the audit log.

The default audit path is:

```text
~/.local/state/openstack-mcp/audit.jsonl
```

Override it with:

```bash
OPENSTACK_MCP_AUDIT_LOG=/path/to/audit.jsonl ./scripts/run-mcp-server.sh
```

Example event:

```json
{"timestamp":"2026-08-07T03:00:00Z","source":"openstack-mcp","operation":"delete_instance","target":"demo","destructive":true,"openstack_args":["server","delete","demo"],"status":"success","duration_ms":912}
```

If `delete_instance` is called without an exact `confirm_name` match, the server
records a `rejected` audit event and does not call OpenStack.

Use `summarize_agent_activity` to brief a human on recent MCP administrator
work:

```json
{
  "since_hours": 12,
  "limit": 20
}
```

This supports an operator workflow such as: "When I start work, summarize what
the OpenStack agent did overnight and highlight failed, rejected, or destructive
operations."

### OpenStack-wide Event Briefing

The audit log covers actions performed through this MCP server. It does not
automatically include every OpenStack event that happened outside MCP.

To brief all overnight OpenStack activity, the server would need an additional
event source such as Nova instance events, OpenStack service logs, Telemetry
services, audit middleware, or the OpenStack notification bus. The current MCP
design keeps that as a future read-only event ingestion layer, separate from the
administrator write tools.

## Memory and Planning

The server can remember non-secret operational defaults and safety rules. The
default memory path is:

```text
~/.config/openstack-mcp/memory.json
```

Create it from the example on the controller:

```bash
mkdir -p ~/.config/openstack-mcp
cp config/memory.example.json ~/.config/openstack-mcp/memory.json
chmod 600 ~/.config/openstack-mcp/memory.json
```

Override the path with:

```bash
OPENSTACK_MCP_MEMORY_FILE=/path/to/memory.json ./scripts/run-mcp-server.sh
```

The planner uses this memory before recommending write tools. For example, a
create request without image/flavor/network will be planned with remembered
defaults such as `RCP Ubuntu 22.04`, `m1.small`, and `demo-net`. A destructive
request against an instance matching a protected pattern such as `prod-*` is
blocked at the planning step.

Use `plan_instance_operation` before write tools in agent workflows:

```json
{
  "operation": "create",
  "name": "dev-box"
}
```

Example result:

```json
{
  "recommended_tool": "create_instance",
  "arguments": {
    "name": "dev-box",
    "image": "RCP Ubuntu 22.04",
    "flavor": "m1.small",
    "network": "demo-net"
  },
  "requires_approval": true,
  "blocked": false
}
```

## Evaluation

Planner behavior is covered by deterministic evaluation tests in
`internal/agent/planner_test.go`. These cases verify that remembered defaults are
applied, deletes require exact confirmation, protected instances are blocked
for risky lifecycle operations, and restorative lifecycle operations can still
be planned.

Run:

```bash
go test ./...
```

## Requirements

- Go 1.24+
- OpenStack CLI
- A configured OpenStack environment (`openrc` sourced)

## Installation

Clone the repository.

```bash
git clone https://github.com/haramj/openstack-mcp.git
cd openstack-mcp
```

Download dependencies.

```bash
go mod download
```

Build the server.

```bash
make build
```

## Usage

Before running the server, make sure your OpenStack credentials are loaded and
the `openstack` CLI is available.

```bash
source openrc
which openstack
openstack server list
```

Run the MCP server.

```bash
./openstack-mcp-server
```

### Controller Setup

When OpenStack is only available on an Ubuntu controller, run the MCP server on
that controller and connect to it from your laptop with an SSH tunnel.

Create a private OpenStack environment file on the controller.

```bash
mkdir -p ~/.config/openstack-mcp
chmod 700 ~/.config/openstack-mcp
cp config/openrc.example ~/.config/openstack-mcp/openrc
chmod 600 ~/.config/openstack-mcp/openrc
```

Edit `~/.config/openstack-mcp/openrc` with the controller's real OpenStack
values. Do not commit that file.

Create a local wrapper from the example.

```bash
cp scripts/run-mcp-server.sh.example scripts/run-mcp-server.sh
chmod +x scripts/run-mcp-server.sh
```

If your OpenStack virtualenv is not `/home/return/openstack-venv`, set
`OPENSTACK_VENV` when running the wrapper.

```bash
OPENSTACK_VENV=/path/to/openstack-venv ./scripts/run-mcp-server.sh
```

## Testing

You can test the server using the official MCP Inspector.

```bash
npx @modelcontextprotocol/inspector ./openstack-mcp-server
```

On the controller, prefer the wrapper so the Inspector-launched MCP server gets
the OpenStack CLI path and authentication environment.

```bash
make build
npx @modelcontextprotocol/inspector /home/return/src/openstack-mcp-server/scripts/run-mcp-server.sh
```

From your laptop, forward the Inspector ports over SSH.

```bash
ssh -L 6274:127.0.0.1:6274 -L 6277:127.0.0.1:6277 return@<controller-ip>
```

Open the tokenized Inspector URL from the controller terminal in your laptop
browser, replacing `localhost` with `127.0.0.1` if needed.

Use the Tools tab to test:

- `list_instances`
- `get_instance` with `{ "name": "test" }`
- `list_networks`
- `list_images`
- `list_flavors`
- `get_agent_memory`
- `plan_instance_operation` with `{ "operation": "create", "name": "demo" }`
- `summarize_agent_activity` with `{ "since_hours": 12, "limit": 20 }`
- `record_agent_memory` with `{ "default_network": "demo-net", "note": "demo-net is the default network for RCP test instances" }`
- `admin_instance_action` with `{ "name": "test", "action": "reboot", "reboot_type": "soft" }`
- `create_instance` with `{ "name": "demo", "image": "RCP Ubuntu 22.04", "flavor": "m1.small", "network": "demo-net" }`
- `delete_instance` with `{ "name": "demo", "confirm_name": "demo" }`

## Project Structure

```
.
├── cmd/
│   └── openstack-mcp-server/
│       └── main.go
├── internal/
│   ├── openstack/
│   │   ├── flavors.go
│   │   ├── images.go
│   │   ├── instances.go
│   │   └── networks.go
│   └── tools/
│       ├── flavors.go
│       ├── images.go
│       ├── instances.go
│       └── networks.go
├── config/
│   └── openrc.example
├── scripts/
│   ├── run-mcp-server.sh.example
│   ├── install.sh
│   ├── run-inspector.sh
│   └── test.sh
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Built With

- Go
- OpenStack CLI
- Model Context Protocol Go SDK

## License

MIT License

## Reliability and bounded local state

- `get_instance` accepts a name or ID and uses `server show`; ambiguous names are
  rejected by OpenStack rather than selecting the first listed server.
- CLI errors contain a category and exit code, never backend stdout/stderr or a
  reconstructed command. Audit arguments remain an array; names and arguments
  can still be sensitive and must not contain credentials. Summaries suppress
  historical error text written by older versions. Existing log files are not
  rewritten; operators should handle historical sensitive logs appropriately.
- Parent context cancellation/deadlines take precedence over the 20-second read,
  60-second action, and 5-minute creation ceilings. CLI output is capped at 16 MiB;
  stderr does not corrupt a successful JSON response.
- Memory retains the latest 100 notes, each at most 8192 bytes. Oversized new notes
  are rejected. The local file is capped at 2 MiB and replaced atomically with
  private permissions. Updates within one server process are serialized; use a
  separate memory path for each independently running server process.
- `protected_instance_patterns` **replaces** the complete list; omit the field to
  preserve it, or pass `[]` to clear it. This is planner policy, not an authorization
  boundary. A blocked plan always returns an empty `arguments` object.
- Audit summaries scan at most the newest 8 MiB and cache unchanged files. Totals
  reflect the inspected records only. `truncated`, `malformed_lines`,
  `scanned_bytes`, and `coverage_warnings` expose limits or damage. Individual
  records up to 1 MiB are supported. Rotation/truncation invalidates the cache;
  rotated archives are not included. Output lists are capped at 1000 entries and
  the requested window at one year. No chronological ordering is assumed.

### Development verification

```bash
go test -race -cover ./...
go vet ./...
go build ./cmd/openstack-mcp-server
```

Tests use temporary local files, a fake `openstack` executable, and in-memory MCP
sessions. They do not contact a real OpenStack cloud. Linux/macOS CI runs the same
checks. Live cloud acceptance remains a separate operator responsibility.

## Resources, prompts, and progress

Clients can read `openstack://memory`, `openstack://audit/summary`,
`openstack://flavors`, `openstack://images`, and `openstack://networks` alongside
all existing tools. User-selected prompts cover `provision-instance`,
`investigate-instance`, `overnight-summary`, and `safe-delete` workflows.

Modifying calls that supply a progress token receive elapsed-wait heartbeats;
these do not represent build completion percentages. Default creation remains
non-waiting. See [capability semantics and rollout decisions](docs/mcp-capabilities.md)
for protocol behavior, trust boundaries, Elicitation, polling events, optional Sampling, and multimodal output.

## Remote HTTPS and advanced interactions

Stdio remains default. Opt-in Streamable HTTP uses TLS 1.3 and pinned client
certificates, per-principal roles, separate credentials/state, request limits and
isolated SSE replay. See [remote deployment](docs/remote.md) and the
[example configuration](examples/remote.example.json). This is mTLS, not an OAuth
server; clients must support client certificates.

- Required Elicitation adds form confirmation without bypassing authorization.
- `openstack://events` retains 100 observed transitions, polled every 15 seconds
  while subscribed. It is not an exhaustive cloud event bus.
- `analyze_agent_activity` optionally sends aggregate counts to a consenting
  client's model; raw logs and credentials are excluded.
- `render_instance_topology` returns text and an optional labeled PNG of observed
  VM/network membership, with explicit size limits.

The [capability reference](docs/mcp-capabilities.md) documents limits and fallback
behavior. Follow [live acceptance](docs/live-validation.md) before rollout;
automated fixture/HTTPS tests do not certify a live cloud deployment.
