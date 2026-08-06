package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListInstancesInput struct{}

type GetInstanceInput struct {
	Name string `json:"name" jsonschema:"Name of the OpenStack instance"`
}

type ListNetworksInput struct{}

type Instance struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Status   string              `json:"status"`
	Networks map[string][]string `json:"networks"`
	Image    string              `json:"image"`
	Flavor   string              `json:"flavor"`
}

type ListInstancesOutput struct {
	Instances []Instance `json:"instances"`
}

type openStackInstance struct {
	ID       string              `json:"ID"`
	Name     string              `json:"Name"`
	Status   string              `json:"Status"`
	Networks map[string][]string `json:"Networks"`
	Image    string              `json:"Image"`
	Flavor   string              `json:"Flavor"`
}

type Network struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ListNetworksOutput struct {
	Networks []Network `json:"networks"`
}

type openStackNetwork struct {
	ID   string `json:"ID"`
	Name string `json:"Name"`
}

func listInstances(ctx context.Context) ([]Instance, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		commandCtx,
		"openstack",
		"server",
		"list",
		"-f",
		"json",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if commandCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("openstack command timed out")
		}

		return nil, fmt.Errorf(
			"openstack server list failed: %w: %s",
			err,
			string(output),
		)
	}

	var rawInstances []openStackInstance
	if err := json.Unmarshal(output, &rawInstances); err != nil {
		return nil, fmt.Errorf(
			"failed to decode OpenStack server list output: %w",
			err,
		)
	}

	instances := make([]Instance, 0, len(rawInstances))
	for _, raw := range rawInstances {
		instances = append(instances, Instance{
			ID:       raw.ID,
			Name:     raw.Name,
			Status:   raw.Status,
			Networks: raw.Networks,
			Image:    raw.Image,
			Flavor:   raw.Flavor,
		})
	}

	return instances, nil
}

func getInstance(ctx context.Context, name string) (*Instance, error) {
	instances, err := listInstances(ctx)
	if err != nil {
		return nil, err
	}

	for _, instance := range instances {
		if instance.Name == name {
			return &instance, nil
		}
	}

	return nil, fmt.Errorf("instance %q not found", name)
}

func listNetworks(ctx context.Context) ([]Network, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		commandCtx,
		"openstack",
		"network",
		"list",
		"-f",
		"json",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if commandCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("openstack command timed out")
		}

		return nil, fmt.Errorf(
			"openstack network list failed: %w: %s",
			err,
			string(output),
		)
	}

	var rawNetworks []openStackNetwork
	if err := json.Unmarshal(output, &rawNetworks); err != nil {
		return nil, fmt.Errorf(
			"failed to decode OpenStack network list output: %w",
			err,
		)
	}

	networks := make([]Network, 0, len(rawNetworks))
	for _, raw := range rawNetworks {
		networks = append(networks, Network{
			ID:   raw.ID,
			Name: raw.Name,
		})
	}

	return networks, nil
}

func main() {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "return-openstack-mcp",
			Version: "0.1.0",
		},
		nil,
	)

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
			instances, err := listInstances(ctx)
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
		) (*mcp.CallToolResult, *Instance, error) {
			instance, err := getInstance(ctx, input.Name)
			if err != nil {
				return nil, nil, err
			}

			return nil, instance, nil
		},
	)

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "list_networks",
			Description: "List OpenStack networks with their IDs and names. This operation is read-only.",
		},
		func(
			ctx context.Context,
			request *mcp.CallToolRequest,
			input ListNetworksInput,
		) (*mcp.CallToolResult, *ListNetworksOutput, error) {
			networks, err := listNetworks(ctx)
			if err != nil {
				return nil, nil, err
			}

			return nil, &ListNetworksOutput{
				Networks: networks,
			}, nil
		},
	)

	if err := server.Run(
		context.Background(),
		&mcp.StdioTransport{},
	); err != nil {
		log.Fatalf("MCP server failed: %v", err)
	}
}
