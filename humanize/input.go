// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package humanize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/xpzouying/xiaohongshu-mcp/internal/rodscope"
)

func pressAndRelease(ctx context.Context, mouse *rod.Mouse) error {
	if err := mouse.Down(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}

	timer := time.NewTimer(defaultProvider.Timing()[ClickHold].Sample())
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return ctx.Err()
	}

	return mouse.Up(proto.InputMouseButtonLeft, 1)
}

func jitterOffset(size float64) float64 {
	limit := math.Min(math.Abs(size)*0.15, 8)
	return (rand.Float64() - 0.5) * 2 * limit
}

func jitterInQuad(pt proto.Point, q proto.DOMQuad) proto.Point {
	return proto.Point{
		X: pt.X + jitterOffset(q[4]-q[0]),
		Y: pt.Y + jitterOffset(q[5]-q[1]),
	}
}

func jitterOn(elem *rod.Element, pt proto.Point) proto.Point {
	shape, err := elem.Shape()
	if err != nil || len(shape.Quads) == 0 {
		return pt
	}
	return jitterInQuad(pt, shape.Quads[0])
}

func ensurePointInViewport(page *rod.Page, pt proto.Point) error {
	res, err := page.Eval(`() => JSON.stringify([window.innerWidth, window.innerHeight])`)
	if err != nil {
		return err
	}

	var size []float64
	if json.Unmarshal([]byte(res.Value.Str()), &size) != nil || len(size) != 2 {
		return nil
	}

	if pt.X < 0 || pt.Y < 0 || pt.X > size[0] || pt.Y > size[1] {
		return fmt.Errorf("落点 (%.0f,%.0f) 在视口 %.0fx%.0f 之外", pt.X, pt.Y, size[0], size[1])
	}
	return nil
}

// 不用 document.elementFromPoint：结果不稳定
func ensureClickable(elem *rod.Element, pt proto.Point) error {
	if err := ensurePointInViewport(elem.Page(), pt); err != nil {
		return err
	}

	res, err := elem.Eval(`() => getComputedStyle(this).visibility`)
	if err != nil {
		return nil
	}
	if res.Value.Str() == "hidden" {
		return errors.New("元素当前不可命中")
	}
	return nil
}

func Click(elem *rod.Element) error {
	ctx, cancel := context.WithTimeout(elem.GetContext(), 10*time.Second)
	defer cancel()
	end := rodscope.Bind(elem.Page(), ctx)
	defer end()
	elem = elem.Context(ctx)
	pt, err := elem.WaitInteractable()
	if err != nil {
		return err
	}

	target := jitterOn(elem, *pt)
	if err := ensureClickable(elem, target); err != nil {
		return err
	}

	mouse := elem.Page().Mouse
	if err := moveMouseCurved(mouse, target); err != nil {
		return err
	}

	Delay(ctx, PointerSettle)

	if err := elem.WaitEnabled(); err != nil {
		return err
	}

	return pressAndRelease(ctx, mouse)
}

// ClickNoWait 跳过 WaitInteractable 的遮挡重试，用于它会误判而死等的场景。
func ClickNoWait(elem *rod.Element) error {
	ctx, cancel := context.WithTimeout(elem.GetContext(), 10*time.Second)
	defer cancel()
	end := rodscope.Bind(elem.Page(), ctx)
	defer end()
	elem = elem.Context(ctx)
	shape, err := elem.Shape()
	if err != nil {
		return err
	}
	if len(shape.Quads) == 0 {
		return errors.New("元素无可点击区域")
	}

	q := shape.Quads[0]
	center := proto.Point{X: (q[0] + q[4]) / 2, Y: (q[1] + q[5]) / 2}

	target := jitterInQuad(center, q)
	if err := ensureClickable(elem, target); err != nil {
		return err
	}

	mouse := elem.Page().Mouse
	if err := moveMouseCurved(mouse, target); err != nil {
		return err
	}
	return pressAndRelease(ctx, mouse)
}

func MoveTo(page *rod.Page, pt proto.Point) error {
	end := rodscope.Bind(page, page.GetContext())
	defer end()
	return moveMouseCurved(page.Mouse, pt)
}

func Hover(elem *rod.Element) error {
	ctx, cancel := context.WithTimeout(elem.GetContext(), 10*time.Second)
	defer cancel()
	end := rodscope.Bind(elem.Page(), ctx)
	defer end()
	elem = elem.Context(ctx)
	shape, err := elem.Shape()
	if err != nil {
		return err
	}
	if len(shape.Quads) == 0 {
		return errors.New("元素无可悬停区域")
	}

	q := shape.Quads[0]
	center := proto.Point{X: (q[0] + q[4]) / 2, Y: (q[1] + q[5]) / 2}

	target := jitterInQuad(center, q)
	if err := ensureClickable(elem, target); err != nil {
		return err
	}

	return moveMouseCurved(elem.Page().Mouse, target)
}

func ClickAt(page *rod.Page, pt proto.Point) error {
	end := rodscope.Bind(page, page.GetContext())
	defer end()
	if err := ensurePointInViewport(page, pt); err != nil {
		return err
	}
	if err := moveMouseCurved(page.Mouse, pt); err != nil {
		return err
	}
	return pressAndRelease(page.GetContext(), page.Mouse)
}

func Type(ctx context.Context, elem *rod.Element, text string) error {
	end := rodscope.Bind(elem.Page(), ctx)
	defer end()
	dist := defaultProvider.Timing()[Keystroke]
	// Bound only readiness checks; normal human-paced typing retains its original
	// caller budget, rather than failing a long but valid body after ten seconds.
	if err := func() error {
		readyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		end := rodscope.Bind(elem.Page(), readyCtx)
		defer end()
		ready := elem.Context(readyCtx)
		if err := ready.Focus(); err != nil {
			return err
		}
		if err := ready.WaitEnabled(); err != nil {
			return err
		}
		return ready.WaitWritable()
	}(); err != nil {
		return err
	}

	page := elem.Page().Context(ctx)

	for _, r := range text {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := page.InsertText(string(r)); err != nil {
			return err
		}

		t := time.NewTimer(dist.Sample())
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		}
	}
	return nil
}
