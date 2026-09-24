package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haramj/openstack-mcp-server/internal/openstack"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func advancedSession(t *testing.T, options *mcp.ClientOptions) (*mcp.ClientSession, *mcp.Server, *EventFeed) {
	t.Helper()
	feed := NewEventFeed()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, &mcp.ServerOptions{SubscribeHandler: feed.Subscribe, UnsubscribeHandler: feed.Unsubscribe})
	feed.Register(server)
	RegisterAdvancedTools(server)
	RegisterAgentTools(server)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, options)
	cs, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, server, feed
}
func TestElicitationAcceptanceDeclineAndUnsupported(t *testing.T) {
	for _, action := range []string{"accept", "decline", "cancel", "unsupported"} {
		t.Run(action, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "memory")
			t.Setenv("OPENSTACK_MCP_MEMORY_FILE", file)
			t.Setenv("OPENSTACK_MCP_REQUIRE_ELICITATION", "1")
			var options *mcp.ClientOptions
			if action != "unsupported" {
				options = &mcp.ClientOptions{ElicitationHandler: func(ctx context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					return &mcp.ElicitResult{Action: action, Content: map[string]any{"confirm": action == "accept"}}, nil
				}}
			}
			cs, _, _ := advancedSession(t, options)
			result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "record_agent_memory", Arguments: map[string]any{"note": "new note"}})
			if err != nil {
				t.Fatal(err)
			}
			if (action == "accept") == result.IsError {
				t.Fatalf("unexpected result: %+v", result)
			}
			_, err = os.Stat(file)
			if (action == "accept") != (err == nil) {
				t.Fatal("mutation before approval or missing approved mutation")
			}
		})
	}
}
func TestSamplingOnlySendsCounts(t *testing.T) {
	file := filepath.Join(t.TempDir(), "audit")
	t.Setenv("OPENSTACK_MCP_AUDIT_LOG", file)
	t.Setenv("OPENSTACK_MCP_ALLOW_SAMPLING", "1")
	data, _ := json.Marshal(openstack.AuditEvent{Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Status: "failed", Target: "PRIVATE_TARGET", Operation: "PRIVATE_OPERATION", Error: "PRIVATE_ERROR"})
	os.WriteFile(file, append(data, '\n'), 0600)
	sampled := make(chan string, 1)
	cs, _, _ := advancedSession(t, &mcp.ClientOptions{CreateMessageHandler: func(ctx context.Context, r *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
		payload, _ := json.Marshal(r.Params)
		sampled <- string(payload)
		return &mcp.CreateMessageResult{Role: "assistant", Model: "test", Content: &mcp.TextContent{Text: "Review the failure count."}}, nil
	}})
	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "analyze_agent_activity", Arguments: map[string]any{"allow_sampling": true}})
	if err != nil || result.IsError {
		t.Fatalf("%+v %v", result, err)
	}
	select {
	case payload := <-sampled:
		if strings.Contains(payload, "PRIVATE_") || strings.Contains(payload, file) || !strings.Contains(payload, `"includeContext":"none"`) {
			t.Fatalf("unsafe sampling: %s", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("sampling missing")
	}
	text, _ := json.Marshal(result)
	if !strings.Contains(string(text), "Untrusted, model-generated advisory") {
		t.Fatalf("missing advisory boundary: %s", text)
	}
	_, err = cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "analyze_agent_activity", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-sampled:
		t.Fatal("sampled without consent")
	default:
	}
}
func TestSamplingUnsupportedFallback(t *testing.T) {
	t.Setenv("OPENSTACK_MCP_ALLOW_SAMPLING", "1")
	t.Setenv("OPENSTACK_MCP_AUDIT_LOG", filepath.Join(t.TempDir(), "audit"))
	cs, _, _ := advancedSession(t, nil)
	r, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "analyze_agent_activity", Arguments: map[string]any{"allow_sampling": true}})
	if err != nil || r.IsError {
		t.Fatalf("%+v %v", r, err)
	}
	data, _ := json.Marshal(r)
	if !strings.Contains(string(data), "aggregate fallback") {
		t.Fatal("missing fallback")
	}
}
func TestTopologyPNGAndTextFallback(t *testing.T) {
	instances := []openstack.Instance{{ID: "id", Name: "vm", Status: "ACTIVE", Networks: map[string][]string{"net": {"192.0.2.1"}}}}
	result, err := topology(instances, true)
	if err != nil {
		t.Fatal(err)
	}
	image := result.Content[2].(*mcp.ImageContent)
	if _, err := png.Decode(bytes.NewReader(image.Data)); err != nil {
		t.Fatal(err)
	}
	result, err = topology(instances, false)
	if err != nil || len(result.Content) != 2 {
		t.Fatal("text fallback failed")
	}
	if !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "192.0.2.1") {
		t.Fatal("missing textual evidence")
	}
}
func TestEventTransitionsCoverageAndBound(t *testing.T) {
	f := NewEventFeed()
	status := "BUILD"
	fail := false
	f.read = func(context.Context) ([]openstack.Instance, error) {
		if fail {
			return nil, errors.New("PRIVATE_BACKEND")
		}
		return []openstack.Instance{{ID: "vm", Status: status}}, nil
	}
	if !f.poll(context.Background()) || f.events[0].Kind != "baseline" {
		t.Fatal("missing baseline")
	}
	status = "ACTIVE"
	f.poll(context.Background())
	if f.events[1].Before != "BUILD" || f.events[1].After != "ACTIVE" {
		t.Fatal("missing transition")
	}
	fail = true
	f.poll(context.Background())
	if f.events[2].Kind != "coverage_unavailable" {
		t.Fatal("missing gap")
	}
	fail = false
	status = "ERROR"
	f.poll(context.Background())
	if f.events[3].Kind != "baseline" {
		t.Fatal("gap falsely bridged")
	}
	for i := 0; i < 120; i++ {
		status += "x"
		f.poll(context.Background())
	}
	if len(f.events) != 100 || f.sequence <= 100 {
		t.Fatal("unbounded history")
	}
}
func TestEventSubscriptionOverMCP(t *testing.T) {
	notifications := make(chan string, 2)
	cs, server, feed := advancedSession(t, &mcp.ClientOptions{ResourceUpdatedHandler: func(ctx context.Context, r *mcp.ResourceUpdatedNotificationRequest) { notifications <- r.Params.URI }})
	subscribeDone := make(chan error, 1)
	go func() { subscribeDone <- cs.Subscribe(context.Background(), &mcp.SubscribeParams{URI: eventsURI}) }()
	deadline := time.Now().Add(time.Second)
	for {
		feed.mu.Lock()
		count := len(feed.subscriptions)
		feed.mu.Unlock()
		if count > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("subscription not registered")
		}
		time.Sleep(time.Millisecond)
	}
	feed.read = func(context.Context) ([]openstack.Instance, error) {
		return []openstack.Instance{{ID: "vm", Status: "ACTIVE"}}, nil
	}
	feed.poll(context.Background())
	if err := server.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: eventsURI}); err != nil {
		t.Fatal(err)
	}
	select {
	case uri := <-notifications:
		if uri != eventsURI {
			t.Fatal(uri)
		}
	case <-time.After(time.Second):
		t.Fatal("missing resource notification")
	}
	r, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: eventsURI})
	if err != nil || !strings.Contains(r.Contents[0].Text, "baseline") {
		t.Fatalf("%+v %v", r, err)
	}
	if err := cs.Unsubscribe(context.Background(), &mcp.UnsubscribeParams{URI: eventsURI}); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for {
		feed.mu.Lock()
		count := len(feed.subscriptions)
		feed.mu.Unlock()
		if count == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("subscription leaked")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestInteractiveStateRejectsTamperingAndChangedArguments(t *testing.T) {
	request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "delete_instance", Arguments: json.RawMessage(`{"name":"dev"}`)}}
	request.Params.RequestState = issueState(context.Background(), request, nil)
	if _, err := readState(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Params.Arguments = json.RawMessage(`{"name":"prod"}`)
	if _, err := readState(context.Background(), request); err == nil {
		t.Fatal("approval reused for another target")
	}
	request.Params.RequestState += "tampered"
	if _, err := readState(context.Background(), request); err == nil {
		t.Fatal("tampered approval accepted")
	}
}
