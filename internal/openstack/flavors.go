package openstack

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type Flavor struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	RAM   int    `json:"ram"`
	Disk  int    `json:"disk"`
	VCPUs int    `json:"vcpus"`
}

type rawFlavor struct {
	ID    string `json:"ID"`
	Name  string `json:"Name"`
	RAM   int    `json:"RAM"`
	Disk  int    `json:"Disk"`
	VCPUs int    `json:"VCPUs"`
}

func ListFlavors(ctx context.Context) ([]Flavor, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		commandCtx,
		"openstack",
		"flavor",
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
			"openstack flavor list failed: %w: %s",
			err,
			string(output),
		)
	}

	var rawFlavors []rawFlavor
	if err := json.Unmarshal(output, &rawFlavors); err != nil {
		return nil, fmt.Errorf(
			"failed to decode OpenStack flavor list output: %w",
			err,
		)
	}

	flavors := make([]Flavor, 0, len(rawFlavors))
	for _, raw := range rawFlavors {
		flavors = append(flavors, Flavor{
			ID:    raw.ID,
			Name:  raw.Name,
			RAM:   raw.RAM,
			Disk:  raw.Disk,
			VCPUs: raw.VCPUs,
		})
	}

	return flavors, nil
}
