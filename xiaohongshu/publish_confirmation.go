// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

var ErrPublishResultUnknown = errors.New("发布结果未确认；可能已经发布，请先检查账号主页，禁止直接重试")

// A click can reach Chromium even when its acknowledgement times out. Verify
// once in either case; never click again or reload after this point.
func publishOnce(click, confirm func() error) error {
	clickErr := click()
	confirmErr := confirm()
	if confirmErr == nil {
		return nil
	}
	if clickErr != nil {
		return fmt.Errorf("%w; click=%s; confirmation=%w", ErrPublishResultUnknown, safeDiagnosticText(clickErr.Error()), confirmErr)
	}
	return fmt.Errorf("%w; confirmation=%w", ErrPublishResultUnknown, confirmErr)
}

func submitPublishOnce(page *rod.Page) error {
	if deadline, ok := page.GetContext().Deadline(); ok && time.Until(deadline) < 65*time.Second {
		return errors.New("发布前剩余时间不足65秒，未点击发布；为按钮检查、单次点击和成功核验预留预算")
	}
	var btn *publishButton
	if err := BrowserStep(page, "publish.button_ready", "xhs-publish-btn / button.bg-red enabled", elementStepTimeout, func(p *rod.Page) error {
		if err := pageGuard(p); err != nil {
			return err
		}
		var err error
		btn, err = waitForPublishButtonClickable(p, elementStepTimeout)
		return err
	}); err != nil {
		return err
	}
	var err error
	btn.elem, err = rebindElement(page, btn.elem, "publish.button_rebind", "publish button")
	if err != nil {
		return err
	}
	return publishOnce(func() error {
		return BrowserStep(page, "publish.click", "one irreversible publish click", 10*time.Second, func(p *rod.Page) error {
			elem := btn.elem.Context(p.GetContext())
			if btn.isWidget {
				return clickPublishWidget(p, elem)
			}
			return humanize.Click(elem)
		})
	}, func() error { return waitPublishSuccess(page, 30*time.Second) })
}
