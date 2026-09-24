package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/haramj/openstack-mcp-server/internal/openstack"
	"github.com/haramj/openstack-mcp-server/internal/scope"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"path"
)

// Remote identity checks are independent of mutable planner memory. Resolve IDs
// before checking names, then mutate the exact resolved ID to avoid name races.
func authorizedTarget(ctx context.Context, target string) (string, error) {
	config, remote := scope.From(ctx)
	if !remote {
		return target, nil
	}
	instance, err := openstack.GetInstance(ctx, target)
	if err != nil {
		return "", err
	}
	patterns := config.ProtectedPatterns
	if patterns == nil {
		patterns = []string{"prod-*", "*-prod", "production-*"}
	}
	for _, pattern := range patterns {
		match, err := path.Match(pattern, instance.Name)
		if err != nil {
			return "", fmt.Errorf("invalid protected pattern")
		}
		if match {
			return "", fmt.Errorf("target is protected by remote policy")
		}
	}
	return instance.ID, nil
}

// Elicitation is an additional interaction, not authentication or an override.
func confirmInteraction(ctx context.Context, request *mcp.CallToolRequest, operation string) (*mcp.CallToolResult, error) {
	required := os.Getenv("OPENSTACK_MCP_REQUIRE_ELICITATION") == "1"
	if config, ok := scope.From(ctx); ok {
		required = config.RequireElicitation
	}
	if !required {
		return nil, nil
	}
	if request == nil || request.Params == nil || request.ClientCapabilities() == nil || request.ClientCapabilities().Elicitation == nil {
		return nil, fmt.Errorf("this operation requires a client with form elicitation")
	}
	if len(request.Params.InputResponses) == 0 {
		return &mcp.CallToolResult{RequestState: issueState(ctx, request, func() []byte { data, _ := json.Marshal(operation); return data }()), InputRequests: mcp.InputRequestMap{"confirmation": &mcp.ElicitParams{Mode: "form", Message: "Confirm " + operation + ". This modifies cloud or local state.", RequestedSchema: map[string]any{"type": "object", "properties": map[string]any{"confirm": map[string]any{"type": "boolean", "title": "Approve this operation"}}, "required": []string{"confirm"}}}}}, nil
	}
	original, err := readState(ctx, request)
	if err != nil {
		return nil, err
	}
	var confirmedOperation string
	if json.Unmarshal(original, &confirmedOperation) != nil || confirmedOperation != operation {
		return nil, fmt.Errorf("target identity changed; request a new confirmation")
	}
	result, ok := request.Params.InputResponses["confirmation"].(*mcp.ElicitResult)
	if !ok || result.Action != "accept" || result.Content["confirm"] != true {
		return nil, fmt.Errorf("operation declined or canceled")
	}
	return nil, nil
}
