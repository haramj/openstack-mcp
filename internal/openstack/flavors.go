package openstack

import (
	"context"
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
	var rawFlavors []rawFlavor
	if err := runOpenStackJSON(
		ctx,
		20*time.Second,
		&rawFlavors,
		"flavor",
		"list",
		"-f",
		"json",
	); err != nil {
		return nil, err
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
