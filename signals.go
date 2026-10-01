package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/yetone/magpie/internal/proc"
)

// endProbesOnSignal: the probes magpie asks a CLI with lead a session of
// their own (proc.ProbeContext), so Ctrl+C in the terminal, the terminal
// closing or a service manager's SIGTERM reach magpie and not them, and
// magpie ending on the signal left them to init. They are ended first, and
// magpie exits as the signal would have had it. A command that waits on
// Ctrl+C itself (interruptContext) is left to finish its own way, its probes
// ended all the same; the app and the TUI, which quit on a signal their own
// way, take the signals back (ownSignals). A signal magpie was started
// ignoring (nohup, a background job) stays ignored: asking for it would undo
// that.
func endProbesOnSignal() {
	var want []os.Signal
	for _, s := range []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP} {
		if !signal.Ignored(s) {
			want = append(want, s)
		}
	}
	if len(want) == 0 {
		return
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, want...)
	onSignal = sigs
	go func() {
		for s := range sigs {
			proc.EndProbes()
			if s == os.Interrupt && interrupts.Load() > 0 {
				continue
			}
			code := 1
			if n, ok := s.(syscall.Signal); ok {
				code = 128 + int(n) // as a shell reports a process the signal ended
			}
			exit(code)
		}
	}()
}

// onSignal is endProbesOnSignal's channel, while it has the signals.
var onSignal chan os.Signal

// exit is os.Exit; a var so tests can see what a signal would do.
var exit = os.Exit

// ownSignals hands the signals back, for a command that quits on them its
// own way: Wails ends the app through OnShutdown (which ends the probes, and
// installs a downloaded update), bubbletea leaves the TUI's screen as it
// found it, and each comes back to main, where the probes are ended too.
// Exiting first skipped all of that.
func ownSignals() {
	if onSignal != nil {
		signal.Stop(onSignal)
		onSignal = nil
	}
}

// interrupts counts the commands ending themselves on Ctrl+C.
var interrupts atomic.Int32

// interruptContext is signal.NotifyContext(…, os.Interrupt), for a command
// that ends itself on Ctrl+C rather than being ended by it.
func interruptContext() (context.Context, context.CancelFunc) {
	interrupts.Add(1)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	var once sync.Once
	return ctx, func() {
		stop()
		once.Do(func() { interrupts.Add(-1) })
	}
}
