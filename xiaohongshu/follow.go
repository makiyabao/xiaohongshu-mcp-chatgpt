// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

const followButtonSelector = "button.follow-button"

type FollowResult struct {
	UserID        string `json:"user_id"`
	Nickname      string `json:"nickname,omitempty"`
	PreviousState string `json:"previous_state"`
	CurrentState  string `json:"current_state"`
	Success       bool   `json:"success"`
	Message       string `json:"message"`
}

type FollowAction struct{ page *rod.Page }

func NewFollowAction(page *rod.Page) *FollowAction { return &FollowAction{page: page} }

func (a *FollowAction) SetFollow(ctx context.Context, userID, xsecToken string, wantFollow, override bool, protected map[string]struct{}) (*FollowResult, error) {
	page := a.page.Context(ctx).Timeout(60 * time.Second)
	page.MustNavigate(makeUserProfileURL(userID, xsecToken, TabNotes))
	page.MustWaitStable()

	nickname, current, err := followPageState(page)
	if err != nil { return nil, err }
	result := &FollowResult{UserID: userID, Nickname: nickname, PreviousState: current, CurrentState: current}
	if !wantFollow {
		if _, ok := protected[userID]; ok && !override {
			result.Message = "该用户受 protected_user_ids 保护；需要 override=true 才能取消关注"
			return result, nil
		}
		if current == "not_following" {
			result.Success, result.Message = true, "当前未关注，无需取消"
			return result, nil
		}
	} else if current == "following" {
		result.Success, result.Message = true, "当前已关注，无需重复关注"
		return result, nil
	}

	button, err := page.Element(followButtonSelector)
	if err != nil { return nil, fmt.Errorf("未找到关注按钮，无法在写操作前确认状态: %w", err) }
	humanize.Delay(ctx, humanize.BeforeClick)
	if err := humanize.Click(button); err != nil { return nil, fmt.Errorf("点击%s失败: %w", map[bool]string{true:"关注", false:"取消关注"}[wantFollow], err) }
	humanize.Delay(ctx, humanize.AfterInteract)

	deadline := time.Now().Add(4 * time.Second)
	for {
		_, state, stateErr := followPageState(page)
		if stateErr != nil { return nil, stateErr }
		result.CurrentState = state
		if (wantFollow && state == "following") || (!wantFollow && state == "not_following") {
			result.Success = true
			result.Message = map[bool]string{true:"关注成功", false:"取消关注成功"}[wantFollow]
			return result, nil
		}
		if time.Now().After(deadline) { return nil, fmt.Errorf("点击后状态未变为预期，已停止且未重试") }
		time.Sleep(500 * time.Millisecond)
	}
}

func followPageState(page *rod.Page) (string, string, error) {
	raw := page.MustEval(`() => {
 const text = (document.body && document.body.innerText || '');
 const risk = ['验证码','安全验证','操作频繁','访问异常','滑块'].find(x => text.includes(x));
 if (risk) return JSON.stringify({risk});
 const b = document.querySelector('button.follow-button');
 const u = window.__INITIAL_STATE__ && window.__INITIAL_STATE__.user;
 const d = u && u.userPageData && (u.userPageData.value || u.userPageData._value || u.userPageData);
 return JSON.stringify({text: b && (b.innerText || b.textContent || '').trim(), nickname: d && d.basicInfo && d.basicInfo.nickname});
}`).String()
	var data struct { Text, Nickname, Risk string }
	if err := json.Unmarshal([]byte(raw), &data); err != nil { return "", "", fmt.Errorf("读取关注状态失败: %w", err) }
	if data.Risk != "" { return "", "", fmt.Errorf("检测到%s，已停止操作", data.Risk) }
	switch strings.ReplaceAll(data.Text, " ", "") {
	case "关注": return data.Nickname, "not_following", nil
	case "已关注", "互相关注", "相互关注", "已互关": return data.Nickname, "following", nil
	default: return data.Nickname, "unknown", fmt.Errorf("无法确认关注状态（按钮文字：%q），已停止操作", data.Text)
	}
}
