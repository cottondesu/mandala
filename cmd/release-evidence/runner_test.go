package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestExecRunnerInterruptedWithInheritedPipes(t *testing.T) {
	if len(os.Args) >= 3 && os.Args[len(os.Args)-2] == "pipe-helper" {
		ready := os.Args[len(os.Args)-1]
		if os.Getenv("RELEASE_EVIDENCE_PIPE_CHILD") == "1" {
			if err := os.WriteFile(ready, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
				os.Exit(2)
			}
			time.Sleep(30 * time.Second)
			return
		}
		child := exec.Command(os.Args[0], "-test.run=^TestExecRunnerInterruptedWithInheritedPipes$", "--", "pipe-helper", ready)
		child.Env = append(os.Environ(), "RELEASE_EVIDENCE_PIPE_CHILD=1")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		fmt.Fprint(os.Stdout, "partial stdout\n")
		fmt.Fprint(os.Stderr, "partial stderr\n")
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		_ = child.Wait()
		return
	}

	ready := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan ExecResult, 1)
	go func() {
		result <- (&execRunner{ctx: ctx}).Run([]string{os.Args[0], "-test.run=^TestExecRunnerInterruptedWithInheritedPipes$", "--", "pipe-helper", ready})
	}()

	var pid int
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(ready); err == nil {
			pid, _ = strconv.Atoi(string(data))
			if pid > 0 {
				break
			}
		}
		select {
		case res := <-result:
			t.Fatalf("helper exited before child was ready: %+v", res)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if pid == 0 {
		t.Fatal("timed out waiting for descendant")
	}
	defer func() {
		if process, err := os.FindProcess(pid); err == nil {
			_ = process.Kill()
		}
	}()
	cancel()
	select {
	case res := <-result:
		if !res.Interrupted || res.LaunchError != "" {
			t.Errorf("expected interrupted process, got %+v", res)
		}
		if !strings.Contains(string(res.Stdout), "partial stdout") || !strings.Contains(string(res.Stderr), "partial stderr") {
			t.Errorf("partial output missing: stdout=%q stderr=%q", res.Stdout, res.Stderr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runner did not return after cancellation")
	}
}

func TestExecRunnerWaitErrorWithInheritedPipes(t *testing.T) {
	if len(os.Args) >= 3 && os.Args[len(os.Args)-2] == "wait-pipe-helper" {
		ready := os.Args[len(os.Args)-1]
		if os.Getenv("RELEASE_EVIDENCE_WAIT_PIPE_CHILD") == "1" {
			if err := os.WriteFile(ready, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
				os.Exit(2)
			}
			time.Sleep(30 * time.Second)
			os.Exit(0)
		}
		child := exec.Command(os.Args[0], "-test.run=^TestExecRunnerWaitErrorWithInheritedPipes$", "--", "wait-pipe-helper", ready)
		child.Env = append(os.Environ(), "RELEASE_EVIDENCE_WAIT_PIPE_CHILD=1")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		fmt.Fprint(os.Stdout, "partial stdout\n")
		fmt.Fprint(os.Stderr, "partial stderr\n")
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(ready); err == nil {
				os.Exit(0)
			}
			time.Sleep(10 * time.Millisecond)
		}
		_ = child.Process.Kill()
		os.Exit(2)
	}

	ready := filepath.Join(t.TempDir(), "child.pid")
	res := (&execRunner{ctx: context.Background()}).Run([]string{os.Args[0], "-test.run=^TestExecRunnerWaitErrorWithInheritedPipes$", "--", "wait-pipe-helper", ready})
	data, err := os.ReadFile(ready)
	if err != nil {
		t.Fatalf("read descendant pid: %v; result: %+v", err, res)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatalf("invalid descendant pid: %v", err)
	}
	defer func() {
		if process, err := os.FindProcess(pid); err == nil {
			_ = process.Kill()
		}
	}()
	if res.ExitCode != 0 || res.WaitError != exec.ErrWaitDelay.Error() || res.LaunchError != "" || res.Interrupted {
		t.Errorf("unexpected result: %+v", res)
	}
	if !strings.Contains(string(res.Stdout), "partial stdout") || !strings.Contains(string(res.Stderr), "partial stderr") {
		t.Errorf("partial output missing: stdout=%q stderr=%q", res.Stdout, res.Stderr)
	}
}
