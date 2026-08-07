package tools

import (
	"context"

	"github.com/haramj/openstack-mcp-server/internal/openstack"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListInstancesInput struct{}

type GetInstanceInput struct {
	Name string `json:"name" jsonschema:"Name of the OpenStack instance"`
}

type ListInstancesOutput struct {
	Instances []openstack.Instance `json:"instances"`
}

func RegisterInstanceTools(server *mcp.Server) {
	mcp.AddTool(
		server,
		&mcp.Tool{
			Name: "list_instances",
			Description: "List virtual machine instances in the Return OpenStack " +
				"project. Returns IDs, names, statuses, networks, images, and flavors. " +
				"This operation is read-only.",
		},
		func(
			ctx context.Context,
			request *mcp.CallToolRequest,
			input ListInstancesInput,
		) (*mcp.CallToolResult, *ListInstancesOutput, error) {
			instances, err := openstack.ListInstances(ctx)
			if err != nil {
				return nil, nil, err
			}

			return nil, &ListInstancesOutput{
				Instances: instances,
			}, nil
		},
	)

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "get_instance",
			Description: "Get detailed information about an OpenStack instance by name. This operation is read-only.",
		},
		func(
			ctx context.Context,
			request *mcp.CallToolRequest,
			input GetInstanceInput,
		) (*mcp.CallToolResult, *openstack.Instance, error) {
			instance, err := openstack.GetInstance(ctx, input.Name)
			if err != nil {
				return nil, nil, err
			}

			return nil, instance, nil
		},
	)
}
