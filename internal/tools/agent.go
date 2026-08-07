package tools

import (
	"context"

	"github.com/haramj/openstack-mcp-server/internal/agent"
	"github.com/haramj/openstack-mcp-server/internal/openstack"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetAgentMemoryInput struct{}

type PlanInstanceOperationInput struct {
	Operation   string `json:"operation" jsonschema:"Operation to plan: list, get, create, delete, or admin_instance_action"`
	Name        string `json:"name,omitempty" jsonschema:"Target instance name"`
	Action      string `json:"action,omitempty" jsonschema:"Lifecycle action for admin_instance_action"`
	RebootType  string `json:"reboot_type,omitempty" jsonschema:"soft or hard for reboot actions"`
	Image       string `json:"image,omitempty" jsonschema:"Image name or ID for create"`
	Flavor      string `json:"flavor,omitempty" jsonschema:"Flavor name or ID for create"`
	Network     string `json:"network,omitempty" jsonschema:"Network name or ID for create"`
	ConfirmName string `json:"confirm_name,omitempty" jsonschema:"Confirmation name for delete"`
}

type RecordAgentMemoryInput struct {
	DefaultImage              string   `json:"default_image,omitempty" jsonschema:"Remembered default image name or ID"`
	DefaultFlavor             string   `json:"default_flavor,omitempty" jsonschema:"Remembered default flavor name or ID"`
	DefaultNetwork            string   `json:"default_network,omitempty" jsonschema:"Remembered default network name or ID"`
	DeleteRequiresConfirmName *bool    `json:"delete_requires_confirm_name,omitempty" jsonschema:"Whether delete_instance must require exact confirm_name"`
	ProtectedInstancePatterns []string `json:"protected_instance_patterns,omitempty" jsonschema:"Glob patterns for instances that planner should protect from risky operations"`
	Note                      string   `json:"note,omitempty" jsonschema:"Free-form operational note to remember. Do not store secrets."`
}

type SummarizeAgentActivityInput struct {
	SinceHours int `json:"since_hours,omitempty" jsonschema:"How many hours of audit log activity to summarize. Defaults to 12."`
	Limit      int `json:"limit,omitempty" jsonschema:"Maximum number of recent audit events to include. Defaults to 20."`
}

func RegisterAgentTools(server *mcp.Server) {
	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "get_agent_memory",
			Annotations: readOnlyAnnotations("Get Agent Memory"),
			Description: "Read remembered OpenStack agent policy and defaults such as default image, flavor, network, and protected instance patterns. This operation is read-only.",
		},
		func(
			ctx context.Context,
			request *mcp.CallToolRequest,
			input GetAgentMemoryInput,
		) (*mcp.CallToolResult, *agent.Memory, error) {
			memory, err := agent.LoadMemory()
			if err != nil {
				return nil, nil, err
			}

			return nil, &memory, nil
		},
	)

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "plan_instance_operation",
			Annotations: readOnlyAnnotations("Plan Instance Operation"),
			Description: "Plan which OpenStack MCP tool and arguments should be used for an instance operation. It applies remembered defaults and blocks risky operations against protected instances. This operation is read-only and does not call OpenStack.",
		},
		func(
			ctx context.Context,
			request *mcp.CallToolRequest,
			input PlanInstanceOperationInput,
		) (*mcp.CallToolResult, *agent.Plan, error) {
			memory, err := agent.LoadMemory()
			if err != nil {
				return nil, nil, err
			}

			plan := agent.PlanInstanceOperation(agent.PlanInput{
				Operation:   input.Operation,
				Name:        input.Name,
				Action:      input.Action,
				RebootType:  input.RebootType,
				Image:       input.Image,
				Flavor:      input.Flavor,
				Network:     input.Network,
				ConfirmName: input.ConfirmName,
			}, memory)

			return nil, &plan, nil
		},
	)

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "record_agent_memory",
			Annotations: adminAnnotations("Record Agent Memory", false, true),
			Description: "Update remembered OpenStack agent policy/defaults. Use for non-secret operational preferences only, such as default network or protected instance patterns. This modifies local MCP memory.",
		},
		func(
			ctx context.Context,
			request *mcp.CallToolRequest,
			input RecordAgentMemoryInput,
		) (*mcp.CallToolResult, *agent.Memory, error) {
			memory, err := agent.UpdateMemory(agent.MemoryPatch{
				DefaultImage:              input.DefaultImage,
				DefaultFlavor:             input.DefaultFlavor,
				DefaultNetwork:            input.DefaultNetwork,
				DeleteRequiresConfirmName: input.DeleteRequiresConfirmName,
				ProtectedInstancePatterns: input.ProtectedInstancePatterns,
				Note:                      input.Note,
			})
			if err != nil {
				return nil, nil, err
			}

			return nil, &memory, nil
		},
	)

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "summarize_agent_activity",
			Annotations: readOnlyAnnotations("Summarize Agent Activity"),
			Description: "Summarize recent administrator MCP audit activity, including success/failure/rejected counts, destructive operations, events needing attention, and recommendations. This operation is read-only and does not call OpenStack.",
		},
		func(
			ctx context.Context,
			request *mcp.CallToolRequest,
			input SummarizeAgentActivityInput,
		) (*mcp.CallToolResult, *openstack.ActivitySummary, error) {
			summary, err := openstack.SummarizeAgentActivity(openstack.ActivitySummaryOptions{
				SinceHours: input.SinceHours,
				Limit:      input.Limit,
			})
			if err != nil {
				return nil, nil, err
			}

			return nil, summary, nil
		},
	)
}
