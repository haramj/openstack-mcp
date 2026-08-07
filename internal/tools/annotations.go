package tools

import "github.com/modelcontextprotocol/go-sdk/mcp"

func boolPtr(value bool) *bool {
	return &value
}

func readOnlyAnnotations(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:        title,
		ReadOnlyHint: true,
	}
}

func adminAnnotations(title string, destructive bool, idempotent bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		ReadOnlyHint:    false,
		DestructiveHint: boolPtr(destructive),
		IdempotentHint:  idempotent,
		OpenWorldHint:   boolPtr(false),
	}
}
