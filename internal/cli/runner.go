package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// CommandRunner executes host commands. The interface exists so the installer
// can be exercised in tests and in dry-run mode without touching the host.
type CommandRunner interface {
	// Run executes a command and returns its combined output.
	Run(ctx context.Context, name string, args ...string) (string, error)
	// RunWithInput executes a command with data on standard input.
	RunWithInput(ctx context.Context, stdin string, name string, args ...string) (string, error)
	// WriteFile writes a file on the host, creating parent directories.
	WriteFile(path string, content string, mode os.FileMode) error
	// RemoveAll deletes a path on the host.
	RemoveAll(path string) error
}

// ExecRunner runs commands for real.
type ExecRunner struct {
	// Timeout bounds a single command. Zero means no timeout.
	Timeout time.Duration
}

// Run executes a command and returns its combined output.
func (r *ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	return r.RunWithInput(ctx, "", name, args...)
}

// RunWithInput executes a command with data on standard input.
func (r *ExecRunner) RunWithInput(ctx context.Context, stdin string, name string, args ...string) (string, error) {
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	cmd.Stderr = &buffer
	err := cmd.Run()
	output := buffer.String()
	if err != nil {
		return output, fmt.Errorf("command %q failed: %w: %s", commandLine(name, args), err, strings.TrimSpace(output))
	}
	return output, nil
}

// WriteFile writes a file on the host, creating parent directories.
func (r *ExecRunner) WriteFile(path string, content string, mode os.FileMode) error {
	if err := os.MkdirAll(parentDir(path), 0o755); err != nil {
		return fmt.Errorf("directory for %q could not be created: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return fmt.Errorf("file %q could not be written: %w", path, err)
	}
	return nil
}

// RemoveAll deletes a path on the host.
func (r *ExecRunner) RemoveAll(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("path %q could not be removed: %w", path, err)
	}
	return nil
}

// DryRunner prints what would happen instead of changing the host.
type DryRunner struct {
	Printer *Printer
}

// Run prints the command.
func (r *DryRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	r.Printer.DryRun(commandLine(name, args))
	return "", nil
}

// RunWithInput prints the command and the size of its input.
func (r *DryRunner) RunWithInput(_ context.Context, stdin string, name string, args ...string) (string, error) {
	if stdin == "" {
		r.Printer.DryRun(commandLine(name, args))
	} else {
		r.Printer.DryRun(fmt.Sprintf("%s  (with %d bytes on stdin)", commandLine(name, args), len(stdin)))
	}
	return "", nil
}

// WriteFile prints the file that would be written.
func (r *DryRunner) WriteFile(path string, content string, mode os.FileMode) error {
	r.Printer.DryRun(fmt.Sprintf("write %s (%d bytes, mode %04o)", path, len(content), mode))
	return nil
}

// RemoveAll prints the path that would be removed.
func (r *DryRunner) RemoveAll(path string) error {
	r.Printer.DryRun(fmt.Sprintf("remove %s", path))
	return nil
}

// commandLine renders a command for human readable output.
func commandLine(name string, args []string) string {
	if len(args) == 0 {
		return name
	}
	return name + " " + strings.Join(args, " ")
}

// parentDir returns the directory part of a path.
func parentDir(path string) string {
	index := strings.LastIndex(path, "/")
	if index <= 0 {
		return "/"
	}
	return path[:index]
}

// binaryAvailable reports whether an executable is on PATH.
func binaryAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
