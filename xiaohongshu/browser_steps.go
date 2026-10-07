// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/xpzouying/xiaohongshu-mcp/internal/rodscope"
)

const elementStepTimeout = 15 * time.Second
const navigationStepTimeout = 30 * time.Second

var ErrBrowserRisk = errors.New("检测到风控/验证，已停止，不恢复或重试")
var ErrBrowserLogin = errors.New("页面进入登录状态，已停止，需人工处理")

type BrowserStepError struct {
	Step, Selector, URL, Title, Diagnostic string
	Cause                                  error
}

func (e *BrowserStepError) Error() string {
	reason := safeDiagnosticText(e.Cause.Error())
	var declaration *aiDeclarationError
	if errors.As(e.Cause, &declaration) && aiDeclarationMessage(declaration.Code) != "" {
		reason = declaration.Error()
	}
	return fmt.Sprintf("浏览器步骤失败：step=%s; url=%s; title=%q; waiting=%q; reason=%s; diagnostic=%s",
		e.Step, e.URL, e.Title, e.Selector, reason, e.Diagnostic)
}
func (e *BrowserStepError) Unwrap() error { return e.Cause }

// runBounded executes synchronously: Rod receives the child context, so no abandoned
// goroutine can continue an interaction after the caller receives a timeout.
func runBounded(ctx context.Context, budget time.Duration, fn func(context.Context) error) (err error) {
	stepCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	defer func() {
		if p := recover(); p != nil {
			if cause, ok := p.(error); ok {
				err = cause
			} else {
				err = errors.New("Rod operation panicked")
			}
		}
	}()
	err = fn(stepCtx)
	if err == nil {
		err = stepCtx.Err()
	}
	return err
}

// A single recovery is allowed only for read-only page preparation, never for a
// publishing/editing/interaction callback. Risk, login, cancellation stop immediately.
func recoverPageOnce(ctx context.Context, attempt, recovery func() error) error {
	err := attempt()
	if err == nil || recovery == nil || ctx.Err() != nil || errors.Is(err, ErrBrowserRisk) || errors.Is(err, ErrBrowserLogin) {
		return err
	}
	if err := recovery(); err != nil {
		return err
	}
	return attempt()
}

// BrowserStep keeps the original page context intact. Returning elements must
// rebind their context to the parent before this function cancels its child.
func BrowserStep(page *rod.Page, step, waiting string, budget time.Duration, fn func(*rod.Page) error) error {
	err := runBounded(page.GetContext(), budget, func(ctx context.Context) error {
		end := rodscope.Bind(page, ctx)
		defer end()
		return fn(page.Context(ctx))
	})
	if err == nil {
		return nil
	}
	var existing *BrowserStepError
	if errors.As(err, &existing) {
		return err
	}
	var declaration *aiDeclarationError
	errors.As(err, &declaration)
	d := captureBrowserDiagnostic(page, step, waiting, declaration)
	return &BrowserStepError{Step: step, Selector: waiting, URL: d.URL, Title: d.Title, Diagnostic: d.Artifact, Cause: err}
}

func stepElement(page *rod.Page, step, selector string) (*rod.Element, error) {
	var elem *rod.Element
	err := BrowserStep(page, step, selector, elementStepTimeout, func(p *rod.Page) error {
		var err error
		elem, err = p.Element(selector)
		return err
	})
	if err != nil {
		return nil, err
	}
	return rebindElement(page, elem, step+".rebind", selector)
}

func rebindElement(page *rod.Page, elem *rod.Element, step, selector string) (*rod.Element, error) {
	var bound *rod.Element
	err := BrowserStep(page, step, selector, elementStepTimeout, func(_ *rod.Page) error {
		var err error
		bound, err = page.ElementFromObject(elem.Object)
		return err
	})
	return bound, err
}

func waitPageState(page *rod.Page, step, state, javascript string) error {
	return BrowserStep(page, step, state, elementStepTimeout, func(p *rod.Page) error {
		return p.Wait(rod.Eval(javascript))
	})
}

func pageGuard(page *rod.Page) error {
	res, err := page.Eval(`() => {
  if (/\/(login|signin)(\/|$)/i.test(location.pathname)) return 'login';
  const visible = el => el && el.getBoundingClientRect().width > 0 && el.getBoundingClientRect().height > 0;
  if (Array.from(document.querySelectorAll('#captcha-verify,.captcha-container,.verify-container,.red-captcha-container,iframe[src*="/captcha/"],[role="dialog"][data-testid="captcha"]')).some(visible)) return 'risk';
  // Ordinary note text is not a risk signal. Only dedicated URL/visible controls.
  if (/^\/(captcha|verify|verification)(\/|$)/i.test(location.pathname)) return 'risk';
  return '';
}`)
	if err != nil {
		return err
	}
	switch res.Value.Str() {
	case "risk":
		return ErrBrowserRisk
	case "login":
		return ErrBrowserLogin
	}
	return nil
}

// NavigateBrowserPage bounds each navigation and retries page preparation once.
// It is not used after uploading/editing or after any irreversible click.
func NavigateBrowserPage(page *rod.Page, target, step string) error {
	attempt := func() error {
		return BrowserStep(page, step, "navigation + document load", navigationStepTimeout, func(p *rod.Page) error {
			if err := p.Navigate(target); err != nil {
				return err
			}
			if err := p.WaitLoad(); err != nil {
				return err
			}
			return pageGuard(p)
		})
	}
	recovery := func() error {
		return BrowserStep(page, step+".recovery_guard", "risk/login state", 3*time.Second, pageGuard)
	}
	return recoverPageOnce(page.GetContext(), attempt, recovery)
}

func safeDiagnosticURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "[unavailable]"
	}
	// Paths contain account/note identifiers; retain only a classified route.
	return u.Scheme + "://" + u.Host + "/" + diagnosticPageState(u.Path)
}

func diagnosticPageState(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for _, segment := range parts {
		switch segment {
		case "login", "signin":
			return "login"
		case "captcha", "verify", "verification":
			return "verification"
		case "profile":
			return "profile"
		case "publish":
			return "publish"
		}
	}
	return "unknown"
}

func safeDiagnosticText(raw string) string {
	// Error strings can contain arbitrary JSON, request bodies or account text.
	// Exact allowlist, never regex-based removal of supposedly known secrets.
	switch raw {
	case "context deadline exceeded", "context canceled", "Rod operation panicked":
		return raw
	case ErrBrowserRisk.Error(), ErrBrowserLogin.Error(), ErrPublishResultUnknown.Error():
		return raw
	default:
		return "browser operation failed (details omitted)"
	}
}
