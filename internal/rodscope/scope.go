// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
// Package rodscope intersects contexts at the CDP boundary. Rod v0.116.2
// shallow Page/Element copies retain parent Mouse, Keyboard and Browser pointers.
package rodscope

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
)

type Transport struct {
	delegate *rod.Browser
	mu       sync.Mutex
	next     uint64
	scopes   map[uint64]context.Context
	invalid  bool
	life     context.Context
	cancel   context.CancelFunc
}

var registry sync.Map // *rod.Browser -> *Transport; removed on managed close
var installMu sync.Mutex

// Install is called before the browser is shared. Connect has synchronously
// captured the original Event channel, so replacing Client preserves events.
// Copy only for delegation; do not Connect the installed client again.
func Install(b *rod.Browser) *Transport {
	installMu.Lock()
	defer installMu.Unlock()
	if existing, ok := registry.Load(b); ok {
		return existing.(*Transport)
	}
	copy := *b
	life, cancel := context.WithCancel(context.Background())
	t := &Transport{delegate: &copy, scopes: make(map[uint64]context.Context), life: life, cancel: cancel}
	b.Client(t)
	registry.Store(b, t)
	return t
}

func (t *Transport) Event() <-chan *cdp.Event {
	panic("rodscope must be installed after Connect; reconnect is unsupported")
}

// Bind is lexical, supports nested steps and never makes a stale scope valid.
func Bind(page *rod.Page, ctx context.Context) func() {
	t := Install(page.Browser())
	t.mu.Lock()
	t.next++
	id := t.next
	t.scopes[id] = ctx
	t.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { t.mu.Lock(); delete(t.scopes, id); t.mu.Unlock() }) }
}

func (t *Transport) Invalidate() {
	t.mu.Lock()
	t.invalid = true
	t.mu.Unlock()
	t.cancel()
}

func (t *Transport) Forget(b *rod.Browser) { registry.Delete(b) }

func (t *Transport) Call(ctx context.Context, sessionID, method string, params interface{}) ([]byte, error) {
	closing := method == "Browser.close" || method == "Target.closeTarget"
	t.mu.Lock()
	if t.invalid && !closing {
		t.mu.Unlock()
		return nil, context.Canceled
	}
	parents := []context.Context{ctx}
	if !closing {
		parents = append(parents, t.life)
		for _, parent := range t.scopes {
			parents = append(parents, parent)
		}
	}
	t.mu.Unlock()
	budget := 30 * time.Second
	if closing {
		budget = 2 * time.Second
	}
	deadline := time.Now().Add(budget)
	for _, parent := range parents {
		if err := parent.Err(); err != nil {
			return nil, err
		}
		if d, ok := parent.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
	}
	callCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	stops := make([]func() bool, 0, len(parents))
	for _, parent := range parents {
		stops = append(stops, context.AfterFunc(parent, cancel))
	}
	defer func() {
		for _, stop := range stops {
			stop()
		}
	}()
	result, err := t.delegate.Call(callCtx, sessionID, method, params)
	if err == nil && callCtx.Err() != nil {
		err = callCtx.Err()
	}
	if errors.Is(err, context.Canceled) {
		for _, parent := range parents {
			if parent.Err() != nil {
				return nil, parent.Err()
			}
		}
	}
	return result, err
}
