package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/haramj/openstack-mcp-server/internal/scope"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Memory struct {
	DefaultImage              string   `json:"default_image"`
	DefaultFlavor             string   `json:"default_flavor"`
	DefaultNetwork            string   `json:"default_network"`
	DeleteRequiresConfirmName bool     `json:"delete_requires_confirm_name"`
	ProtectedInstancePatterns []string `json:"protected_instance_patterns"`
	Notes                     []string `json:"notes,omitempty"`
	UpdatedAt                 string   `json:"updated_at,omitempty"`
}

type MemoryPatch struct {
	DefaultImage              string   `json:"default_image,omitempty"`
	DefaultFlavor             string   `json:"default_flavor,omitempty"`
	DefaultNetwork            string   `json:"default_network,omitempty"`
	DeleteRequiresConfirmName *bool    `json:"delete_requires_confirm_name,omitempty"`
	ProtectedInstancePatterns []string `json:"protected_instance_patterns,omitempty"`
	Note                      string   `json:"note,omitempty"`
}

func DefaultMemory() Memory {
	return Memory{
		DefaultImage:              "RCP Ubuntu 22.04",
		DefaultFlavor:             "m1.small",
		DefaultNetwork:            "demo-net",
		DeleteRequiresConfirmName: true,
		ProtectedInstancePatterns: []string{"prod-*", "*-prod", "production-*"},
	}
}

func MemoryPath() string {
	if path := os.Getenv("OPENSTACK_MCP_MEMORY_FILE"); path != "" {
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "openstack-mcp-memory.json"
	}

	return filepath.Join(home, ".config", "openstack-mcp", "memory.json")
}

const MaxNotes = 100
const MaxNoteBytes = 8192
const maxMemoryBytes = 2 << 20

var memoryMu sync.Mutex

func memoryPath(ctx context.Context) string {
	if c, ok := scope.From(ctx); ok {
		return c.MemoryFile
	}
	return MemoryPath()
}
func LoadMemory() (Memory, error) { return LoadMemoryContext(context.Background()) }
func LoadMemoryContext(ctx context.Context) (Memory, error) {
	memoryMu.Lock()
	defer memoryMu.Unlock()
	return loadMemory(memoryPath(ctx))
}
func loadMemory(path string) (Memory, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultMemory(), nil
		}

		return Memory{}, err
	}

	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxMemoryBytes+1))
	if err != nil {
		return Memory{}, err
	}
	if len(data) > maxMemoryBytes {
		return Memory{}, fmt.Errorf("memory exceeds 2 MiB; archive old notes manually")
	}
	memory := DefaultMemory()
	if err := json.Unmarshal(data, &memory); err != nil {
		return Memory{}, err
	}

	return memory, nil
}

func SaveMemory(memory Memory) error {
	memoryMu.Lock()
	defer memoryMu.Unlock()
	return saveMemory(MemoryPath(), memory)
}
func saveMemory(path string, memory Memory) error {
	if err := validateMemory(memory); err != nil {
		return err
	}
	memory.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(memory, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > maxMemoryBytes {
		return fmt.Errorf("memory exceeds 2 MiB")
	}

	file, err := os.CreateTemp(filepath.Dir(path), ".memory-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func UpdateMemory(patch MemoryPatch) (Memory, error) {
	return UpdateMemoryContext(context.Background(), patch)
}
func UpdateMemoryContext(ctx context.Context, patch MemoryPatch) (Memory, error) {
	memoryMu.Lock()
	defer memoryMu.Unlock()
	if len(patch.Note) > MaxNoteBytes {
		return Memory{}, fmt.Errorf("note exceeds %d bytes", MaxNoteBytes)
	}
	memory, err := loadMemory(memoryPath(ctx))
	if err != nil {
		return Memory{}, err
	}

	if patch.DefaultImage != "" {
		memory.DefaultImage = patch.DefaultImage
	}
	if patch.DefaultFlavor != "" {
		memory.DefaultFlavor = patch.DefaultFlavor
	}
	if patch.DefaultNetwork != "" {
		memory.DefaultNetwork = patch.DefaultNetwork
	}
	if patch.DeleteRequiresConfirmName != nil {
		memory.DeleteRequiresConfirmName = *patch.DeleteRequiresConfirmName
	}
	if patch.ProtectedInstancePatterns != nil {
		memory.ProtectedInstancePatterns = append([]string(nil), patch.ProtectedInstancePatterns...)
	}
	if patch.Note != "" {
		memory.Notes = append(memory.Notes, patch.Note)
	}

	if len(memory.Notes) > MaxNotes {
		memory.Notes = memory.Notes[len(memory.Notes)-MaxNotes:]
	}
	memory.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := saveMemory(memoryPath(ctx), memory); err != nil {
		return Memory{}, err
	}

	return memory, nil
}

func validateMemory(m Memory) error {
	if len(m.Notes) > MaxNotes {
		return fmt.Errorf("memory exceeds %d notes", MaxNotes)
	}
	for _, note := range m.Notes {
		if len(note) > MaxNoteBytes {
			return fmt.Errorf("note exceeds %d bytes", MaxNoteBytes)
		}
	}
	if len(m.ProtectedInstancePatterns) > 100 {
		return fmt.Errorf("too many protected patterns")
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(data) > maxMemoryBytes {
		return fmt.Errorf("memory exceeds 2 MiB")
	}
	return nil
}
