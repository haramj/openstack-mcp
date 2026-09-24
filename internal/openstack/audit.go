package openstack

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/haramj/openstack-mcp-server/internal/scope"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type AuditEvent struct {
	Principal     string   `json:"principal,omitempty"`
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
	CoverageWarnings  []string       `json:"coverage_warnings,omitempty"`
	MalformedLines    int            `json:"malformed_lines"`
	ScannedBytes      int64          `json:"scanned_bytes"`
	Truncated         bool           `json:"truncated"`
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

func scopedAuditPath(ctx context.Context) string {
	if c, ok := scope.From(ctx); ok {
		return c.AuditFile
	}
	return auditLogPath()
}
func writeAuditEvent(event AuditEvent) { writeAuditEventContext(context.Background(), event) }
func writeAuditEventContext(ctx context.Context, event AuditEvent) {
	if c, ok := scope.From(ctx); ok {
		event.Principal = c.Principal
	}
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Source == "" {
		event.Source = "openstack-mcp"
	}

	path := scopedAuditPath(ctx)
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
	RecordRejectedAuditContext(context.Background(), operation, target, destructive, message)
}
func RecordRejectedAuditContext(ctx context.Context, operation string, target string, destructive bool, message string) {
	writeAuditEventContext(ctx, AuditEvent{
		Operation:   operation,
		Target:      target,
		Destructive: destructive,
		Status:      "rejected",
		Error:       message,
	})
}

func SummarizeAgentActivity(options ActivitySummaryOptions) (*ActivitySummary, error) {
	return SummarizeAgentActivityContext(context.Background(), options)
}
func SummarizeAgentActivityContext(ctx context.Context, options ActivitySummaryOptions) (*ActivitySummary, error) {
	now := time.Now().UTC()
	sinceHours := options.SinceHours
	if sinceHours <= 0 {
		sinceHours = 12
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}

	if sinceHours > 24*365 {
		sinceHours = 24 * 365
	}
	if limit > 1000 {
		limit = 1000
	}
	since := now.Add(-time.Duration(sinceHours) * time.Hour)
	summary := &ActivitySummary{
		Since:           since.Format(time.RFC3339Nano),
		Until:           now.Format(time.RFC3339Nano),
		StatusCounts:    map[string]int{},
		OperationCounts: map[string]int{},
		AuditLogPath:    scopedAuditPath(ctx),
	}

	events, coverage, err := readAuditWindowContext(ctx, since, now)
	summary.MalformedLines = coverage.Malformed
	summary.ScannedBytes = coverage.Bytes
	summary.Truncated = coverage.Truncated
	if coverage.Truncated {
		summary.CoverageWarnings = append(summary.CoverageWarnings, "audit scan limited to newest 8 MiB; totals cover only observed records")
	}
	if coverage.Malformed > 0 {
		summary.CoverageWarnings = append(summary.CoverageWarnings, "malformed or oversized audit records were skipped")
	}
	if err != nil {
		if os.IsNotExist(err) {
			summary.Recommendations = append(summary.Recommendations, "no audit log exists yet; run an administrator MCP tool to start recording activity")
			return summary, nil
		}

		return nil, err
	}

	sort.SliceStable(events, func(i, j int) bool {
		a, _ := time.Parse(time.RFC3339Nano, events[i].Timestamp)
		b, _ := time.Parse(time.RFC3339Nano, events[j].Timestamp)
		return a.Before(b)
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

	if len(summary.NeedsAttention) > limit {
		summary.NeedsAttention = summary.NeedsAttention[len(summary.NeedsAttention)-limit:]
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

const maxAuditScan = 8 << 20
const maxAuditLine = 1 << 20

type auditCoverage struct {
	Bytes     int64
	Truncated bool
	Malformed int
}

var auditCache struct {
	sync.Mutex
	path     string
	info     os.FileInfo
	events   []AuditEvent
	coverage auditCoverage
}

func readAuditEventsSince(since time.Time) ([]AuditEvent, error) {
	events, _, err := readAuditWindow(since, time.Now().UTC())
	return events, err
}

// A bounded tail avoids unbounded historical scans. It never assumes timestamps
// are monotonic: concurrent commands and imported records can be out of order.
func readAuditWindow(since, until time.Time) ([]AuditEvent, auditCoverage, error) {
	return readAuditWindowContext(context.Background(), since, until)
}
func readAuditWindowContext(ctx context.Context, since, until time.Time) ([]AuditEvent, auditCoverage, error) {
	auditCache.Lock()
	defer auditCache.Unlock()
	path := scopedAuditPath(ctx)
	file, err := os.Open(path)
	if err != nil {
		return nil, auditCoverage{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, auditCoverage{}, err
	}
	if auditCache.path != path || auditCache.info == nil || !os.SameFile(info, auditCache.info) || info.Size() != auditCache.info.Size() || !info.ModTime().Equal(auditCache.info.ModTime()) {
		coverage := auditCoverage{}
		offset := int64(0)
		if info.Size() > maxAuditScan {
			offset = info.Size() - maxAuditScan
			coverage.Truncated = true
		}
		// Read one extra byte to decide whether the tail starts on a complete line.
		start := offset
		if start > 0 {
			start--
		}
		if _, err = file.Seek(start, io.SeekStart); err != nil {
			return nil, coverage, err
		}
		data, err := io.ReadAll(io.LimitReader(file, maxAuditScan+1))
		if err != nil {
			return nil, coverage, err
		}
		coverage.Bytes = int64(len(data))
		if offset > 0 {
			if data[0] == '\n' {
				data = data[1:]
			} else if i := bytes.IndexByte(data, '\n'); i >= 0 {
				data = data[i+1:]
			} else {
				data = nil
			}
		}
		events := []AuditEvent{}
		for _, line := range bytes.Split(data, []byte{'\n'}) {
			if len(line) == 0 {
				continue
			}
			var event AuditEvent
			if len(line) > maxAuditLine || json.Unmarshal(line, &event) != nil {
				coverage.Malformed++
				continue
			}
			if _, err := time.Parse(time.RFC3339Nano, event.Timestamp); err != nil {
				coverage.Malformed++
				continue
			}
			// Historical versions persisted CLI output here. Never redistribute it.
			if event.Error != "" {
				event.Error = "operation failed or rejected; backend details omitted"
			}
			events = append(events, event)
		}
		auditCache.path = path
		auditCache.info = info
		auditCache.events = events
		auditCache.coverage = coverage
	}
	events := []AuditEvent{}
	for _, event := range auditCache.events {
		at, _ := time.Parse(time.RFC3339Nano, event.Timestamp)
		if !at.Before(since) && !at.After(until) {
			events = append(events, event)
		}
	}
	return events, auditCache.coverage, nil
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
		event.Error = "openstack command failed"
	}
	writeAuditEventContext(ctx, event)

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

// RecordMCPAudit records method identity/outcome, never request arguments or responses.
func RecordMCPAudit(ctx context.Context, method, status string) {
	writeAuditEventContext(ctx, AuditEvent{Operation: method, Status: status})
}
