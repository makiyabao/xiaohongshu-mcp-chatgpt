// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/xpzouying/xiaohongshu-mcp/internal/rodscope"
)

// Opt-in offline browser tests: only an explicitly provided installed browser,
// a brand-new temporary profile, and loopback fixtures. No auto-download/login.
func offlinePage(t *testing.T, html string) (*rod.Page, func()) {
	t.Helper()
	bin := os.Getenv("XHS_OFFLINE_BROWSER_BIN")
	if bin == "" {
		t.Skip("offline browser path not supplied")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, html)
	}))
	l := launcher.New().Bin(bin).UserDataDir(t.TempDir()).Headless(true).Leakless(false).Set("disable-background-networking").Set("no-first-run").Set("no-default-browser-check")
	control, err := l.Launch()
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	b := rod.New().ControlURL(control)
	if err := b.Connect(); err != nil {
		l.Kill()
		server.Close()
		t.Fatal(err)
	}
	page, err := b.Page(proto.TargetCreateTarget{URL: server.URL})
	if err != nil {
		b.Close()
		l.Kill()
		server.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		transport := rodscope.Install(b)
		transport.Invalidate()
		_ = b.Close()
		transport.Forget(b)
		l.Kill()
		server.Close()
	})
	if err := page.Timeout(5 * time.Second).WaitLoad(); err != nil {
		t.Fatal(err)
	}
	return page, func() {}
}

func TestOfflineRodMissingElementDiagnostics(t *testing.T) {
	t.Setenv("TMP", t.TempDir())
	t.Setenv("TEMP", os.Getenv("TMP"))
	t.Setenv("TMPDIR", os.Getenv("TMP"))
	page, close := offlinePage(t, `<title>offline fixture</title><p>No target</p><input type="password" value="SYNTHETIC_SECRET">`)
	defer close()
	start := time.Now()
	err := BrowserStep(page, "fixture.missing", "#never-present", 50*time.Millisecond, func(p *rod.Page) error { _, err := p.Element("#never-present"); return err })
	var stepErr *BrowserStepError
	if !errors.As(err, &stepErr) || !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 5*time.Second {
		t.Fatalf("missing element did not stop correctly: %v", err)
	}
	if stepErr.Step != "fixture.missing" || stepErr.Title != "unknown" || !strings.Contains(stepErr.URL, "127.0.0.1") {
		t.Fatalf("missing diagnostics: %v", err)
	}
	data, err := os.ReadFile(stepErr.Diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "SYNTHETIC_SECRET") {
		t.Fatal("sensitive state persisted")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(stepErr.Diagnostic), "screenshot.redacted.png")); err != nil {
		t.Fatal(err)
	}
	// The lookup child was canceled; the parent page must remain usable.
	if _, err := page.Info(); err != nil {
		t.Fatal("parent context canceled", err)
	}
}

func TestOfflineRodNavigationFailureAndRecovery(t *testing.T) {
	page, close := offlinePage(t, `<p>fixture</p>`)
	defer close()
	var attempts, recoveries int
	err := recoverPageOnce(page.GetContext(), func() error {
		attempts++
		return BrowserStep(page, "fixture.navigate", "ready URL", 40*time.Millisecond, func(p *rod.Page) error {
			if attempts == 1 {
				return p.Wait(rod.Eval(`() => false`))
			}
			return p.Navigate("about:blank")
		})
	}, func() error { recoveries++; return nil })
	if err != nil || attempts != 2 || recoveries != 1 {
		t.Fatal(err, attempts, recoveries)
	}
	err = BrowserStep(page, "fixture.navigation_failure", "navigation", 50*time.Millisecond, func(p *rod.Page) error { return p.Navigate("://invalid-url") })
	if err == nil {
		t.Fatal("failed navigation accepted")
	}
}

func TestOfflineRodPublishClickTimeoutNoDuplicate(t *testing.T) {
	page, close := offlinePage(t, `<button id="submit" onclick="window.clicks=(window.clicks||0)+1">Publish fixture</button>`)
	defer close()
	var calls atomic.Int32
	err := publishOnce(func() error {
		calls.Add(1)
		if _, err := page.Eval(`() => document.querySelector('#submit').click()`); err != nil {
			return err
		}
		return context.DeadlineExceeded
	}, func() error {
		r, err := page.Eval(`() => window.clicks`)
		if err != nil {
			return err
		}
		if r.Value.Int() != 1 {
			return errors.New("duplicate click")
		}
		return nil
	})
	if err != nil || calls.Load() != 1 {
		t.Fatal(err, calls.Load())
	}
}

func TestOfflineRodElementRebindAndSubmit(t *testing.T) {
	page, close := offlinePage(t, `<title>fixture</title><div class="publish-page-publish-btn"><button class="bg-red" onclick="window.clicks=(window.clicks||0)+1;document.querySelector('#status').textContent='发布成功'">Publish</button></div><p id="status"></p>`)
	defer close()
	button, err := stepElement(page, "fixture.button", "button.bg-red")
	if err != nil {
		t.Fatal(err)
	}
	// The lookup child has already been canceled; returning an element must not
	// leave the caller holding that canceled page, as Element.Context alone does.
	if _, err := button.Page().Info(); err != nil {
		t.Fatal("returned element has dead page context", err)
	}
	if err := submitPublishOnce(page); err != nil {
		t.Fatal(err)
	}
	count, err := page.Eval(`() => window.clicks`)
	if err != nil || count.Value.Int() != 1 {
		t.Fatal("submit did not click exactly once", err)
	}
}

func TestOfflineRodPublishInsufficientBudgetDoesNotClick(t *testing.T) {
	page, close := offlinePage(t, `<button onclick="window.clicks=(window.clicks||0)+1">Publish</button>`)
	defer close()
	err := submitPublishOnce(page.Timeout(20 * time.Second))
	if err == nil || !strings.Contains(err.Error(), "未点击发布") {
		t.Fatal("insufficient deadline accepted", err)
	}
	count, err := page.Eval(`() => window.clicks || 0`)
	if err != nil || count.Value.Int() != 0 {
		t.Fatal("clicked without verification budget", err)
	}
}

func TestOfflineRodRiskStopsRecovery(t *testing.T) {
	page, close := offlinePage(t, `<div class="verify-container">安全验证</div>`)
	defer close()
	count := 0
	err := recoverPageOnce(page.GetContext(), func() error { count++; return pageGuard(page) }, func() error { t.Fatal("risk page must not be reloaded"); return nil })
	if !errors.Is(err, ErrBrowserRisk) || count != 1 {
		t.Fatal("risk did not stop immediately", err, count)
	}
}
