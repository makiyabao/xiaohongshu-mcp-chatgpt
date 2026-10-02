// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

const (
	feedLinkRedirectTimeout = 10 * time.Second
	feedLinkPageTimeout     = 35 * time.Second
	feedLinkMaxRedirects    = 5
)

var sharedURLPattern = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^\s<>"'“”]+`)

// ResolvedFeedLink contains only the note parameters needed by existing
// read-only tools. It intentionally excludes browser state, cookies, and any
// other login data.
type ResolvedFeedLink struct {
	ResolvedURL string `json:"resolved_url"`
	FeedID      string `json:"feed_id"`
	XsecToken   string `json:"xsec_token"`
	Title       string `json:"title,omitempty"`
	Author      string `json:"author,omitempty"`
	ImageCount  int    `json:"image_count,omitempty"`
}

type resolvedPageState struct {
	Status   string                  `json:"status"`
	Note     xiaohongshu.FeedDetail  `json:"note"`
	Comments xiaohongshu.CommentList `json:"comments"`
}

func (s *XiaohongshuService) ResolveFeedLink(ctx context.Context, input string) (*ResolvedFeedLink, error) {
	resolved, err := s.resolveFeedPage(ctx, input)
	if err != nil {
		return nil, err
	}
	return resolved.Link, nil
}

type resolvedFeedPage struct {
	Link  *ResolvedFeedLink
	State resolvedPageState
}

// ReadFeedByURL resolves a share link and reads the existing page state once.
// It deliberately does not call GetFeedDetail again, which would open a second
// browser page for the same note.
func (s *XiaohongshuService) ReadFeedByURL(ctx context.Context, input string) (*FeedByURLResult, error) {
	resolved, err := s.resolveFeedPage(ctx, input)
	if err != nil {
		return nil, err
	}

	comments := limitInitialComments(resolved.State.Comments)

	note := resolved.State.Note
	images := make([]FeedImageMetadata, 0, len(note.ImageList))
	for index, image := range note.ImageList {
		images = append(images, FeedImageMetadata{
			Index:     index,
			Width:     image.Width,
			Height:    image.Height,
			LivePhoto: image.LivePhoto,
		})
	}

	return &FeedByURLResult{
		ResolvedFeedLink: *resolved.Link,
		Desc:             note.Desc,
		NoteType:         note.Type,
		Images:           images,
		Interaction:      note.InteractInfo,
		Video:            summarizeFeedVideo(note.Video),
		Comments:         comments.List,
		CommentsHasMore:  comments.HasMore,
		CommentsCursor:   comments.Cursor,
	}, nil
}

func limitInitialComments(comments xiaohongshu.CommentList) xiaohongshu.CommentList {
	if len(comments.List) <= 10 {
		return comments
	}
	comments.List = append([]xiaohongshu.Comment(nil), comments.List[:10]...)
	comments.HasMore = true
	return comments
}

// FeedByURLResult is intentionally compact: it has the same readable note
// details as get_feed_detail without image bytes or full video streams.
type FeedByURLResult struct {
	ResolvedFeedLink
	Desc            string                   `json:"desc,omitempty"`
	NoteType        string                   `json:"note_type,omitempty"`
	Images          []FeedImageMetadata      `json:"image_list,omitempty"`
	Interaction     xiaohongshu.InteractInfo `json:"interaction_data"`
	Video           *FeedVideoMetadata       `json:"video,omitempty"`
	Comments        []xiaohongshu.Comment    `json:"comments"`
	CommentsHasMore bool                     `json:"comments_has_more"`
	CommentsCursor  string                   `json:"comments_cursor,omitempty"`
}

type FeedImageMetadata struct {
	Index     int  `json:"index"`
	Width     int  `json:"width,omitempty"`
	Height    int  `json:"height,omitempty"`
	LivePhoto bool `json:"live_photo,omitempty"`
}

type FeedVideoMetadata struct {
	Duration          int      `json:"duration,omitempty"`
	SubtitleLanguages []string `json:"subtitle_languages,omitempty"`
}

func summarizeFeedVideo(video *xiaohongshu.VideoDetail) *FeedVideoMetadata {
	if video == nil {
		return nil
	}
	metadata := &FeedVideoMetadata{Duration: video.Capa.Duration}
	for language := range video.Subtitles {
		metadata.SubtitleLanguages = append(metadata.SubtitleLanguages, language)
	}
	sort.Strings(metadata.SubtitleLanguages)
	return metadata
}

