// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestCleanupFaultsReleaseWriteGateAndInvalidateOldGeneration(t *testing.T) {
	for _, mode := range []string{"hang", "panic", "error", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			var gate browserWriteGate
			var valid atomic.Bool
			valid.Store(true)
			life, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			unblock := make(chan struct{})
			defer close(unblock)
			go func() {
				defer close(done)
				release, err := gate.acquire(context.Background())
				if err != nil {
					t.Error(err)
					return
				}
				defer release()
				defer func() { _ = recover() }()
				defer func() {
					err := boundedBrowserCleanup(20*time.Millisecond, func() { valid.Store(false); cancel() }, func() {
						if valid.Load() {
							t.Error("cleanup before invalidation")
						}
						switch mode {
						case "hang":
							<-unblock
						case "panic":
							panic("synthetic cleanup panic")
						}
					}, func() error { return nil })
					if err != nil {
						t.Error(err)
					}
				}()
				switch mode {
				case "panic":
					panic("synthetic operation panic")
				case "error":
					return
				case "cancel":
					cancel()
					return
				}
			}()
			select {
			case <-done:
			case <-time.After(250 * time.Millisecond):
				t.Fatal("cleanup held write gate")
			}
			ctx, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer stop()
			release, err := gate.acquire(ctx)
			if err != nil {
				t.Fatal("next writer blocked", err)
			}
			defer release()
			if valid.Load() || life.Err() == nil {
				t.Fatal("two valid write generations")
			}
			release() // idempotent release must not drain another generation's token
		})
	}
}

func TestCleanupUnconfirmedTerminationQuarantinesWrites(t *testing.T) {
	var gate browserWriteGate
	unblock := make(chan struct{})
	defer close(unblock)
	err := boundedBrowserCleanup(time.Millisecond, func() {}, func() { <-unblock }, func() error { return errors.New("synthetic kill failure") })
	if err == nil {
		t.Fatal("termination failure swallowed")
	}
	gate.quarantine()
	if _, err := gate.acquire(context.Background()); err == nil {
		t.Fatal("new writer allowed after failed termination")
	}
}
