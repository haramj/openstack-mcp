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

type AdminInstanceActionInput struct {
	Name       string `json:"name" jsonschema:"Name of the OpenStack instance"`
	Action     string `json:"action" jsonschema:"Action to run: start, stop, reboot, pause, unpause, suspend, resume, shelve, unshelve, lock, or unlock"`
	RebootType string `json:"reboot_type,omitempty" jsonschema:"For action=reboot only. Use soft or hard. Defaults to soft."`
}

type CreateInstanceInput struct {
	Name           string   `json:"name" jsonschema:"Name for the new OpenStack instance"`
	Image          string   `json:"image" jsonschema:"Image name or ID"`
	Flavor         string   `json:"flavor" jsonschema:"Flavor name or ID"`
	Network        string   `json:"network" jsonschema:"Network name or ID"`
	KeyName        string   `json:"key_name,omitempty" jsonschema:"Optional keypair name"`
	SecurityGroups []string `json:"security_groups,omitempty" jsonschema:"Optional security group names"`
	NoWait         bool     `json:"no_wait" jsonschema:"If true, return after the create request is accepted instead of waiting for OpenStack to finish building the instance"`
}

type DeleteInstanceInput struct {
	Name        string `json:"name" jsonschema:"Name of the OpenStack instance to delete"`
	ConfirmName string `json:"confirm_name" jsonschema:"Must exactly match name. This prevents accidental deletion."`
}

func RegisterInstanceTools(server *mcp.Server) {
	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "list_instances",
			Annotations: readOnlyAnnotations("List Instances"),
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
			Annotations: readOnlyAnnotations("Get Instance"),
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

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "admin_instance_action",
			Annotations: adminAnnotations("Admin Instance Action", true, true),
			Description: "Run an administrator lifecycle action on an OpenStack instance. Supported actions: start, stop, reboot, pause, unpause, suspend, resume, shelve, unshelve, lock, unlock. This modifies OpenStack state.",
		},
		func(
			ctx context.Context,
			request *mcp.CallToolRequest,
			input AdminInstanceActionInput,
		) (*mcp.CallToolResult, *openstack.InstanceActionResult, error) {
			result, err := openstack.RunInstanceAction(
				ctx,
				input.Name,
				openstack.InstanceAction(input.Action),
				input.RebootType,
			)
			if err != nil {
				return nil, nil, err
			}

			return nil, result, nil
		},
	)

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "create_instance",
			Annotations: adminAnnotations("Create Instance", false, false),
			Description: "Create a new OpenStack instance using an image, flavor, and network. Optional keypair and security groups are supported. This modifies OpenStack state.",
		},
		func(
			ctx context.Context,
			request *mcp.CallToolRequest,
			input CreateInstanceInput,
		) (*mcp.CallToolResult, *openstack.CreatedInstance, error) {
			result, err := openstack.CreateInstance(ctx, openstack.CreateInstanceOptions{
				Name:           input.Name,
				Image:          input.Image,
				Flavor:         input.Flavor,
				Network:        input.Network,
				KeyName:        input.KeyName,
				SecurityGroups: input.SecurityGroups,
				NoWait:         input.NoWait,
			})
			if err != nil {
				return nil, nil, err
			}

			return nil, result, nil
		},
	)

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "delete_instance",
			Annotations: adminAnnotations("Delete Instance", true, false),
			Description: "Delete an OpenStack instance by name. The confirm_name input must exactly match name. This is destructive and modifies OpenStack state.",
		},
		func(
			ctx context.Context,
			request *mcp.CallToolRequest,
			input DeleteInstanceInput,
		) (*mcp.CallToolResult, *openstack.DeleteInstanceResult, error) {
			if input.Name != input.ConfirmName {
				openstack.RecordRejectedAudit(
					"delete_instance",
					input.Name,
					true,
					"confirm_name must exactly match name before deleting an instance",
				)
				return nil, nil, &confirmationError{
					message: "confirm_name must exactly match name before deleting an instance",
				}
			}

			result, err := openstack.DeleteInstance(ctx, input.Name)
			if err != nil {
				return nil, nil, err
			}

			return nil, result, nil
		},
	)
}

type confirmationError struct {
	message string
}

func (e *confirmationError) Error() string {
	return e.message
}
