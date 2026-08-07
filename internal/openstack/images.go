package openstack

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type Image struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type rawImage struct {
	ID     string `json:"ID"`
	Name   string `json:"Name"`
	Status string `json:"Status"`
}

func ListImages(ctx context.Context) ([]Image, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		commandCtx,
		"openstack",
		"image",
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
			"openstack image list failed: %w: %s",
			err,
			string(output),
		)
	}

	var rawImages []rawImage
	if err := json.Unmarshal(output, &rawImages); err != nil {
		return nil, fmt.Errorf(
			"failed to decode OpenStack image list output: %w",
			err,
		)
	}

	images := make([]Image, 0, len(rawImages))
	for _, raw := range rawImages {
		images = append(images, Image{
			ID:     raw.ID,
			Name:   raw.Name,
			Status: raw.Status,
		})
	}

	return images, nil
}
