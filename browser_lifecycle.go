// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/xpzouying/xiaohongshu-mcp/internal/rodscope"
)

const browserCleanupTimeout = 3 * time.Second

// Invalidation happens synchronously BEFORE cleanup; the abandoned cleanup
// goroutine may close a browser but cannot send any more input/navigation CDP.
// A failed force termination quarantines new writes rather than risking overlap.
func boundedBrowserCleanup(budget time.Duration, invalidate func(), close func(), terminate func() error) error {
	invalidate()
	done := make(chan error, 1)
	go func() {
		var err error
		defer func() {
			if recover() != nil {
				err = errors.New("browser cleanup panicked")
			}
			done <- err
		}()
		close()
	}()
	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case err := <-done:
		if err == nil {
			return nil
		}
	case <-timer.C:
	}
	return terminate()
}

type managedBrowser struct {
	raw             pageBrowser
	rod             *rod.Browser
	transport       *rodscope.Transport
	life            context.Context
	cancel          context.CancelFunc
	end             func()
	pid             int
	processIdentity string
	once            sync.Once
}

type pageBrowser interface {
	NewPage() *rod.Page
	Close()
}

func manageBrowser(raw pageBrowser) *managedBrowser {
	ctx, cancel := context.WithCancel(context.Background())
	return &managedBrowser{raw: raw, life: ctx, cancel: cancel}
}

func (m *managedBrowser) NewPage(operation ...context.Context) *rod.Page {
	p := m.raw.NewPage()
	if m.rod == nil {
		m.rod = p.Browser()
		m.transport = rodscope.Install(m.rod)
		m.end = rodscope.Bind(p, m.life)
		ctx, cancel := context.WithTimeout(m.life, 2*time.Second)
		defer cancel()
		if info, err := (proto.SystemInfoGetProcessInfo{}).Call(m.rod.Context(ctx)); err == nil {
			for _, process := range info.ProcessInfo {
				if process.Type == "browser" {
					m.pid = process.ID
					m.processIdentity, _, _ = browserProcessIdentity(m.pid)
				}
			}
		}
	}
	if len(operation) != 0 {
		previous := m.end
		end := rodscope.Bind(p, operation[0])
		m.end = func() {
			end()
			if previous != nil {
				previous()
			}
		}
	}
	return p.Context(m.life)
}

func (m *managedBrowser) Close() {
	m.once.Do(func() {
		err := boundedBrowserCleanup(browserCleanupTimeout, func() {
			m.cancel()
			if m.transport != nil {
				m.transport.Invalidate()
			}
		}, m.raw.Close, m.terminateOwnedBrowser)
		if m.end != nil {
			m.end()
		}
		if m.transport != nil {
			m.transport.Forget(m.rod)
		}
		if err != nil {
			globalBrowserWrites.quarantine()
		}
	})
}

// /proc start ticks identify this exact Chrome process, avoiding PID reuse.
// These process-local values are never diagnostic output or account state.
func browserProcessIdentity(pid int) (identity, state string, err error) {
	if runtime.GOOS != "linux" || pid <= 0 {
		return "", "", errors.New("process identity unavailable")
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", "", err
	}
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return "", "", errors.New("invalid process state")
	}
	fields := strings.Fields(string(data)[end+1:])
	if len(fields) < 20 {
		return "", "", errors.New("invalid process state")
	}
	return fields[19], fields[0], nil
}

func (m *managedBrowser) terminateOwnedBrowser() error {
	if m.pid <= 0 || m.processIdentity == "" {
		return errors.New("browser termination could not be confirmed")
	}
	identity, state, err := browserProcessIdentity(m.pid)
	if errors.Is(err, os.ErrNotExist) || err == nil && (identity != m.processIdentity || state == "Z" || state == "X") {
		return nil
	}
	if err != nil {
		return errors.New("browser termination could not be confirmed")
	}
	p, err := os.FindProcess(m.pid)
	if err != nil {
		return errors.New("browser termination failed")
	}
	if err := p.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
		return errors.New("browser termination failed")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		identity, state, err := browserProcessIdentity(m.pid)
		if errors.Is(err, os.ErrNotExist) || err == nil && (identity != m.processIdentity || state == "Z" || state == "X") {
			return nil
		}
		if err != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("browser termination could not be confirmed")
}

func closeBrowserPage(page *rod.Page) {
	rodscope.Install(page.Browser()).Invalidate()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Page.Close waits for event acknowledgements using its page context.
	// Target.closeTarget is a bounded close command without that event loop.
	_, _ = (proto.TargetCloseTarget{TargetID: page.TargetID}).Call(page.Browser().Context(ctx))
}
