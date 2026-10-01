//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

// exits records what the signal handler would exit with.
func exits(t *testing.T) chan int {
	t.Helper()
	codes := make(chan int, 4)
	old := exit
	exit = func(code int) { codes <- code }
	t.Cleanup(func() {
		ownSignals()
		exit = old
	})
	return codes
}

// a command that doesn't quit on a signal itself ends its probes and exits
// as the signal would have had it
func TestSignalExits(t *testing.T) {
	if signal.Ignored(syscall.SIGTERM) {
		t.Skip("SIGTERM is ignored here")
	}
	codes := exits(t)
	endProbesOnSignal()
	syscall.Kill(os.Getpid(), syscall.SIGTERM)
	select {
	case code := <-codes:
		if code != 128+int(syscall.SIGTERM) {
			t.Fatalf("exit(%d), want %d", code, 128+int(syscall.SIGTERM))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SIGTERM didn't end magpie")
	}
}

// the app and the TUI take the signals back: the signal reaches their own
// handling (Wails's, bubbletea's) and magpie doesn't exit before them, which
// skipped the app's OnShutdown and left the terminal on the TUI's screen
func TestOwnSignalsHandsThemBack(t *testing.T) {
	if signal.Ignored(syscall.SIGTERM) {
		t.Skip("SIGTERM is ignored here")
	}
	codes := exits(t)
	endProbesOnSignal()
	ownSignals()
	theirs := make(chan os.Signal, 1) // the app's or the TUI's own
	signal.Notify(theirs, syscall.SIGTERM)
	defer signal.Stop(theirs)
	syscall.Kill(os.Getpid(), syscall.SIGTERM)
	select {
	case <-theirs:
	case <-time.After(3 * time.Second):
		t.Fatal("the command's own handling didn't get SIGTERM")
	}
	select {
	case code := <-codes:
		t.Fatalf("magpie exited (%d) before the command could quit its own way", code)
	case <-time.After(300 * time.Millisecond):
	}
}
