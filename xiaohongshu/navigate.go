// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"context"
	"time"

	"github.com/go-rod/rod"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

type NavigateAction struct {
	page *rod.Page
}

func NewNavigate(page *rod.Page) *NavigateAction {
	return &NavigateAction{page: page}
}

func (n *NavigateAction) ToExplorePage(ctx context.Context) error {
	page := n.page.Context(ctx)
	if err := NavigateBrowserPage(page, "https://www.xiaohongshu.com/explore", "explore.navigate"); err != nil {
		return err
	}
	_, err := stepElement(page, "explore.app", `div#app`)
	return err
}

func (n *NavigateAction) ToProfilePage(ctx context.Context) error {
	page := n.page.Context(ctx)
	const selector = `div.main-container li.user.side-bar-component a.link-wrapper span.channel`
	// Re-enter the explore page once only if read-only sidebar navigation fails.
	// The callback must never contain publishing or other account interactions.
	attempt := func() error {
		if err := BrowserStep(page, "profile.open_explore", "document load", navigationStepTimeout, func(p *rod.Page) error {
			if err := p.Navigate("https://www.xiaohongshu.com/explore"); err != nil {
				return err
			}
			if err := p.WaitLoad(); err != nil {
				return err
			}
			return pageGuard(p)
		}); err != nil {
			return err
		}
		if err := BrowserStep(page, "profile.sidebar", selector, elementStepTimeout, func(p *rod.Page) error {
			if err := pageGuard(p); err != nil {
				return err
			}
			elem, err := p.Element(selector)
			if err != nil {
				return err
			}
			humanize.Delay(p.GetContext(), humanize.BeforeClick)
			return humanize.Click(elem)
		}); err != nil {
			return err
		}
		return waitPageState(page, "profile.destination", "URL /user/profile/ + profile data", `() => location.pathname.startsWith('/user/profile/') && !!window.__INITIAL_STATE__?.user?.userPageData`)
	}
	recovery := func() error {
		return BrowserStep(page, "profile.recovery_guard", "risk/login state", 3*time.Second, pageGuard)
	}
	return recoverPageOnce(ctx, attempt, recovery)
}