func (s *XiaohongshuService) resolveFeedPage(ctx context.Context, input string) (*resolvedFeedPage, error) {
	inputURL, err := extractXiaohongshuURL(input)
	if err != nil {
		return nil, err
	}

	redirectedURL, err := resolveXiaohongshuRedirect(ctx, inputURL)
	if err != nil {
		return nil, err
	}
	if !isXiaohongshuHost(redirectedURL.Hostname()) {
		return nil, errors.New("短链跳转到了非小红书域名")
	}

	pageState, pageURL, err := s.readFeedStateFromURL(ctx, redirectedURL)
	if err != nil {
		return nil, err
	}

	feedID := pageState.Note.NoteID
	if feedID == "" {
		feedID = feedIDFromXiaohongshuURL(pageURL)
	}
	if feedID == "" {
		return nil, errors.New("无法解析 feed_id")
	}

	xsecToken := pageState.Note.XsecToken
	if xsecToken == "" {
		xsecToken = pageURL.Query().Get("xsec_token")
	}
	if xsecToken == "" {
		xsecToken = redirectedURL.Query().Get("xsec_token")
	}
	if xsecToken == "" {
		xsecToken = inputURL.Query().Get("xsec_token")
	}
	if xsecToken == "" {
		return nil, errors.New("无法取得后续读取所需 token")
	}

	author := pageState.Note.User.Nickname
	if author == "" {
		author = pageState.Note.User.NickName
	}

	return &resolvedFeedPage{Link: &ResolvedFeedLink{
		ResolvedURL: redactXiaohongshuURL(pageURL),
		FeedID:      feedID,
		XsecToken:   xsecToken,
		Title:       pageState.Note.Title,
		Author:      author,
		ImageCount:  len(pageState.Note.ImageList),
	}, State: pageState}, nil
}

func extractXiaohongshuURL(input string) (*url.URL, error) {
	matches := sharedURLPattern.FindAllString(input, -1)
	if len(matches) == 0 {
		return nil, errors.New("无法从输入中找到 URL")
	}

	var firstErr error
	for _, match := range matches {
		raw := strings.TrimRight(match, ".,;:!?)]}，。！？；：、】【）》」』")
		parsed, err := url.ParseRequestURI(raw)
		if err != nil {
			if firstErr == nil {
				firstErr = errors.New("无法解析分享 URL")
			}
			continue
		}
		if err := validateXiaohongshuURL(parsed); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		return parsed, nil
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return nil, errors.New("非小红书 URL")
}

func resolveXiaohongshuRedirect(ctx context.Context, inputURL *url.URL) (*url.URL, error) {
	if err := validateXiaohongshuURL(inputURL); err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout: feedLinkRedirectTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= feedLinkMaxRedirects {
				return errors.New("短链重定向次数过多")
			}
			return validateXiaohongshuURL(req.URL)
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, inputURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("创建短链请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; XiaohongshuMCP/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, classifyFeedLinkRequestError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("短链或笔记页面不可用: HTTP %d", resp.StatusCode)
	}
	if err := validateXiaohongshuURL(resp.Request.URL); err != nil {
		return nil, err
	}
	return resp.Request.URL, nil
}

func (s *XiaohongshuService) readFeedStateFromURL(ctx context.Context, resolvedURL *url.URL) (resolvedPageState, *url.URL, error) {
	var empty resolvedPageState
	b := newBrowser()
	defer b.Close()

	page := b.NewPage().Context(ctx).Timeout(feedLinkPageTimeout)
	defer page.Close()
	if err := page.Navigate(resolvedURL.String()); err != nil {
		return empty, nil, fmt.Errorf("打开笔记页面失败: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		return empty, nil, fmt.Errorf("等待笔记页面失败: %w", err)
	}
	if err := page.WaitStable(500 * time.Millisecond); err != nil {
		logrus.Debugf("解析分享链接时页面未完全稳定: %v", err)
	}

	location, err := page.Eval(`() => location.href`)
	if err != nil {
		return empty, nil, fmt.Errorf("读取笔记最终地址失败: %w", err)
	}
	pageURL, err := url.Parse(location.Value.String())
	if err != nil || validateXiaohongshuURL(pageURL) != nil {
		return empty, nil, errors.New("笔记页面跳转到了非小红书域名")
	}

	stateResult, err := page.Eval(`() => {
  const text = document.body ? document.body.innerText || "" : "";
  if (/安全验证|人机验证|滑动验证|验证码/.test(text)) return JSON.stringify({status:"risk"});
  if (/当前笔记暂时无法浏览|该内容因违规已被删除|该笔记已被删除|内容不存在|笔记不存在|私密笔记|仅作者可见|因用户设置，你无法查看|因违规无法查看/.test(text)) return JSON.stringify({status:"unavailable"});
  const details = window.__INITIAL_STATE__?.note?.noteDetailMap;
  const entry = details && Object.values(details).find((item) => item && item.note);
  if (!entry || !entry.note) {
    if (/登录/.test(text)) return JSON.stringify({status:"login"});
    return JSON.stringify({status:"missing"});
  }
  return JSON.stringify({status:"ok", note:entry.note, comments:entry.comments || {}});
}`)
	if err != nil {
		return empty, nil, fmt.Errorf("读取笔记页面状态失败: %w", err)
	}

	var state resolvedPageState
	if err := json.Unmarshal([]byte(stateResult.Value.String()), &state); err != nil {
		return empty, nil, fmt.Errorf("解析笔记页面状态失败: %w", err)
	}
	switch state.Status {
	case "ok":
		return state, pageURL, nil
	case "risk":
		return empty, nil, errors.New("小红书触发风险验证，已停止解析")
	case "unavailable":
		return empty, nil, errors.New("笔记不存在、已删除或无访问权限")
	case "login":
		return empty, nil, errors.New("登录失效，无法读取笔记")
	default:
		return empty, nil, errors.New("无法从笔记页面解析 feed_id")
	}
}

func validateXiaohongshuURL(parsed *url.URL) error {
	if parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("只允许 http 或 https 小红书 URL")
	}
	if parsed.User != nil || parsed.Port() != "" {
		return errors.New("小红书 URL 不能包含用户信息或自定义端口")
	}
	if !isXiaohongshuHost(parsed.Hostname()) {
		return errors.New("非小红书 URL")
	}
	return nil
}

func isXiaohongshuHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" || host == "localhost" || net.ParseIP(host) != nil {
		return false
	}
	return host == "xiaohongshu.com" || strings.HasSuffix(host, ".xiaohongshu.com") ||
		host == "xhslink.cn" || strings.HasSuffix(host, ".xhslink.cn")
}

