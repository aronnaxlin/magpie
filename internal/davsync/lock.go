package davsync

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/yetone/magpie/internal/settings"
)

// ErrBusy is a wait for another magpie's sync that ran out of time.
var ErrBusy = errors.New("another magpie is syncing: try again in a moment")

// lock keeps what reads or writes sync's files — a sync, Configure, Off,
// Dismiss — to one at a time across magpies, as mu does within one: the
// gateway's syncs every Every, and magpie webdav from a terminal
// meanwhile, would each save sync-state.json over the other's. The system
// holds it on sync.lock and lets it go when the magpie holding it ends,
// however it ends, so none is ever left behind. The file stays: taken
// away, one magpie could hold the lock on it while another took one on a
// new file.
func lock(ctx context.Context) (unlock func(), err error) {
	os.MkdirAll(settings.Dir(), 0o755)
	f, err := os.OpenFile(path("sync.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return func() {}, nil // no lock to be had here: sync as before there was one
	}
	for {
		got, err := tryLock(f)
		if err != nil { // a file system without locks, as an NFS home with no lockd
			f.Close()
			return func() {}, nil
		}
		if got {
			return func() { unlockFile(f); f.Close() }, nil
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ErrBusy
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// locked runs fn holding the lock, after a sync another magpie is in —
// it would save its state over what fn does — waited for as long as a
// sync is given.
func locked(fn func() error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	unlock, err := lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}
