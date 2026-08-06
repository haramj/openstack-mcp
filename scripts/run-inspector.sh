#!/usr/bin/env bash

set -euo pipefail

echo "==> Building..."
make build

echo "==> Starting MCP Inspector..."
npx @modelcontextprotocol/inspector ./openstack-mcp-server
