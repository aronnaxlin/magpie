//go:build !windows

package proc

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// probeScript is a CLI that is a script starting another program and
// waiting on it, as a version manager's wrapper does; it writes down the
// pid of what it started.
func probeScript(t *testing.T) (sh, pidFile string) {
	dir := t.TempDir()
	sh, pidFile = filepath.Join(dir, "cli"), filepath.Join(dir, "child.pid")
	os.WriteFile(sh, []byte("#!/bin/sh\nsleep 30 &\necho $! > "+pidFile+"\nwait\n"), 0o755)
	old := waitDelay
	waitDelay = 200 * time.Millisecond
	t.Cleanup(func() { waitDelay = old })
	return sh, pidFile
}

func childPid(t *testing.T, pidFile string) int {
	t.Helper()
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if b, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
				return pid
			}
		}
	}
	t.Fatal("the script started nothing")
	return 0
}

// a probe that runs out of time ends with what it started: killing the
// script alone left its child running
func TestProbeTimeoutEndsWhatItStarted(t *testing.T) {
	sh, pidFile := probeScript(t)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	ProbeContext(ctx, sh).Output()
	if pid := childPid(t, pidFile); !gone(pid) {
		t.Fatalf("what the probe started (pid %d) outlived its timeout", pid)
	}
}

// a probe still asking when magpie exits is ended with it, rather than
// left to init
func TestEndProbes(t *testing.T) {
	sh, pidFile := probeScript(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := ProbeContext(ctx, sh)
	done := make(chan struct{})
	go func() {
		cmd.Output()
		close(done)
	}()
	pid := childPid(t, pidFile)
	start := time.Now()
	EndProbes()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the probe went on after EndProbes")
	}
	if !gone(pid) {
		t.Fatalf("what the probe started (pid %d) outlived EndProbes", pid)
	}
	if d := time.Since(start); d > endWait+time.Second {
		t.Fatalf("EndProbes took %v", d)
	}
}

// a probe that answered is forgotten once its context ends, so EndProbes
// doesn't wait on it
func TestProbeForgotten(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	if _, err := ProbeContext(ctx, "true").Output(); err != nil {
		t.Fatal(err)
	}
	cancel()
	for end := time.Now().Add(time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		probes.Lock()
		n := len(probes.m)
		probes.Unlock()
		if n == 0 {
			return
		}
	}
	t.Fatal("a probe that answered is still listed")
}
