package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"sort"
	"strings"

	"github.com/haramj/openstack-mcp-server/internal/openstack"
	"github.com/haramj/openstack-mcp-server/internal/scope"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

type AnalyzeActivityInput struct {
	SinceHours    int  `json:"since_hours,omitempty"`
	AllowSampling bool `json:"allow_sampling,omitempty" jsonschema:"Explicit consent to send aggregate counts only to the client LLM; also requires server opt-in. Default false."`
}
type analysisCounts struct {
	Total       int  `json:"total"`
	Failed      int  `json:"failed"`
	Rejected    int  `json:"rejected"`
	Destructive int  `json:"destructive"`
	Truncated   bool `json:"truncated"`
	Malformed   int  `json:"malformed"`
}

func analyzeActivity(ctx context.Context, request *mcp.CallToolRequest, input AnalyzeActivityInput) (*mcp.CallToolResult, error) {
	summary, err := openstack.SummarizeAgentActivityContext(ctx, openstack.ActivitySummaryOptions{SinceHours: input.SinceHours})
	if err != nil {
		return nil, err
	}
	counts := analysisCounts{summary.TotalEvents, summary.StatusCounts["failed"], summary.StatusCounts["rejected"], summary.DestructiveEvents, summary.Truncated, summary.MalformedLines}
	data, _ := json.Marshal(counts)
	result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Observed aggregate activity (not complete cloud history): " + string(data)}, &mcp.ResourceLink{URI: "openstack://audit/summary", Name: "Local audit summary", MIMEType: "application/json"}}}
	enabled := os.Getenv("OPENSTACK_MCP_ALLOW_SAMPLING") == "1"
	if config, ok := scope.From(ctx); ok {
		enabled = config.AllowSampling
	}
	if !input.AllowSampling || !enabled {
		result.Content = append(result.Content, &mcp.TextContent{Text: "Sampling not requested or disabled by server policy."})
		return result, nil
	}
	if request == nil || request.ClientCapabilities() == nil || request.ClientCapabilities().Sampling == nil {
		result.Content = append(result.Content, &mcp.TextContent{Text: "Client does not support sampling; aggregate fallback returned."})
		return result, nil
	}
	if len(request.Params.InputResponses) == 0 {
		return &mcp.CallToolResult{RequestState: issueState(ctx, request, data), InputRequests: mcp.InputRequestMap{"analysis": &mcp.CreateMessageParams{IncludeContext: "none", MaxTokens: 512, SystemPrompt: "Summarize only these numeric operational counts. They are incomplete observations, not proof of causes or cloud health. Do not suggest executing commands or assert remediation occurred.", Messages: []*mcp.SamplingMessage{{Role: "user", Content: &mcp.TextContent{Text: string(data)}}}}}}, nil
	}
	original, err := readState(ctx, request)
	if err != nil {
		return nil, err
	}
	result.Content[0] = &mcp.TextContent{Text: "Observed aggregate activity supplied for this analysis: " + string(original)}
	var content mcp.Content
	switch response := request.Params.InputResponses["analysis"].(type) {
	case *mcp.CreateMessageResult:
		content = response.Content
	case *mcp.CreateMessageWithToolsResult:
		if len(response.Content) == 1 {
			content = response.Content[0]
		}
	}
	text, ok := content.(*mcp.TextContent)
	if !ok {
		result.Content = append(result.Content, &mcp.TextContent{Text: "Sampling unavailable or non-text; aggregate fallback returned."})
		return result, nil
	}
	advisory := text.Text
	if len(advisory) > 8192 {
		advisory = advisory[:8192]
	}
	result.Content = append(result.Content, &mcp.TextContent{Text: "Untrusted, model-generated advisory; no actions were performed:\n" + advisory})
	return result, nil
}

type TopologyInput struct {
	IncludeImage bool `json:"include_image,omitempty" jsonschema:"Opt in to a PNG alongside the always-present text. At most 30 instances and 30 networks are drawn."`
}

