package davsync

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestLockHolder is the other magpie of TestLock: it takes the lock, says
// so, and holds it until it is killed.
func TestLockHolder(t *testing.T) {
	if os.Getenv("MAGPIE_LOCK_HOLDER") == "" {
		t.Skip("run by TestLock")
	}
	if _, err := lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	fmt.Println("locked")
	time.Sleep(time.Minute)
}

func TestLock(t *testing.T) {
	newComputer(t).use(t)
	ctx := context.Background()

	// another magpie holds it: this one waits, and gives up when told to
	holder := exec.Command(os.Args[0], "-test.run=^TestLockHolder$")
	holder.Env = append(os.Environ(), "MAGPIE_LOCK_HOLDER=1")
	out, _ := holder.StdoutPipe()
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	defer holder.Process.Kill()
	if line, _ := bufio.NewReader(out).ReadString('\n'); line != "locked\n" {
		t.Fatalf("the other magpie: %q", line)
	}
	short, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	if _, err := lock(short); err == nil || !strings.Contains(err.Error(), "another magpie") {
		t.Fatalf("taken while another magpie held it: %v", err)
	}

	// it ends without letting go, as one stopped mid-sync does: the system
	// lets go for it
	holder.Process.Kill()
	holder.Wait()
	short, cancel = context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	unlock, err := lock(short)
	if err != nil {
		t.Fatalf("after the other magpie was killed: %v", err)
	}

	// Off waits for a sync in progress, and goes on once it is done
	done := make(chan error)
	go func() { done <- locked(func() error { return nil }) }()
	select {
	case err := <-done:
		t.Fatalf("ran while the lock was held: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
