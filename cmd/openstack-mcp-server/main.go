package main

import (
	"context"
	"log"

	"github.com/haramj/openstack-mcp-server/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "return-openstack-mcp",
			Version: "0.1.0",
		},
		nil,
	)

	tools.RegisterAgentTools(server)
	tools.RegisterInstanceTools(server)
	tools.RegisterNetworkTools(server)
	tools.RegisterImageTools(server)
	tools.RegisterFlavorTools(server)

	if err := server.Run(
		context.Background(),
		&mcp.StdioTransport{},
	); err != nil {
		log.Fatalf("MCP server failed: %v", err)
	}
}
