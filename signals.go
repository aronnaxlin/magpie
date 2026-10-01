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
// ended all the same. A signal magpie was started ignoring (nohup, a
// background job) stays ignored: asking for it would undo that.
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
			os.Exit(code)
		}
	}()
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
