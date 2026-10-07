// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

func TestBrowserWritesSerialAndCancelable(t *testing.T) {
	var gate browserWriteGate
	var active, maximum atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := gate.acquire(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			defer release()
			n := active.Add(1)
			for {
				old := maximum.Load()
				if n <= old || maximum.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
		}()
	}
	wg.Wait()
	if maximum.Load() != 1 {
		t.Fatalf("simultaneous writes: %d", maximum.Load())
	}
	release, _ := gate.acquire(context.Background())
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := gate.acquire(ctx); err == nil {
		t.Fatal("canceled waiter must not start a browser")
	}
}

func TestTwoPublishContentCallsWaitBeforePreparing(t *testing.T) {
	// Invalid paths allow testing the real service entry without launching a
	// browser, reading login data or creating a real post.
	release, err := globalBrowserWrites.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	finished := make(chan error, 2)
	for range 2 {
		go func() {
			s := &XiaohongshuService{}
			_, err := s.PublishContent(context.Background(), &PublishRequest{Title: "test", Content: "test", Images: []string{"definitely-missing-image.jpg"}})
			finished <- err
		}()
	}
	select {
	case err := <-finished:
		t.Fatalf("publish bypassed global gate: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	release()
	release = nil
	for range 2 {
		select {
		case err := <-finished:
			if err == nil || !strings.Contains(err.Error(), "图片准备失败") {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("publish queue did not release")
		}
	}
}

func TestUncertainPublishReplayBlockedBeforeBrowser(t *testing.T) {
	var guard uncertainPublishGuard
	req := &PublishRequest{Title: "fixture", Content: "fixture", Images: []string{"fixture.jpg"}}
	now := time.Now()
	guard.record(req, context.DeadlineExceeded, now)
	if err := guard.check(req, now); err != nil {
		t.Fatal("pre-click failure must not block a safe retry", err)
	}
	guard.record(req, xiaohongshu.ErrPublishResultUnknown, now)
	if err := guard.check(req, now.Add(time.Minute)); !errors.Is(err, xiaohongshu.ErrPublishResultUnknown) {
		t.Fatal("unknown outcome replay accepted", err)
	}
	if err := guard.check(&PublishRequest{Title: "different fixture"}, now); err != nil {
		t.Fatal("unrelated publish blocked", err)
	}
	if err := guard.check(req, now.Add(16*time.Minute)); err != nil {
		t.Fatal("guard does not expire", err)
	}
}
