package openstack

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fakeCLI(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "argv")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ARGV_LOG", log)
	t.Setenv("OPENSTACK_MCP_AUDIT_LOG", filepath.Join(dir, "audit.jsonl"))
	if err := os.WriteFile(filepath.Join(dir, "openstack"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ARGV_LOG\"\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	return log
}
func TestFailureNeverLeaksOutput(t *testing.T) {
	fakeCLI(t, "echo LEAKED_TOKEN_123; echo PRIVATE_STDERR >&2; exit 7\n")
	_, err := RunInstanceAction(context.Background(), "my vm", InstanceActionStop, "")
	if err == nil || strings.Contains(err.Error(), "LEAKED") || strings.Contains(err.Error(), "PRIVATE") || strings.Contains(err.Error(), "my vm") {
		t.Fatalf("unsafe error: %v", err)
	}
	var command *CommandError
	if !errors.As(err, &command) || command.ExitCode != 7 {
		t.Fatalf("missing typed exit: %v", err)
	}
	data, _ := os.ReadFile(auditLogPath())
	if strings.Contains(string(data), "LEAKED") || strings.Contains(string(data), "PRIVATE") {
		t.Fatal("audit leak")
	}
	summary, err := SummarizeAgentActivity(ActivitySummaryOptions{})
	if err != nil || summary.TotalEvents != 1 || summary.StatusCounts["failed"] != 1 {
		t.Fatalf("%+v %v", summary, err)
	}
	if !reflect.DeepEqual(summary.RecentEvents[0].OpenStackArgs, []string{"server", "stop", "my vm"}) {
		t.Fatal("argv not preserved")
	}
}
func TestParentContextCancellation(t *testing.T) {
	fakeCLI(t, "exec sleep 10\n")
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)
	start := time.Now()
	_, err := runOpenStackCommand(ctx, 20*time.Second, "network", "list")
	if !errors.Is(err, context.Canceled) || time.Since(start) > 2*time.Second {
		t.Fatalf("cancellation: %v %v", err, time.Since(start))
	}
}
func TestEarlierParentDeadline(t *testing.T) {
	fakeCLI(t, "exec sleep 10\n")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := runOpenStackCommand(ctx, 20*time.Second, "image", "list")
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 2*time.Second {
		t.Fatalf("deadline: %v", err)
	}
}
func TestCreateWaitOptInAndIDs(t *testing.T) {
	for _, wait := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "wait"}[wait], func(t *testing.T) {
			log := fakeCLI(t, "echo '{\"ID\":\"id-123\",\"Status\":\"BUILD\"}'\n")
			got, err := CreateInstance(context.Background(), CreateInstanceOptions{Name: "my vm", Image: "image", Flavor: "small", Network: "net", Wait: wait, SecurityGroups: []string{"default"}})
			if err != nil || got.ID != "id-123" {
				t.Fatalf("%+v %v", got, err)
			}
			data, _ := os.ReadFile(log)
			args := strings.Split(strings.TrimSpace(string(data)), "\n")
			if strings.Contains(string(data), "--wait") != wait || args[len(args)-1] != "my vm" {
				t.Fatalf("argv %v", args)
			}
		})
	}
}
func TestGetInstanceUsesShowAndHandlesNativeFields(t *testing.T) {
	log := fakeCLI(t, `echo '{"id":"abc-123","name":"same-name","status":"ACTIVE","addresses":{"net":["192.0.2.1"]},"image":{"id":"image-id"},"flavor":{"name":"small"}}'`)
	got, err := GetInstance(context.Background(), "abc-123")
	if err != nil || got.ID != "abc-123" || got.Flavor != "small" || got.Networks["net"][0] != "192.0.2.1" {
		t.Fatalf("%+v %v", got, err)
	}
	args, _ := os.ReadFile(log)
	if string(args) != "server\nshow\nabc-123\n-f\njson\n" {
		t.Fatalf("%s", args)
	}
}
func TestGetInstanceAmbiguityIsError(t *testing.T) {
	fakeCLI(t, "echo 'multiple servers found' >&2; exit 1\n")
	if _, err := GetInstance(context.Background(), "duplicate"); err == nil {
		t.Fatal("ambiguity silently accepted")
	}
}
func TestAuditMalformedLargeLegacyAndRotation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "audit")
	t.Setenv("OPENSTACK_MCP_AUDIT_LOG", file)
	event := AuditEvent{Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Operation: "test", Status: "failed", Error: strings.Repeat("s", 70000)}
	row, _ := json.Marshal(event)
	data := append([]byte("broken\n"), row...)
	data = append(data, '\n')
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	summary, err := SummarizeAgentActivity(ActivitySummaryOptions{})
	if err != nil || summary.TotalEvents != 1 || summary.MalformedLines != 1 {
		t.Fatalf("%+v %v", summary, err)
	}
	if strings.Contains(summary.RecentEvents[0].Error, strings.Repeat("s", 10)) {
		t.Fatal("legacy output redistributed")
	}
	if err := os.Rename(file, file+".1"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(file, []byte{}, 0600)
	summary, err = SummarizeAgentActivity(ActivitySummaryOptions{})
	if err != nil || summary.TotalEvents != 0 {
		t.Fatalf("stale cache: %+v %v", summary, err)
	}
}
func TestAuditTailIsBoundedAndExplicit(t *testing.T) {
	file := filepath.Join(t.TempDir(), "audit")
	t.Setenv("OPENSTACK_MCP_AUDIT_LOG", file)
	row, _ := json.Marshal(AuditEvent{Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Status: "success"})
	data := append([]byte(strings.Repeat("x", maxAuditScan+20)+"\n"), row...)
	os.WriteFile(file, data, 0600)
	summary, err := SummarizeAgentActivity(ActivitySummaryOptions{})
	if err != nil || !summary.Truncated || summary.ScannedBytes > maxAuditScan+1 || summary.TotalEvents != 1 || len(summary.CoverageWarnings) == 0 {
		t.Fatalf("%+v %v", summary, err)
	}
}
func TestSuccessfulJSONIgnoresStderr(t *testing.T) {
	fakeCLI(t, "echo 'warning' >&2; echo '[]'\n")
	if _, err := ListInstances(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCreateLowercaseAndConflictingWait(t *testing.T) {
	fakeCLI(t, `echo '{"id":"lower-id","status":"ACTIVE"}'`)
	opts := CreateInstanceOptions{Name: "dev", Image: "i", Flavor: "f", Network: "n", NoWait: true}
	got, err := CreateInstance(context.Background(), opts)
	if err != nil || got.ID != "lower-id" || got.Status != "ACTIVE" {
		t.Fatalf("%+v %v", got, err)
	}
	opts.Wait = true
	if _, err := CreateInstance(context.Background(), opts); err == nil {
		t.Fatal("conflicting flags accepted")
	}
}
func TestAuditOutOfOrderAndCacheRefresh(t *testing.T) {
	file := filepath.Join(t.TempDir(), "audit")
	t.Setenv("OPENSTACK_MCP_AUDIT_LOG", file)
	now := time.Now().UTC()
	for _, at := range []time.Time{now.Add(-time.Minute), now.Add(-24 * time.Hour), now.Add(-2 * time.Minute)} {
		writeAuditEvent(AuditEvent{Timestamp: at.Format(time.RFC3339Nano), Status: "success"})
	}
	first, err := SummarizeAgentActivity(ActivitySummaryOptions{SinceHours: 1})
	if err != nil || first.TotalEvents != 2 {
		t.Fatalf("%+v %v", first, err)
	}
	second, _ := SummarizeAgentActivity(ActivitySummaryOptions{SinceHours: 1})
	if second.TotalEvents != 2 {
		t.Fatal("cache mismatch")
	}
	writeAuditEvent(AuditEvent{Status: "success"})
	third, _ := SummarizeAgentActivity(ActivitySummaryOptions{SinceHours: 1})
	if third.TotalEvents != 3 {
		t.Fatal("append not observed")
	}
	os.WriteFile(file, []byte{}, 0600)
	last, _ := SummarizeAgentActivity(ActivitySummaryOptions{})
	if last.TotalEvents != 0 {
		t.Fatal("truncate not observed")
	}
}
func TestReadCommandsAndActions(t *testing.T) {
	fakeCLI(t, "echo '[]'\n")
	if _, err := ListNetworks(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := ListImages(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := ListFlavors(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, action := range []InstanceAction{InstanceActionStart, InstanceActionStop, InstanceActionReboot, InstanceActionPause, InstanceActionUnpause, InstanceActionSuspend, InstanceActionResume, InstanceActionShelve, InstanceActionUnshelve, InstanceActionLock, InstanceActionUnlock} {
		if _, err := RunInstanceAction(context.Background(), "dev", action, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := RunInstanceAction(context.Background(), "dev", InstanceActionReboot, "hard"); err != nil {
		t.Fatal(err)
	}
	if _, err := RunInstanceAction(context.Background(), "dev", InstanceAction("invalid"), ""); err == nil {
		t.Fatal("invalid action")
	}
	if _, err := DeleteInstance(context.Background(), "dev"); err != nil {
		t.Fatal(err)
	}
}
