package openstack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// CommandError deliberately excludes argv and backend output from every consumer.
type CommandError struct {
	Kind     string
	ExitCode int
	cause    error
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("openstack command %s (exit code %d)", e.Kind, e.ExitCode)
}
func (e *CommandError) Unwrap() error { return e.cause }

const maxCommandOutput = 16 << 20

// Keep draining after the limit so a noisy subprocess cannot block on its pipe.
type boundedOutput struct {
	bytes.Buffer
	overflow bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := maxCommandOutput - b.Len()
	if len(p) > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}
func runOpenStackCommand(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "openstack", args...)
	// Descendants inheriting stdout must not hold a canceled call indefinitely.
	cmd.WaitDelay = 100 * time.Millisecond
	var output boundedOutput
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if err != nil {
		kind := "failed"
		cause := err
		code := -1
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		}
		if commandCtx.Err() != nil {
			cause = commandCtx.Err()
			kind = "canceled"
			if errors.Is(cause, context.DeadlineExceeded) {
				kind = "timed out"
			}
		}
		return nil, &CommandError{Kind: kind, ExitCode: code, cause: cause}
	}
	if output.overflow {
		return nil, &CommandError{Kind: "output limit exceeded", ExitCode: 0}
	}
	return output.Bytes(), nil
}
func runOpenStackJSON[T any](ctx context.Context, timeout time.Duration, target *T, args ...string) error {
	output, err := runOpenStackCommand(ctx, timeout, args...)
	if err != nil {
		return err
	}
	if json.Unmarshal(output, target) != nil {
		return &CommandError{Kind: "invalid JSON response", ExitCode: 0}
	}
	return nil
}
