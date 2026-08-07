package openstack

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type Network struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type rawNetwork struct {
	ID   string `json:"ID"`
	Name string `json:"Name"`
}

func ListNetworks(ctx context.Context) ([]Network, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		commandCtx,
		"openstack",
		"network",
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
			"openstack network list failed: %w: %s",
			err,
			string(output),
		)
	}

	var rawNetworks []rawNetwork
	if err := json.Unmarshal(output, &rawNetworks); err != nil {
		return nil, fmt.Errorf(
			"failed to decode OpenStack network list output: %w",
			err,
		)
	}

	networks := make([]Network, 0, len(rawNetworks))
	for _, raw := range rawNetworks {
		networks = append(networks, Network{
			ID:   raw.ID,
			Name: raw.Name,
		})
	}

	return networks, nil
}
