// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

type browserWriteGate struct {
	once    sync.Once
	token   chan struct{}
	blocked bool
	mu      sync.Mutex
}

// Process-global, cancelable while queued; all service instances and both MCP
// and HTTP entry points share it. Lock before starting Chromium, release after
// bounded cleanup/invalidation. Waiting does not consume the publish budget.
func (g *browserWriteGate) acquire(ctx context.Context) (func(), error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	g.once.Do(func() { g.token = make(chan struct{}, 1) })
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("浏览器写操作排队已取消，本次未启动浏览器或执行写操作: %w", err)
	}
	select {
	case g.token <- struct{}{}:
		g.mu.Lock()
		blocked := g.blocked
		g.mu.Unlock()
		if blocked {
			<-g.token
			return nil, errors.New("旧浏览器终止未确认，写操作已隔离；需人工核验后重启实例")
		}
		if err := ctx.Err(); err != nil {
			<-g.token
			return nil, err
		}
		var once sync.Once
		return func() { once.Do(func() { <-g.token }) }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("已有浏览器写操作运行，排队超时或取消；本次未启动浏览器或执行写操作: %w", ctx.Err())
	}
}

func (g *browserWriteGate) quarantine() { g.mu.Lock(); g.blocked = true; g.mu.Unlock() }

var globalBrowserWrites browserWriteGate

// Access only while holding globalBrowserWrites. Keep hashes, never a draft,
// image URL or token; this is a short process-local guard, not durable dedup.
type uncertainPublishGuard struct{ blocked map[[32]byte]time.Time }

func publishFingerprint(req *PublishRequest) [32]byte {
	data, _ := json.Marshal(req)
	return sha256.Sum256(data)
}

func (g *uncertainPublishGuard) check(req *PublishRequest, now time.Time) error {
	for key, until := range g.blocked {
		if !now.Before(until) {
			delete(g.blocked, key)
		}
	}
	if now.Before(g.blocked[publishFingerprint(req)]) {
		return fmt.Errorf("%w；相同请求15分钟内禁止重放，未启动浏览器；请人工核验，不要更换参数规避保护", xiaohongshu.ErrPublishResultUnknown)
	}
	return nil
}

func (g *uncertainPublishGuard) record(req *PublishRequest, err error, now time.Time) {
	if !errors.Is(err, xiaohongshu.ErrPublishResultUnknown) {
		return
	}
	if g.blocked == nil {
		g.blocked = make(map[[32]byte]time.Time)
	}
	g.blocked[publishFingerprint(req)] = now.Add(15 * time.Minute)
}

var uncertainImagePublishes uncertainPublishGuard
