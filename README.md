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

Before running the server, make sure your OpenStack credentials are loaded.

```bash
source openrc
```

Run the MCP server.

```bash
./openstack-mcp-server
```

## Testing

You can test the server using the official MCP Inspector.

```bash
npx @modelcontextprotocol/inspector ./openstack-mcp-server
```

## Project Structure

```
.
├── cmd/
│   └── openstack-mcp-server/
│       └── main.go
├── internal/
│   ├── openstack/
│   │   ├── instances.go
│   │   └── networks.go
│   └── tools/
│       ├── instances.go
│       └── networks.go
├── scripts/
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