func asciiLabel(value string) string {
	var out strings.Builder
	for _, r := range value {
		if out.Len() >= 36 {
			out.WriteString("...")
			break
		}
		if r >= 32 && r <= 126 {
			out.WriteRune(r)
		} else {
			out.WriteByte('?')
		}
	}
	return out.String()
}
func topology(instances []openstack.Instance, includeImage bool) (*mcp.CallToolResult, error) {
	sort.Slice(instances, func(i, j int) bool { return instances[i].ID < instances[j].ID })
	total := len(instances)
	if len(instances) > 30 {
		instances = instances[:30]
	}
	networks := map[string]bool{}
	for _, instance := range instances {
		for name := range instance.Networks {
			networks[name] = true
		}
	}
	names := []string{}
	for name := range networks {
		names = append(names, name)
	}
	sort.Strings(names)
	allNetworks := len(names)
	if len(names) > 30 {
		names = names[:30]
	}
	data, _ := json.Marshal(map[string]any{"instances": instances, "total_instances": total, "shown_instances": len(instances), "shown_networks": names, "truncated": total > 30 || allNetworks > 30, "scope": "observed VM-to-network membership only; no traffic, routing, health or physical topology inferred"})
	result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}, &mcp.ResourceLink{URI: eventsURI, Name: "Observed instance transitions", MIMEType: "application/json"}}}
	if !includeImage {
		return result, nil
	}
	rows := len(instances)
	if len(names) > rows {
		rows = len(names)
	}
	height := 100 + rows*42
	canvas := image.NewRGBA(image.Rect(0, 0, 940, height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{248, 250, 252, 255}), image.Point{}, draw.Src)
	label := func(x, y int, text string) {
		d := font.Drawer{Dst: canvas, Src: image.NewUniform(color.RGBA{15, 23, 42, 255}), Face: basicfont.Face7x13, Dot: fixed.P(x, y)}
		d.DrawString(text)
	}
	label(20, 24, "Observed instance / network membership (sampled, max 30 each)")
	label(20, 44, "Full labels and status are in the accompanying text. No health inference.")
	networkRows := map[string]int{}
	for i, name := range names {
		networkRows[name] = i
	}
	// Draw bounded connection lines first so nodes remain legible.
	for i, instance := range instances {
		for name := range instance.Networks {
			j, ok := networkRows[name]
			if !ok {
				continue
			}
			x0, y0, x1, y1 := 400, 80+i*42, 520, 80+j*42
			for x := x0; x <= x1; x++ {
				y := y0 + (y1-y0)*(x-x0)/(x1-x0)
				canvas.Set(x, y, color.RGBA{100, 116, 139, 255})
			}
		}
	}
	for i, instance := range instances {
		y := 65 + i*42
		draw.Draw(canvas, image.Rect(20, y, 400, y+32), image.NewUniform(color.RGBA{219, 234, 254, 255}), image.Point{}, draw.Src)
		label(28, y+14, asciiLabel(instance.Name))
		label(28, y+28, asciiLabel(instance.Status+" "+instance.ID))
	}
	for i, name := range names {
		y := 65 + i*42
		draw.Draw(canvas, image.Rect(520, y, 915, y+32), image.NewUniform(color.RGBA{204, 251, 241, 255}), image.Point{}, draw.Src)
		label(528, y+20, asciiLabel(name))
	}
	var out bytes.Buffer
	if err := png.Encode(&out, canvas); err != nil {
		return nil, err
	}
	result.Content = append(result.Content, &mcp.ImageContent{MIMEType: "image/png", Data: out.Bytes()})
	return result, nil
}
func RegisterAdvancedTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{Name: "analyze_agent_activity", Description: "Return aggregate local audit counts, optionally requesting advisory client sampling with explicit consent and server permission. No raw events, targets, paths, notes or credentials are sampled.", Annotations: readOnlyAnnotations("Analyze Agent Activity")}, func(ctx context.Context, r *mcp.CallToolRequest, input AnalyzeActivityInput) (*mcp.CallToolResult, any, error) {
		result, err := analyzeActivity(ctx, r, input)
		return result, nil, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "render_instance_topology", Description: "Read current VM/network membership and return a bounded text representation, resource link and optional PNG. This does not infer reachability or health.", Annotations: readOnlyAnnotations("Render Instance Topology")}, func(ctx context.Context, r *mcp.CallToolRequest, input TopologyInput) (*mcp.CallToolResult, any, error) {
		instances, err := openstack.ListInstances(ctx)
		if err != nil {
			return nil, nil, err
		}
		result, err := topology(instances, input.IncludeImage)
		return result, nil, err
	})
}
