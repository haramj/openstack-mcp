package openstack

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

func GetInstance(ctx context.Context, nameOrID string) (*Instance, error) {
	if strings.TrimSpace(nameOrID) == "" || strings.HasPrefix(nameOrID, "-") {
		return nil, fmt.Errorf("a non-option instance name or ID is required")
	}
	var raw map[string]json.RawMessage
	if err := runOpenStackJSON(ctx, 20*time.Second, &raw, "server", "show", nameOrID, "-f", "json"); err != nil {
		return nil, err
	}
	get := func(keys ...string) string {
		for _, key := range keys {
			if v, ok := raw[key]; ok {
				var text string
				if json.Unmarshal(v, &text) == nil {
					return text
				}
				var object map[string]any
				if json.Unmarshal(v, &object) == nil {
					if name, ok := object["name"].(string); ok {
						return name
					}
					if id, ok := object["id"].(string); ok {
						return id
					}
				}
			}
		}
		return ""
	}
	instance := &Instance{ID: get("id", "ID"), Name: get("name", "Name"), Status: get("status", "Status"), Image: get("image", "Image"), Flavor: get("flavor", "Flavor"), Networks: map[string][]string{}}
	for _, key := range []string{"addresses", "Networks"} {
		if value, ok := raw[key]; ok {
			if json.Unmarshal(value, &instance.Networks) != nil {
				var text string
				if json.Unmarshal(value, &text) != nil {
					return nil, fmt.Errorf("unsupported instance address representation")
				}
				for _, network := range strings.Split(text, ";") {
					name, addresses, ok := strings.Cut(strings.TrimSpace(network), "=")
					if ok {
						for _, address := range strings.Split(addresses, ",") {
							instance.Networks[name] = append(instance.Networks[name], strings.TrimSpace(address))
						}
					}
				}
			}
		}
	}
	if instance.ID == "" {
		return nil, fmt.Errorf("instance response has no ID")
	}
	return instance, nil
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
	Wait           bool     `json:"wait,omitempty"`
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
	if strings.TrimSpace(name) == "" || strings.HasPrefix(name, "-") {
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
	for _, value := range append([]string{opts.Name, opts.Image, opts.Flavor, opts.Network, opts.KeyName}, opts.SecurityGroups...) {
		if strings.HasPrefix(value, "-") {
			return nil, fmt.Errorf("resource identifiers cannot start with an option prefix")
		}
	}

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
	if opts.Wait && opts.NoWait {
		return nil, fmt.Errorf("wait and no_wait cannot both be true")
	}
	if opts.Wait {
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
	if strings.TrimSpace(name) == "" || strings.HasPrefix(name, "-") {
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
