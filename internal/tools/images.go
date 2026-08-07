package tools

import (
	"context"

	"github.com/haramj/openstack-mcp-server/internal/openstack"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListImagesInput struct{}

type ListImagesOutput struct {
	Images []openstack.Image `json:"images"`
}

func RegisterImageTools(server *mcp.Server) {
	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "list_images",
			Annotations: readOnlyAnnotations("List Images"),
			Description: "List OpenStack images with their IDs, names, and statuses. " +
				"This operation is read-only.",
		},
		func(
			ctx context.Context,
			request *mcp.CallToolRequest,
			input ListImagesInput,
		) (*mcp.CallToolResult, *ListImagesOutput, error) {
			images, err := openstack.ListImages(ctx)
			if err != nil {
				return nil, nil, err
			}

			return nil, &ListImagesOutput{
				Images: images,
			}, nil
		},
	)
}
