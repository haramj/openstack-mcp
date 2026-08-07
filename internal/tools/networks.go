package tools

import (
	"context"

	"github.com/haramj/openstack-mcp-server/internal/openstack"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListNetworksInput struct{}

type ListNetworksOutput struct {
	Networks []openstack.Network `json:"networks"`
}

func RegisterNetworkTools(server *mcp.Server) {
	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "list_networks",
			Description: "List OpenStack networks with their IDs and names. This operation is read-only.",
		},
		func(
			ctx context.Context,
			request *mcp.CallToolRequest,
			input ListNetworksInput,
		) (*mcp.CallToolResult, *ListNetworksOutput, error) {
			networks, err := openstack.ListNetworks(ctx)
			if err != nil {
				return nil, nil, err
			}

			return nil, &ListNetworksOutput{
				Networks: networks,
			}, nil
		},
	)
}
