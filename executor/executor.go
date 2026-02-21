/*
 * Copyright (c) 2026 RuturajS (ROne). All rights reserved.
 * This code belongs to the author. No modification or republication 
 * is allowed without explicit permission.
 */
package executor

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	// MaxOutputBytes is the maximum number of bytes captured from command output.
	MaxOutputBytes = 4096

	// DefaultTimeout is the default command execution timeout.
	DefaultTimeout = 30 * time.Second
)

// Result holds the output of an executed command.
type Result struct {
	Command  string
	Output   string
	ExitCode int
	Duration time.Duration
	Error    string
}

// Run executes a shell command with a timeout and captures stdout+stderr.
func Run(ctx context.Context, command string, timeout time.Duration) *Result {
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()

	slog.Info("executor: running command", "command", command, "timeout", timeout)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-c", command)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	result := &Result{
		Command:  command,
		Duration: duration,
	}

	// Combine stdout and stderr
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	// Truncate if too long
	if len(output) > MaxOutputBytes {
		output = output[:MaxOutputBytes] + "\n... (output truncated)"
	}

	result.Output = strings.TrimSpace(output)

	if err != nil {
		result.Error = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
		slog.Warn("executor: command failed",
			"command", command,
			"exit_code", result.ExitCode,
			"error", result.Error,
			"duration", duration,
		)
	} else {
		result.ExitCode = 0
		slog.Info("executor: command succeeded",
			"command", command,
			"output_bytes", len(result.Output),
			"duration", duration,
		)
	}

	return result
}

// FormatResult produces a clean text summary of the execution result.
func FormatResult(r *Result) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("$ %s\n", r.Command))
	if r.Output != "" {
		sb.WriteString(r.Output)
		sb.WriteString("\n")
	}
	if r.Error != "" && r.ExitCode != 0 {
		sb.WriteString(fmt.Sprintf("(exit code: %d)\n", r.ExitCode))
	}
	return strings.TrimSpace(sb.String())
}

