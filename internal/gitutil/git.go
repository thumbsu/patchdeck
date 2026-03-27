package gitutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type CommandError struct {
	Command []string
	Stderr  string
	Cause   error
}

func (e *CommandError) Error() string {
	cmd := strings.Join(e.Command, " ")
	if e.Stderr == "" {
		return fmt.Sprintf("%s: %v", cmd, e.Cause)
	}

	return fmt.Sprintf("%s: %s", cmd, strings.TrimSpace(e.Stderr))
}

func RunGit(ctx context.Context, dir string, args ...string) (string, error) {
	return RunGitAllowExitCodes(ctx, dir, nil, args...)
}

func RunGitAllowExitCodes(ctx context.Context, dir string, allowed []int, args ...string) (string, error) {
	cmdArgs := make([]string, 0, len(args)+2)
	if dir != "" {
		cmdArgs = append(cmdArgs, "-C", dir)
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, "git", cmdArgs...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.String(), nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		for _, code := range allowed {
			if exitErr.ExitCode() == code {
				return stdout.String(), nil
			}
		}
	}

	return "", &CommandError{
		Command: append([]string{"git"}, cmdArgs...),
		Stderr:  stderr.String(),
		Cause:   err,
	}
}
