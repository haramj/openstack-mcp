package openstack

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type AuditEvent struct {
	Timestamp     string   `json:"timestamp"`
	Source        string   `json:"source"`
	Operation     string   `json:"operation"`
	Target        string   `json:"target,omitempty"`
	Destructive   bool     `json:"destructive"`
	OpenStackArgs []string `json:"openstack_args,omitempty"`
	Status        string   `json:"status"`
	Error         string   `json:"error,omitempty"`
	DurationMs    int64    `json:"duration_ms,omitempty"`
}

func auditLogPath() string {
	if path := os.Getenv("OPENSTACK_MCP_AUDIT_LOG"); path != "" {
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "openstack-mcp-audit.jsonl"
	}

	return filepath.Join(home, ".local", "state", "openstack-mcp", "audit.jsonl")
}

func writeAuditEvent(event AuditEvent) {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Source == "" {
		event.Source = "openstack-mcp"
	}

	path := auditLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	encoder := json.NewEncoder(file)
	_ = encoder.Encode(event)
}

func RecordRejectedAudit(operation string, target string, destructive bool, message string) {
	writeAuditEvent(AuditEvent{
		Operation:   operation,
		Target:      target,
		Destructive: destructive,
		Status:      "rejected",
		Error:       message,
	})
}

func runAuditedOpenStackCommand(
	ctx context.Context,
	timeout time.Duration,
	operation string,
	target string,
	destructive bool,
	args ...string,
) ([]byte, error) {
	start := time.Now()
	output, err := runOpenStackCommand(ctx, timeout, args...)

	event := AuditEvent{
		Operation:     operation,
		Target:        target,
		Destructive:   destructive,
		OpenStackArgs: append([]string(nil), args...),
		Status:        "success",
		DurationMs:    time.Since(start).Milliseconds(),
	}
	if err != nil {
		event.Status = "failed"
		event.Error = err.Error()
	}
	writeAuditEvent(event)

	return output, err
}

func runAuditedOpenStackJSON[T any](
	ctx context.Context,
	timeout time.Duration,
	target *T,
	operation string,
	resource string,
	destructive bool,
	args ...string,
) error {
	output, err := runAuditedOpenStackCommand(ctx, timeout, operation, resource, destructive, args...)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(output, target); err != nil {
		return err
	}

	return nil
}
