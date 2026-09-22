package agent

import (
	"fmt"
	"path"
	"strings"
)

type PlanInput struct {
	Operation   string `json:"operation"`
	Name        string `json:"name,omitempty"`
	Action      string `json:"action,omitempty"`
	RebootType  string `json:"reboot_type,omitempty"`
	Image       string `json:"image,omitempty"`
	Flavor      string `json:"flavor,omitempty"`
	Network     string `json:"network,omitempty"`
	ConfirmName string `json:"confirm_name,omitempty"`
}

type Plan struct {
	RecommendedTool  string         `json:"recommended_tool"`
	Arguments        map[string]any `json:"arguments"`
	RequiresApproval bool           `json:"requires_approval"`
	Blocked          bool           `json:"blocked"`
	Reasons          []string       `json:"reasons,omitempty"`
	Warnings         []string       `json:"warnings,omitempty"`
	Memory           Memory         `json:"memory"`
}

func PlanInstanceOperation(input PlanInput, memory Memory) Plan {
	operation := normalize(input.Operation)
	plan := Plan{
		Arguments: map[string]any{},
		Memory:    memory,
	}

	switch operation {
	case "list_instances", "list":
		plan.RecommendedTool = "list_instances"
	case "get_instance", "get":
		plan.RecommendedTool = "get_instance"
		plan.Arguments["name"] = input.Name
		requireName(&plan, input.Name)
	case "create_instance", "create":
		plan.RecommendedTool = "create_instance"
		plan.RequiresApproval = true
		plan.Arguments["name"] = input.Name
		plan.Arguments["image"] = firstNonEmpty(input.Image, memory.DefaultImage)
		plan.Arguments["flavor"] = firstNonEmpty(input.Flavor, memory.DefaultFlavor)
		plan.Arguments["network"] = firstNonEmpty(input.Network, memory.DefaultNetwork)
		requireName(&plan, input.Name)
		if input.Image == "" {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("using remembered default image %q", memory.DefaultImage))
		}
		if input.Flavor == "" {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("using remembered default flavor %q", memory.DefaultFlavor))
		}
		if input.Network == "" {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("using remembered default network %q", memory.DefaultNetwork))
		}
	case "delete_instance", "delete":
		plan.RecommendedTool = "delete_instance"
		plan.RequiresApproval = true
		plan.Arguments["name"] = input.Name
		plan.Arguments["confirm_name"] = input.ConfirmName
		requireName(&plan, input.Name)
		if memory.DeleteRequiresConfirmName && input.Name != input.ConfirmName {
			block(&plan, "delete_instance requires confirm_name to exactly match name")
		}
		blockIfProtected(&plan, input.Name, memory)
	case "admin_instance_action", "action", "lifecycle":
		plan.RecommendedTool = "admin_instance_action"
		plan.RequiresApproval = true
		plan.Arguments["name"] = input.Name
		plan.Arguments["action"] = input.Action
		if input.RebootType != "" {
			plan.Arguments["reboot_type"] = input.RebootType
		}
		requireName(&plan, input.Name)
		if input.Action == "" {
			block(&plan, "action is required for admin_instance_action")
		}
		if isRiskyLifecycle(input.Action, input.RebootType) {
			blockIfProtected(&plan, input.Name, memory)
		}
	default:
		block(&plan, fmt.Sprintf("unsupported operation %q", input.Operation))
	}

	if plan.Blocked {
		plan.Arguments = map[string]any{}
	}
	return plan
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

func requireName(plan *Plan, name string) {
	if name == "" {
		block(plan, "name is required")
	}
}

func block(plan *Plan, reason string) {
	plan.Blocked = true
	plan.Reasons = append(plan.Reasons, reason)
}

func blockIfProtected(plan *Plan, name string, memory Memory) {
	if name == "" {
		return
	}

	for _, pattern := range memory.ProtectedInstancePatterns {
		if pattern == "" {
			continue
		}
		if matched, _ := path.Match(pattern, name); matched || pattern == name {
			block(plan, fmt.Sprintf("target %q matches protected instance pattern %q", name, pattern))
			return
		}
	}
}

func isRiskyLifecycle(action string, rebootType string) bool {
	switch normalize(action) {
	case "stop", "pause", "suspend", "shelve":
		return true
	case "reboot":
		return normalize(rebootType) == "hard"
	default:
		return false
	}
}
