// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package rodscope

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

type blockingCDP struct {
	events  chan *cdp.Event
	blocked string
	budget  time.Duration
	calls   atomic.Int32
}

func (c *blockingCDP) Event() <-chan *cdp.Event { return c.events }
func (c *blockingCDP) Call(ctx context.Context, _, method string, _ interface{}) ([]byte, error) {
	if method == c.blocked {
		c.calls.Add(1)
		if deadline, ok := ctx.Deadline(); ok {
			c.budget = time.Until(deadline)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if method == "Target.attachToTarget" {
		return []byte(`{"sessionId":"fixture"}`), nil
	}
	if method == "Runtime.evaluate" {
		return []byte(`{"result":{"type":"object","objectId":"window-fixture"}}`), nil
	}
	return []byte(`{}`), nil
}

func fixture(t *testing.T, method string) (*rod.Page, *Transport, *blockingCDP) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	c := &blockingCDP{events: make(chan *cdp.Event), blocked: method}
	b := rod.New().Context(ctx).Client(c).NoDefaultDevice()
	if err := b.Connect(); err != nil {
		t.Fatal(err)
	}
	p, err := b.PageFromTarget("fixture")
	if err != nil {
		t.Fatal(err)
	}
	transport := Install(b)
	t.Cleanup(func() { transport.Invalidate(); transport.Forget(b); cancel(); close(c.events) })
	return p, transport, c
}

func TestCDPShallowCopiesHonor25msStep(t *testing.T) {
	cases := []struct {
		name, method string
		call         func(*rod.Page) error
	}{
		{"mouse", "Input.dispatchMouseEvent", func(p *rod.Page) error { return p.Mouse.MoveTo(proto.Point{X: 1, Y: 1}) }},
		{"keyboard", "Input.dispatchKeyEvent", func(p *rod.Page) error { return p.Keyboard.Press(input.Enter) }},
		{"info", "Target.getTargetInfo", func(p *rod.Page) error { _, err := p.Info(); return err }},
		{"navigate", "Page.navigate", func(p *rod.Page) error { return p.Navigate("https://www.xiaohongshu.com/explore/fixture") }},
		{"element", "Runtime.callFunctionOn", func(p *rod.Page) error { _, err := p.Element("#missing"); return err }},
		{"wait", "Runtime.callFunctionOn", func(p *rod.Page) error { return p.Wait(rod.Eval(`() => false`)) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, _, c := fixture(t, tc.method)
			ctx, cancel := context.WithTimeout(p.GetContext(), 25*time.Millisecond)
			defer cancel()
			end := Bind(p, ctx)
			defer end()
			start := time.Now()
			err := tc.call(p.Context(ctx))
			elapsed := time.Since(start)
			if !errors.Is(err, context.DeadlineExceeded) || elapsed > 100*time.Millisecond || c.budget > 30*time.Millisecond || c.calls.Load() == 0 {
				t.Fatalf("step escaped: elapsed=%s CDP budget=%s calls=%d err=%v", elapsed, c.budget, c.calls.Load(), err)
			}
			t.Logf("25ms budget: CDP=%s elapsed=%s", c.budget, elapsed)
		})
	}
}

func TestCDPInvalidateCancelsInFlightAndRejectsLateWrites(t *testing.T) {
	p, transport, c := fixture(t, "Input.dispatchMouseEvent")
	life, cancel := context.WithCancel(context.Background())
	end := Bind(p, life)
	defer end()
	done := make(chan error, 1)
	go func() { done <- p.Mouse.MoveTo(proto.Point{X: 1, Y: 1}) }()
	deadline := time.After(time.Second)
	for c.calls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("call not started")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	transport.Invalidate()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("in-flight write did not cancel")
	}
	if err := p.Mouse.MoveTo(proto.Point{X: 2, Y: 2}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if c.calls.Load() != 1 {
		t.Fatal("late write reached CDP")
	}
}
