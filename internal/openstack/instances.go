package openstack

import (
	"context"
	"fmt"
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
	var rawInstances []rawInstance
	if err := runOpenStackJSON(
		ctx,
		20*time.Second,
		&rawInstances,
		"server",
		"list",
		"-f",
		"json",
	); err != nil {
		return nil, err
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

type InstanceAction string

const (
	InstanceActionStart    InstanceAction = "start"
	InstanceActionStop     InstanceAction = "stop"
	InstanceActionReboot   InstanceAction = "reboot"
	InstanceActionPause    InstanceAction = "pause"
	InstanceActionUnpause  InstanceAction = "unpause"
	InstanceActionSuspend  InstanceAction = "suspend"
	InstanceActionResume   InstanceAction = "resume"
	InstanceActionShelve   InstanceAction = "shelve"
	InstanceActionUnshelve InstanceAction = "unshelve"
	InstanceActionLock     InstanceAction = "lock"
	InstanceActionUnlock   InstanceAction = "unlock"
)

type InstanceActionResult struct {
	Name       string `json:"name"`
	Action     string `json:"action"`
	RebootType string `json:"reboot_type,omitempty"`
	Message    string `json:"message"`
}

type CreateInstanceOptions struct {
	Name           string   `json:"name"`
	Image          string   `json:"image"`
	Flavor         string   `json:"flavor"`
	Network        string   `json:"network"`
	KeyName        string   `json:"key_name,omitempty"`
	SecurityGroups []string `json:"security_groups,omitempty"`
	NoWait         bool     `json:"no_wait"`
}

type CreatedInstance struct {
	ID     string         `json:"id,omitempty"`
	Name   string         `json:"name"`
	Status string         `json:"status,omitempty"`
	Raw    map[string]any `json:"raw"`
}

type DeleteInstanceResult struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

func RunInstanceAction(ctx context.Context, name string, action InstanceAction, rebootType string) (*InstanceActionResult, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	args := []string{"server"}
	switch action {
	case InstanceActionStart:
		args = append(args, "start", name)
	case InstanceActionStop:
		args = append(args, "stop", name)
	case InstanceActionReboot:
		switch rebootType {
		case "", "soft":
			args = append(args, "reboot", "--soft", name)
			rebootType = "soft"
		case "hard":
			args = append(args, "reboot", "--hard", name)
		default:
			return nil, fmt.Errorf("unsupported reboot_type %q", rebootType)
		}
	case InstanceActionPause:
		args = append(args, "pause", name)
	case InstanceActionUnpause:
		args = append(args, "unpause", name)
	case InstanceActionSuspend:
		args = append(args, "suspend", name)
	case InstanceActionResume:
		args = append(args, "resume", name)
	case InstanceActionShelve:
		args = append(args, "shelve", name)
	case InstanceActionUnshelve:
		args = append(args, "unshelve", name)
	case InstanceActionLock:
		args = append(args, "lock", name)
	case InstanceActionUnlock:
		args = append(args, "unlock", name)
	default:
		return nil, fmt.Errorf("unsupported action %q", action)
	}

	if _, err := runAuditedOpenStackCommand(
		ctx,
		60*time.Second,
		"admin_instance_action:"+string(action),
		name,
		true,
		args...,
	); err != nil {
		return nil, err
	}

	return &InstanceActionResult{
		Name:       name,
		Action:     string(action),
		RebootType: rebootType,
		Message:    fmt.Sprintf("instance %q action %q requested", name, action),
	}, nil
}

func CreateInstance(ctx context.Context, opts CreateInstanceOptions) (*CreatedInstance, error) {
	if opts.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if opts.Image == "" {
		return nil, fmt.Errorf("image is required")
	}
	if opts.Flavor == "" {
		return nil, fmt.Errorf("flavor is required")
	}
	if opts.Network == "" {
		return nil, fmt.Errorf("network is required")
	}

	args := []string{
		"server",
		"create",
		"--image",
		opts.Image,
		"--flavor",
		opts.Flavor,
		"--network",
		opts.Network,
		"-f",
		"json",
	}
	if !opts.NoWait {
		args = append(args, "--wait")
	}
	if opts.KeyName != "" {
		args = append(args, "--key-name", opts.KeyName)
	}
	for _, securityGroup := range opts.SecurityGroups {
		if securityGroup != "" {
			args = append(args, "--security-group", securityGroup)
		}
	}
	args = append(args, opts.Name)

	raw := map[string]any{}
	if err := runAuditedOpenStackJSON(
		ctx,
		5*time.Minute,
		&raw,
		"create_instance",
		opts.Name,
		false,
		args...,
	); err != nil {
		return nil, err
	}

	created := &CreatedInstance{
		Name: opts.Name,
		Raw:  raw,
	}
	if id, ok := raw["id"].(string); ok {
		created.ID = id
	} else if id, ok := raw["ID"].(string); ok {
		created.ID = id
	}
	if status, ok := raw["status"].(string); ok {
		created.Status = status
	} else if status, ok := raw["Status"].(string); ok {
		created.Status = status
	}

	return created, nil
}

func DeleteInstance(ctx context.Context, name string) (*DeleteInstanceResult, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	if _, err := runAuditedOpenStackCommand(
		ctx,
		60*time.Second,
		"delete_instance",
		name,
		true,
		"server",
		"delete",
		name,
	); err != nil {
		return nil, err
	}

	return &DeleteInstanceResult{
		Name:    name,
		Message: fmt.Sprintf("instance %q delete requested", name),
	}, nil
}
