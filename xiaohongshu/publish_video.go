// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/pkg/errors"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

// PublishVideoContent 发布视频内容
type PublishVideoContent struct {
	Title        string
	Content      string
	Tags         []string
	VideoPath    string
	ScheduleTime *time.Time // 定时发布时间，nil 表示立即发布
	AIGenerated  bool       // 是否声明含有 AI 合成内容
	Visibility   string     // 可见范围: "公开可见"(默认), "仅自己可见", "仅互关好友可见"
	Products     []string   // 商品关键词列表，用于绑定带货商品
}

// NewPublishVideoAction 进入发布页并切换到"上传视频"
func NewPublishVideoAction(page *rod.Page) (*PublishAction, error) {
	if err := preparePublishPage(page, "上传视频"); err != nil {
		return nil, err
	}
	return &PublishAction{page: page}, nil
}

// PublishVideo 上传视频并提交
func (p *PublishAction) PublishVideo(ctx context.Context, content PublishVideoContent) error {
	if content.VideoPath == "" {
		return errors.New("视频不能为空")
	}

	// 重设超时：.Context(ctx) 会替换掉 NewPublishVideoAction 里 Timeout(300s) 的 deadline
	page := p.page.Context(ctx).Timeout(300 * time.Second)

	if err := uploadVideo(page, content.VideoPath); err != nil {
		return errors.Wrap(err, "小红书上传视频失败")
	}

	if err := submitPublishVideo(ctx, page, content.Title, content.Content, content.Tags, content.ScheduleTime, content.AIGenerated, content.Visibility, content.Products); err != nil {
		return errors.Wrap(err, "小红书发布失败")
	}
	return nil
}

// uploadVideo 上传单个本地视频
func uploadVideo(page *rod.Page, videoPath string) error {
	pp := page.Timeout(5 * time.Minute) // 视频处理耗时更长

	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		return errors.Wrapf(err, "视频文件不存在: %s", videoPath)
	}

	// 寻找文件上传输入框（与图文一致的 class，或退回到 input[type=file]）
	var fileInput *rod.Element
	var err error
	fileInput, err = stepElement(pp, "publish_video.upload_input", ".upload-input")
	if err != nil || fileInput == nil {
		fileInput, err = stepElement(pp, "publish_video.upload_input_fallback", "input[type='file']")
		if err != nil || fileInput == nil {
			return errors.New("未找到视频上传输入框")
		}
	}

	if err := fileInput.Timeout(elementStepTimeout).SetFiles([]string{videoPath}); err != nil {
		return err
	}

	// 对于视频，等待发布按钮变为可点击即表示处理完成
	var btn *publishButton
	// Processing is not a normal element wait. Preserve a larger bounded budget
	// for video transcoding, still inside the existing five-minute parent cap.
	err = BrowserStep(pp, "publish_video.processing", "video processing + enabled publish button", 3*time.Minute, func(p *rod.Page) error {
		var err error
		btn, err = waitForPublishButtonClickable(p, 3*time.Minute)
		return err
	})
	if err != nil {
		return err
	}
	slog.Info("视频上传/处理完成，发布按钮可点击", "btn", btn)
	return nil
}

// submitPublishVideo 填写标题、正文、标签并点击发布（等待按钮可点击后再提交）
func submitPublishVideo(ctx context.Context, page *rod.Page, title, content string, tags []string, scheduleTime *time.Time, aiGenerated bool, visibility string, products []string) error {
	// 标题
	titleElem, err := stepElement(page, "publish_video.title", "div.d-input input")
	if err != nil {
		return errors.Wrap(err, "查找标题输入框失败")
	}
	if err := BrowserStep(page, "publish_video.title_input", "div.d-input input writable", elementStepTimeout, func(p *rod.Page) error {
		return humanize.Type(p.GetContext(), titleElem.Context(p.GetContext()), title)
	}); err != nil {
		return errors.Wrap(err, "输入标题失败")
	}
	humanize.Delay(ctx, humanize.AfterType)

	// 正文 + 标签
	var contentElem *rod.Element
	err = BrowserStep(page, "publish_video.body_editor", strings.Join(contentElemSelectors, " / "), elementStepTimeout, func(p *rod.Page) error {
		var err error
		contentElem, err = getContentElement(p, contentElemTimeout)
		return err
	})
	if err != nil {
		return err
	}
	contentElem, err = rebindElement(page, contentElem, "publish.video_body_rebind", "body editor")
	if err != nil {
		return err
	}
	if err := humanize.Type(ctx, contentElem, content); err != nil {
		return errors.Wrap(err, "输入正文失败")
	}
	if err := waitAndClickTitleInput(titleElem); err != nil {
		return err
	}
	if err := inputTags(ctx, contentElem, tags); err != nil {
		return err
	}

	humanize.Delay(ctx, humanize.AfterType)

	// 处理定时发布
	if scheduleTime != nil {
		if err := BrowserStep(page, "publish_video.schedule", "schedule switch and datetime", 20*time.Second, func(p *rod.Page) error { return setSchedulePublish(p.GetContext(), p, *scheduleTime) }); err != nil {
			return errors.Wrap(err, "设置定时发布失败")
		}
		slog.Info("定时发布设置完成", "schedule_time", scheduleTime.Format("2006-01-02 15:04"))
	}

	// 设置可见范围
	if err := BrowserStep(page, "publish_video.visibility", "permission-card-wrapper", 20*time.Second, func(p *rod.Page) error { return setVisibility(p, visibility) }); err != nil {
		return errors.Wrap(err, "设置可见范围失败")
	}

	// 绑定商品
	if err := BrowserStep(page, "publish_video.products", "goods modal", 30*time.Second, func(p *rod.Page) error { return bindProducts(p.GetContext(), p, products) }); err != nil {
		return errors.Wrap(err, "绑定商品失败")
	}
	if err := BrowserStep(page, "publish_video.ai_declaration", "AI dropdown description AND declaration summary", 20*time.Second, func(p *rod.Page) error {
		return ensureAIGeneratedDeclaration(&rodAIDeclarationUI{page: p}, aiGenerated)
	}); err != nil {
		return err
	}

	return submitPublishOnce(page)
}
