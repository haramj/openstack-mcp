package openstack

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type Instance struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Status   string              `json:"status"`
	Networks map[string][]string `json:"networks"`
	Image    string              `json:"image"`
	Flavor   string              `json:"flavor"`
}

type rawInstance struct {
	ID       string              `json:"ID"`
	Name     string              `json:"Name"`
	Status   string              `json:"Status"`
	Networks map[string][]string `json:"Networks"`
	Image    string              `json:"Image"`
	Flavor   string              `json:"Flavor"`
}

func ListInstances(ctx context.Context) ([]Instance, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		commandCtx,
		"openstack",
		"server",
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
			"openstack server list failed: %w: %s",
			err,
			string(output),
		)
	}

	var rawInstances []rawInstance
	if err := json.Unmarshal(output, &rawInstances); err != nil {
		return nil, fmt.Errorf(
			"failed to decode OpenStack server list output: %w",
			err,
		)
	}

	instances := make([]Instance, 0, len(rawInstances))
	for _, raw := range rawInstances {
		instances = append(instances, Instance{
			ID:       raw.ID,
			Name:     raw.Name,
			Status:   raw.Status,
			Networks: raw.Networks,
			Image:    raw.Image,
			Flavor:   raw.Flavor,
		})
	}

	return instances, nil
}

func GetInstance(ctx context.Context, name string) (*Instance, error) {
	instances, err := ListInstances(ctx)
	if err != nil {
		return nil, err
	}

	for _, instance := range instances {
		if instance.Name == name {
			return &instance, nil
		}
	}

	return nil, fmt.Errorf("instance %q not found", name)
}
