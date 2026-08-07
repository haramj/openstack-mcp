package openstack

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func runOpenStackCommand(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(commandCtx, "openstack", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if commandCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("openstack command timed out")
		}

		return nil, fmt.Errorf(
			"openstack %s failed: %w: %s",
			strings.Join(args, " "),
			err,
			string(output),
		)
	}

	return output, nil
}

func runOpenStackJSON[T any](ctx context.Context, timeout time.Duration, target *T, args ...string) error {
	output, err := runOpenStackCommand(ctx, timeout, args...)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(output, target); err != nil {
		return fmt.Errorf(
			"failed to decode OpenStack %s output: %w",
			strings.Join(args, " "),
			err,
		)
	}

	return nil
}
