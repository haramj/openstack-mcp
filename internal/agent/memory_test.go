package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestMemoryBoundsReplacementAndClear(t *testing.T) {
	t.Setenv("OPENSTACK_MCP_MEMORY_FILE", filepath.Join(t.TempDir(), "memory.json"))
	for i := 0; i < MaxNotes+5; i++ {
		if _, err := UpdateMemory(MemoryPatch{Note: fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}
	m, err := UpdateMemory(MemoryPatch{ProtectedInstancePatterns: []string{"safe-*"}})
	if err != nil || len(m.Notes) != MaxNotes || m.Notes[0] != "5" || len(m.ProtectedInstancePatterns) != 1 {
		t.Fatalf("%+v %v", m, err)
	}
	if _, err := UpdateMemory(MemoryPatch{Note: strings.Repeat("x", MaxNoteBytes+1)}); err == nil {
		t.Fatal("oversized note accepted")
	}
	m, err = UpdateMemory(MemoryPatch{ProtectedInstancePatterns: []string{}})
	if err != nil || len(m.ProtectedInstancePatterns) != 0 {
		t.Fatalf("clear: %+v %v", m, err)
	}
	m, err = LoadMemory()
	if err != nil || len(m.ProtectedInstancePatterns) != 0 {
		t.Fatal("clear not persisted")
	}
	info, _ := os.Stat(MemoryPath())
	if info.Mode().Perm() != 0600 {
		t.Fatal("memory permissions")
	}
}
func TestConcurrentMemoryUpdates(t *testing.T) {
	t.Setenv("OPENSTACK_MCP_MEMORY_FILE", filepath.Join(t.TempDir(), "memory.json"))
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := UpdateMemory(MemoryPatch{Note: fmt.Sprint(i)}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	m, err := LoadMemory()
	if err != nil || len(m.Notes) != 20 {
		t.Fatalf("lost update: %+v %v", m, err)
	}
}
func TestBlockedPlansHaveNoArguments(t *testing.T) {
	for _, input := range []PlanInput{{Operation: "create"}, {Operation: "get"}, {Operation: "delete", Name: "dev", ConfirmName: "other"}, {Operation: "action", Name: "dev"}, {Operation: "action", Name: "prod-api", Action: "stop"}} {
		plan := PlanInstanceOperation(input, DefaultMemory())
		if !plan.Blocked || len(plan.Arguments) != 0 {
			t.Fatalf("unsafe blocked plan: %+v", plan)
		}
	}
}
