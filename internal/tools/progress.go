package tools

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"time"
)

// Heartbeats report elapsed waiting, not a fabricated build completion percentage.
// A caller must opt in with a progress token. Failure to deliver a notification
// must never retry or change an OpenStack operation.
func operationProgress(ctx context.Context, request *mcp.CallToolRequest) func() {
	if request == nil || request.Session == nil || request.Params == nil || request.Params.GetProgressToken() == nil {
		return func() {}
	}
	token := request.Params.GetProgressToken()
	done := make(chan struct{})
	exited := make(chan struct{})
	send := func(progress float64, message string) {
		notifyCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = request.Session.NotifyProgress(notifyCtx, &mcp.ProgressNotificationParams{ProgressToken: token, Progress: progress, Message: message})
	}
	send(0, "OpenStack operation started; waiting for CLI response")
	go func() {
		defer close(exited)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		progress := float64(0)
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				progress += 2
				send(progress, "Still waiting for CLI response; progress measures elapsed seconds, not completion percentage")
			}
		}
	}()
	return func() { close(done); <-exited }
}
