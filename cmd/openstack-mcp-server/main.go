package main

import (
	"context"
	"flag"
	"github.com/haramj/openstack-mcp-server/internal/remote"
	"log"
	"os/signal"
	"syscall"

	"github.com/haramj/openstack-mcp-server/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	transport := flag.String("transport", "stdio", "stdio or https (requires mTLS)")
	configPath := flag.String("http-config", "", "private remote configuration JSON")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if *transport == "https" {
		config, err := remote.Load(*configPath)
		if err != nil {
			log.Fatal(err)
		}
		if err = remote.Serve(ctx, config); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *transport != "stdio" {
		log.Fatal("transport must be stdio or https")
	}

	feed := tools.NewEventFeed()
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "return-openstack-mcp",
			Version: "0.2.0",
		},
		&mcp.ServerOptions{SubscribeHandler: feed.Subscribe, UnsubscribeHandler: feed.Unsubscribe},
	)
	feed.Register(server)
	go feed.Run(ctx, server)

	tools.RegisterAgentTools(server)
	tools.RegisterInstanceTools(server)
	tools.RegisterNetworkTools(server)
	tools.RegisterImageTools(server)
	tools.RegisterFlavorTools(server)
	tools.RegisterResources(server)
	tools.RegisterPrompts(server)
	tools.RegisterAdvancedTools(server)

	if err := server.Run(
		ctx,
		&mcp.StdioTransport{},
	); err != nil {
		log.Fatalf("MCP server failed: %v", err)
	}
}
