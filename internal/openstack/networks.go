package openstack

import (
	"context"
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
	var rawNetworks []rawNetwork
	if err := runOpenStackJSON(
		ctx,
		20*time.Second,
		&rawNetworks,
		"network",
		"list",
		"-f",
		"json",
	); err != nil {
		return nil, err
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
