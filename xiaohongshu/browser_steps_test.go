// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"
)

func TestBrowserStepBoundedMissingElement(t *testing.T) {
	start := time.Now()
	err := runBounded(context.Background(), 20*time.Millisecond, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("unbounded wait: %v", err)
	}
}

func TestBrowserStepParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runBounded(ctx, time.Minute, func(ctx context.Context) error { return ctx.Err() })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestBrowserRecoveryAtMostOnce(t *testing.T) {
	for _, secondFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "recovery_success", true: "still_failed"}[secondFails], func(t *testing.T) {
			attempts, recoveries := 0, 0
			err := recoverPageOnce(context.Background(), func() error {
				attempts++
				if attempts == 1 || secondFails {
					return context.DeadlineExceeded
				}
				return nil
			}, func() error { recoveries++; return nil })
			if attempts != 2 || recoveries != 1 || (err != nil) != secondFails {
				t.Fatalf("attempts=%d recovery=%d error=%v", attempts, recoveries, err)
			}
		})
	}
}

func TestBrowserRecoveryDoesNotRetryRiskOrLogin(t *testing.T) {
	for _, cause := range []error{ErrBrowserRisk, ErrBrowserLogin} {
		calls := 0
		err := recoverPageOnce(context.Background(), func() error { calls++; return &BrowserStepError{Cause: cause} }, func() error { t.Fatal("must not recover risk or login"); return nil })
		if !errors.Is(err, cause) || calls != 1 {
			t.Fatal(err, calls)
		}
	}
}

func TestPublishClickTimeoutNeverClicksTwice(t *testing.T) {
	for _, confirmSuccess := range []bool{false, true} {
		clicks, confirmations := 0, 0
		err := publishOnce(func() error { clicks++; return context.DeadlineExceeded }, func() error {
			confirmations++
			if confirmSuccess {
				return nil
			}
			return context.DeadlineExceeded
		})
		if clicks != 1 || confirmations != 1 {
			t.Fatalf("write replay: %d/%d", clicks, confirmations)
		}
		if confirmSuccess && err != nil {
			t.Fatal(err)
		}
		if !confirmSuccess && !errors.Is(err, ErrPublishResultUnknown) {
			t.Fatal(err)
		}
	}
}

func TestBrowserDiagnosticRedaction(t *testing.T) {
	raw := "https://user:pass@www.xiaohongshu.com/explore/123?xsec_token=SYNTHETIC_SECRET#fragment"
	u := safeDiagnosticURL(raw)
	if strings.Contains(u, "SYNTHETIC_SECRET") || strings.Contains(u, "user:pass") || strings.ContainsAny(u, "?#") {
		t.Fatal("URL is not redacted")
	}
	message := safeDiagnosticText("token=SYNTHETIC_SECRET cookie=SYNTHETIC_COOKIE " + raw)
	if strings.Contains(message, "SYNTHETIC_") {
		t.Fatal("error/title contains credential")
	}
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	src.Set(0, 0, color.RGBA{R: 255, A: 255})
	var b bytes.Buffer
	_ = png.Encode(&b, src)
	shot, err := redactDiagnosticPNG(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	masked, err := png.Decode(bytes.NewReader(shot))
	if err != nil {
		t.Fatal(err)
	}
	r, g, blue, _ := masked.At(0, 0).RGBA()
	if r != g || g != blue {
		t.Fatal("sensitive screenshot pixels not masked")
	}
}
