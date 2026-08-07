package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func LoadMemory() (Memory, error) {
	path := MemoryPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultMemory(), nil
		}

		return Memory{}, err
	}

	memory := DefaultMemory()
	if err := json.Unmarshal(data, &memory); err != nil {
		return Memory{}, err
	}

	return memory, nil
}

func SaveMemory(memory Memory) error {
	memory.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)

	path := MemoryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(memory, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return os.WriteFile(path, data, 0o600)
}

func UpdateMemory(patch MemoryPatch) (Memory, error) {
	memory, err := LoadMemory()
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
	if len(patch.ProtectedInstancePatterns) > 0 {
		memory.ProtectedInstancePatterns = append([]string(nil), patch.ProtectedInstancePatterns...)
	}
	if patch.Note != "" {
		memory.Notes = append(memory.Notes, patch.Note)
	}

	if err := SaveMemory(memory); err != nil {
		return Memory{}, err
	}

	return memory, nil
}
