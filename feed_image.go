// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/h2non/filetype"
	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

const (
	maxFeedImageBytes       int64 = 15 * 1024 * 1024
	feedImageRequestTimeout       = 20 * time.Second
	// feedImageWidgetResultMetaKey contains widget-only routing data. The image
	// bytes themselves remain only in standard MCP ImageContent, so they are not
	// duplicated in structuredContent or _meta.
	feedImageWidgetResultMetaKey = "xiaohongshu/feed-image"
)

type downloadedFeedImage struct {
	data   []byte
	mime   string
	width  int
	height int
}

// handleGetFeedImage 复用详情页 imageList，返回标准 MCP 图片内容而非 URL。
func (s *AppServer) handleGetFeedImage(ctx context.Context, args FeedImageArgs) *MCPToolResult {
	if args.FeedID == "" {
		return feedImageError("缺少 feed_id 参数")
	}
	if args.XsecToken == "" {
		return feedImageError("缺少 xsec_token 参数")
	}
	if args.ImageIndex < 0 {
		return feedImageError("image_index 必须大于等于 0")
	}

	// 不记录 token、图片 URL 或图片内容；feed 详情服务本身负责从已登录会话读取数据。
	logrus.Infof("MCP: 获取笔记图片，image_index=%d", args.ImageIndex)
	detail, err := s.xiaohongshuService.GetFeedDetail(ctx, args.FeedID, args.XsecToken, false)
	if err != nil {
		return feedImageError("获取笔记详情失败: " + err.Error())
	}

	images, ok := feedDetailImages(detail.Data)
	if !ok {
		return feedImageError("笔记详情格式异常，未找到 imageList")
	}
	if len(images) == 0 {
		return feedImageError("该笔记没有可下载的图文图片")
	}
	if args.ImageIndex >= len(images) {
		return feedImageError(fmt.Sprintf("image_index 超出范围：该笔记共有 %d 张图片", len(images)))
	}

	// urlDefault 是详情页的首选源；仅在它为空时使用同一条详情记录里的 urlPre。
	sourceURL := images[args.ImageIndex].URLDefault
	if sourceURL == "" {
		sourceURL = images[args.ImageIndex].URLPre
	}
	if sourceURL == "" {
		return feedImageError("所选图片没有可用下载地址")
	}

	imageData, err := downloadFeedImage(ctx, sourceURL, newFeedImageHTTPClient())
	if err != nil {
		return feedImageError("图片下载失败: " + err.Error())
	}

	description := fmt.Sprintf("第 %d 张，共 %d 张；MIME: %s", args.ImageIndex+1, len(images), imageData.mime)
	if imageData.width > 0 && imageData.height > 0 {
		description += fmt.Sprintf("；尺寸: %d×%d", imageData.width, imageData.height)
	}

	encodedImage := base64.StdEncoding.EncodeToString(imageData.data)
	structuredContent := map[string]any{
		"imageIndex": args.ImageIndex + 1,
		"imageCount": len(images),
		"width":      imageData.width,
		"height":     imageData.height,
	}

	return &MCPToolResult{Content: []MCPContent{
		{Type: "text", Text: description},
		{Type: "image", MimeType: imageData.mime, Data: encodedImage},
	}, StructuredContent: structuredContent, Meta: map[string]any{
		feedImageWidgetResultMetaKey: map[string]any{
			// Content is zero-based: text description at 0, image at 1.
			"imageContentIndex": 1,
		},
	}}
}

// handleGetFeedImageForModel is intentionally separate from get_feed_image's
// display path. It reuses the same read-only download and validation logic but
// removes widget-only metadata, leaving only model-readable MCP content.
func (s *AppServer) handleGetFeedImageForModel(ctx context.Context, args FeedImageArgs) *MCPToolResult {
	return modelOnlyFeedImageResult(s.handleGetFeedImage(ctx, args))
}

func modelOnlyFeedImageResult(result *MCPToolResult) *MCPToolResult {
	if result == nil || result.IsError {
		return result
	}
	resultCopy := *result
	resultCopy.Meta = nil
	return &resultCopy
}

func feedImageError(message string) *MCPToolResult {
	return &MCPToolResult{
		Content: []MCPContent{{Type: "text", Text: "获取笔记图片失败: " + message}},
		IsError: true,
	}
}

func feedDetailImages(data any) ([]xiaohongshu.DetailImageInfo, bool) {
	switch detail := data.(type) {
	case *xiaohongshu.FeedDetailResponse:
		return detail.Note.ImageList, true
	case xiaohongshu.FeedDetailResponse:
		return detail.Note.ImageList, true
	default:
		return nil, false
	}
}

func newFeedImageHTTPClient() *http.Client {
	return &http.Client{
		Timeout: feedImageRequestTimeout,
		// Redirect 后的地址不再是详情返回的图片 URL，明确拒绝以保持来源边界。
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func downloadFeedImage(ctx context.Context, rawURL string, client *http.Client) (*downloadedFeedImage, error) {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("图片地址格式无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("图片地址仅允许 http 或 https")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("图片地址不能包含认证信息")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("图片请求构造失败")
	}
	req.Header.Set("Accept", "image/png,image/jpeg,image/gif,image/webp,*/*;q=0.5")
	req.Header.Set("Referer", "https://www.xiaohongshu.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("图片请求未完成")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("图片服务器返回 HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxFeedImageBytes {
		return nil, fmt.Errorf("图片超过 15MB 限制")
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取图片内容失败")
	}
	if int64(len(data)) > maxFeedImageBytes {
		return nil, fmt.Errorf("图片超过 15MB 限制")
	}

	kind, err := filetype.Match(data)
	if err != nil || !filetype.IsImage(data) {
		return nil, fmt.Errorf("下载内容不是真实图片")
	}
	mime := kind.MIME.Value
	switch mime {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
	default:
		return nil, fmt.Errorf("图片 MIME 类型不受支持: %s", mime)
	}

	width, height := 0, 0
	if config, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		width, height = config.Width, config.Height
	}

	return &downloadedFeedImage{data: data, mime: mime, width: width, height: height}, nil
}
