package main

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// ExecResult is the raw outcome of one executed command. LaunchError is set
// only when the command could not be launched at all (missing binary, canceled
// before start); it is deliberately distinct from a real process exit code.
type ExecResult struct {
	Stdout, Stderr []byte
	ExitCode       int // meaningful only when LaunchError == ""
	LaunchError    string
	WaitError      string
	Start, End     time.Time
	Interrupted    bool // killed because our context was canceled (SIGINT/SIGTERM)
}

// Runner executes exact argv vectors without a shell.
type Runner interface {
	Run(argv []string) ExecResult
}

// execRunner runs commands via os/exec, optionally in a fixed directory.
type execRunner struct {
	ctx context.Context
	dir string
}

func (e *execRunner) Run(argv []string) ExecResult {
	res := ExecResult{Start: time.Now().UTC()}
	if err := e.ctx.Err(); err != nil {
		res.End = res.Start
		res.LaunchError = "not launched: " + err.Error()
		res.Interrupted = true
		return res
	}
	cmd := exec.CommandContext(e.ctx, argv[0], argv[1:]...)
	cmd.WaitDelay = time.Second
	if e.dir != "" {
		cmd.Dir = e.dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	res.End = time.Now().UTC()
	res.Stdout, res.Stderr = stdout.Bytes(), stderr.Bytes()
	if cmd.ProcessState == nil {
		if err != nil {
			res.LaunchError = err.Error()
			res.Interrupted = e.ctx.Err() != nil
		}
		return res
	}
	res.ExitCode = cmd.ProcessState.ExitCode()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			res.WaitError = err.Error()
		}
		res.Interrupted = e.ctx.Err() != nil
	}
	return res
}
