package tools

import (
	"context"

	"github.com/haramj/openstack-mcp-server/internal/openstack"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListFlavorsInput struct{}

type ListFlavorsOutput struct {
	Flavors []openstack.Flavor `json:"flavors"`
}

func RegisterFlavorTools(server *mcp.Server) {
	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "list_flavors",
			Annotations: readOnlyAnnotations("List Flavors"),
			Description: "List OpenStack flavors with their IDs, names, RAM, disk, and vCPU counts. " +
				"This operation is read-only.",
		},
		func(
			ctx context.Context,
			request *mcp.CallToolRequest,
			input ListFlavorsInput,
		) (*mcp.CallToolResult, *ListFlavorsOutput, error) {
			flavors, err := openstack.ListFlavors(ctx)
			if err != nil {
				return nil, nil, err
			}

			return nil, &ListFlavorsOutput{
				Flavors: flavors,
			}, nil
		},
	)
}
