package openstack

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
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

type ActivitySummaryOptions struct {
	SinceHours int `json:"since_hours"`
	Limit      int `json:"limit"`
}

type ActivitySummary struct {
	Since             string         `json:"since"`
	Until             string         `json:"until"`
	TotalEvents       int            `json:"total_events"`
	StatusCounts      map[string]int `json:"status_counts"`
	OperationCounts   map[string]int `json:"operation_counts"`
	DestructiveEvents int            `json:"destructive_events"`
	RecentEvents      []AuditEvent   `json:"recent_events"`
	NeedsAttention    []AuditEvent   `json:"needs_attention"`
	Recommendations   []string       `json:"recommendations"`
	AuditLogPath      string         `json:"audit_log_path"`
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

func AuditLogPath() string {
	return auditLogPath()
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

func SummarizeAgentActivity(options ActivitySummaryOptions) (*ActivitySummary, error) {
	now := time.Now().UTC()
	sinceHours := options.SinceHours
	if sinceHours <= 0 {
		sinceHours = 12
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}

	since := now.Add(-time.Duration(sinceHours) * time.Hour)
	summary := &ActivitySummary{
		Since:           since.Format(time.RFC3339Nano),
		Until:           now.Format(time.RFC3339Nano),
		StatusCounts:    map[string]int{},
		OperationCounts: map[string]int{},
		AuditLogPath:    auditLogPath(),
	}

	events, err := readAuditEventsSince(since)
	if err != nil {
		if os.IsNotExist(err) {
			summary.Recommendations = append(summary.Recommendations, "no audit log exists yet; run an administrator MCP tool to start recording activity")
			return summary, nil
		}

		return nil, err
	}

	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp < events[j].Timestamp
	})

	for _, event := range events {
		summary.TotalEvents++
		summary.StatusCounts[event.Status]++
		summary.OperationCounts[event.Operation]++
		if event.Destructive {
			summary.DestructiveEvents++
		}
		if event.Status == "failed" || event.Status == "rejected" {
			summary.NeedsAttention = append(summary.NeedsAttention, event)
		}
	}

	if len(events) > limit {
		summary.RecentEvents = append(summary.RecentEvents, events[len(events)-limit:]...)
	} else {
		summary.RecentEvents = append(summary.RecentEvents, events...)
	}

	if summary.TotalEvents == 0 {
		summary.Recommendations = append(summary.Recommendations, "no administrator MCP activity was recorded in this window")
		return summary, nil
	}
	if len(summary.NeedsAttention) > 0 {
		summary.Recommendations = append(summary.Recommendations, "review failed or rejected administrator operations before retrying")
	}
	if summary.DestructiveEvents > 0 {
		summary.Recommendations = append(summary.Recommendations, "verify destructive operations against current OpenStack state")
	}
	if summary.StatusCounts["success"] == summary.TotalEvents {
		summary.Recommendations = append(summary.Recommendations, "all recorded administrator operations in this window succeeded")
	}

	return summary, nil
}

func readAuditEventsSince(since time.Time) ([]AuditEvent, error) {
	file, err := os.Open(auditLogPath())
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var events []AuditEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		timestamp, err := time.Parse(time.RFC3339Nano, event.Timestamp)
		if err != nil {
			continue
		}
		if timestamp.Before(since) {
			continue
		}

		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return events, nil
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
