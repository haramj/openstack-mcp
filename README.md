# OpenStack MCP

An MCP (Model Context Protocol) server for OpenStack built with the official Go SDK.

This project provides an MCP server that exposes OpenStack resources as MCP tools, allowing AI assistants and MCP-compatible clients to interact with OpenStack environments through a standardized interface.

## Features

Currently implemented tools:

- `list_instances`
  - List all instances in the current OpenStack project.

- `get_instance`
  - Get detailed information about an instance by name.

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
  - Optional fields: `key_name`, `security_groups`, and `no_wait`.
  - By default, the tool waits for OpenStack to finish building the instance.

- `delete_instance`
  - Delete an instance by name.
  - Requires `confirm_name` to exactly match `name` before deletion.

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
