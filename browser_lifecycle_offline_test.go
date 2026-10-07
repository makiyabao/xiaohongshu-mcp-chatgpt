// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package main

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

type offlineLifecycleBrowser struct {
	b          *rod.Browser
	l          *launcher.Launcher
	blockClose <-chan struct{}
}

func (b *offlineLifecycleBrowser) NewPage() *rod.Page { return b.b.MustPage() }
func (b *offlineLifecycleBrowser) Close() {
	if b.blockClose != nil {
		<-b.blockClose
	}
	_ = b.b.Close()
	b.l.Kill()
}

func TestOfflineManagedBrowserCancellationAndCleanup(t *testing.T) {
	bin := os.Getenv("XHS_OFFLINE_BROWSER_BIN")
	if bin == "" {
		t.Skip("offline browser path not supplied")
	}
	// Direct installed Chrome fixture. Production headless_browser options are
	// unchanged; offline tests use a fresh profile and no leakless helper download.
	l := launcher.New().Bin(bin).UserDataDir(t.TempDir()).Headless(true).Leakless(false).Set("disable-background-networking").Set("no-first-run")
	control, err := l.Launch()
	if err != nil {
		t.Fatal(err)
	}
	b := rod.New().ControlURL(control)
	if err := b.Connect(); err != nil {
		l.Kill()
		t.Fatal(err)
	}
	raw := &offlineLifecycleBrowser{b: b, l: l}
	m := manageBrowser(raw)
	defer m.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := m.NewPage(ctx)
	if m.pid <= 0 {
		t.Fatal("own root browser PID unavailable")
	}
	if _, err := p.Eval(`() => 1`); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := p.Mouse.MoveTo(proto.Point{X: 1, Y: 1}); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled generation sent input", err)
	}
	start := time.Now()
	closeBrowserPage(p)
	m.Close()
	if time.Since(start) > 6*time.Second {
		t.Fatal("cleanup exceeded independent deadline")
	}
	if err := p.Mouse.MoveTo(proto.Point{X: 2, Y: 2}); !errors.Is(err, context.Canceled) {
		t.Fatal("closed generation remains valid", err)
	}
}

func TestOfflineManagedBrowserHungCloseTerminatesExactProcess(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux /proc termination confirmation")
	}
	bin := os.Getenv("XHS_OFFLINE_BROWSER_BIN")
	if bin == "" {
		t.Skip("offline browser path not supplied")
	}
	l := launcher.New().Bin(bin).UserDataDir(t.TempDir()).Headless(true).Leakless(false).Set("disable-background-networking")
	control, err := l.Launch()
	if err != nil {
		t.Fatal(err)
	}
	b := rod.New().ControlURL(control)
	if err := b.Connect(); err != nil {
		l.Kill()
		t.Fatal(err)
	}
	unblock := make(chan struct{})
	defer close(unblock)
	defer l.Kill()
	m := manageBrowser(&offlineLifecycleBrowser{b: b, l: l, blockClose: unblock})
	defer m.Close()
	p := m.NewPage(context.Background())
	if m.processIdentity == "" {
		t.Fatal("own PID identity not captured")
	}
	start := time.Now()
	m.Close()
	if time.Since(start) > 5*time.Second {
		t.Fatal("hung close held lifecycle")
	}
	identity, state, err := browserProcessIdentity(m.pid)
	if err == nil && identity == m.processIdentity && state != "Z" && state != "X" {
		t.Fatal("old Chrome still running")
	}
	if err := p.Mouse.MoveTo(proto.Point{X: 1, Y: 1}); !errors.Is(err, context.Canceled) {
		t.Fatal("old generation input accepted", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	release, err := globalBrowserWrites.acquire(ctx)
	if err != nil {
		t.Fatal("next writer blocked after confirmed termination", err)
	}
	release()
}
