#!/usr/bin/env bash

set -euo pipefail

echo "==> Formatting..."
make fmt

echo "==> Building..."
make build

echo "==> Installing..."
install -m 755 openstack-mcp-server "$HOME/.local/bin/openstack-mcp-server"

echo
echo "Done!"
echo "Installed to: $HOME/.local/bin/openstack-mcp-server"
