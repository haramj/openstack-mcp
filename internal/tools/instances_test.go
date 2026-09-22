package tools

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"path/filepath"
	"testing"
)

func testSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	RegisterInstanceTools(server)
	RegisterAgentTools(server)
	RegisterResources(server)
	RegisterPrompts(server)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}
func TestDeleteMismatchRejectedOverMCP(t *testing.T) {
	t.Setenv("OPENSTACK_MCP_AUDIT_LOG", filepath.Join(t.TempDir(), "audit"))
	t.Setenv("PATH", t.TempDir())
	cs := testSession(t)
	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "delete_instance", Arguments: map[string]any{"name": "dev", "confirm_name": "wrong"}})
	if err != nil || !result.IsError {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestCreateDefaultInputMapping(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OPENSTACK_MCP_AUDIT_LOG", filepath.Join(dir, "audit"))
	t.Setenv("ARGV_LOG", filepath.Join(dir, "args"))
	os.WriteFile(filepath.Join(dir, "openstack"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ARGV_LOG\"\necho '{\"id\":\"created\",\"status\":\"BUILD\"}'\n"), 0700)
	cs := testSession(t)
	r, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_instance", Arguments: map[string]any{"name": "dev", "image": "img", "flavor": "f", "network": "n"}})
	if err != nil || r.IsError {
		t.Fatalf("%+v %v", r, err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "args"))
	if string(data) != "server\ncreate\n--image\nimg\n--flavor\nf\n--network\nn\n-f\njson\ndev\n" {
		t.Fatalf("%s", data)
	}
}
