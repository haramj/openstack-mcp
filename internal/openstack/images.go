package openstack

import (
	"context"
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
	var rawImages []rawImage
	if err := runOpenStackJSON(
		ctx,
		20*time.Second,
		&rawImages,
		"image",
		"list",
		"-f",
		"json",
	); err != nil {
		return nil, err
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