func feedIDFromXiaohongshuURL(parsed *url.URL) string {
	if parsed == nil || !isXiaohongshuHost(parsed.Hostname()) {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "explore" {
		return parts[1]
	}
	if len(parts) >= 3 && parts[0] == "discovery" && parts[1] == "item" {
		return parts[2]
	}
	return ""
}

func redactXiaohongshuURL(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	copy := *parsed
	copy.User = nil
	copy.RawQuery = ""
	copy.ForceQuery = false
	copy.Fragment = ""
	return copy.String()
}

func classifyFeedLinkRequestError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if strings.Contains(urlErr.Err.Error(), "重定向次数过多") {
			return errors.New("短链重定向次数过多")
		}
		if strings.Contains(urlErr.Err.Error(), "非小红书 URL") {
			return errors.New("短链跳转到了非小红书域名")
		}
	}
	return fmt.Errorf("解析短链失败: %w", err)
}

func (s *AppServer) handleResolveXHSLink(ctx context.Context, input string) *MCPToolResult {
	logrus.Info("MCP: 解析小红书分享链接")
	result, err := s.xiaohongshuService.ResolveFeedLink(ctx, input)
	if err != nil {
		return &MCPToolResult{Content: []MCPContent{{Type: "text", Text: "解析小红书分享链接失败: " + err.Error()}}, IsError: true}
	}
	return marshalMCPResult(result, "解析小红书分享链接")
}

func (s *AppServer) handleReadFeedByURL(ctx context.Context, input string) *MCPToolResult {
	logrus.Info("MCP: 通过分享链接读取笔记")
	result, err := s.xiaohongshuService.ReadFeedByURL(ctx, input)
	if err != nil {
		return &MCPToolResult{Content: []MCPContent{{Type: "text", Text: "通过分享链接读取笔记失败: " + err.Error()}}, IsError: true}
	}
	return marshalMCPResult(result, "通过分享链接读取笔记")
}
