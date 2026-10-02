// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package main

import (
	"net/url"
	"testing"

	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

func TestExtractXiaohongshuURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantURL string
		wantErr string
	}{
		{"share copy", "标题 xxx https://xhslink.cn/abcd 复制后打开小红书查看笔记", "https://xhslink.cn/abcd", ""},
		{"explore", "https://www.xiaohongshu.com/explore/abc123?xsec_token=token", "https://www.xiaohongshu.com/explore/abc123?xsec_token=token", ""},
		{"discovery", "https://www.xiaohongshu.com/discovery/item/abc123", "https://www.xiaohongshu.com/discovery/item/abc123", ""},
		{"no URL", "没有链接", "", "无法从输入中找到 URL"},
		{"non xhs", "https://example.com/post", "", "非小红书 URL"},
		{"localhost", "http://localhost:18060/mcp", "", "小红书 URL 不能包含用户信息或自定义端口"},
		{"private IP", "http://127.0.0.1/test", "", "非小红书 URL"},
		{"metadata", "http://169.254.169.254/latest/meta-data", "", "非小红书 URL"},
		{"file", "file:///etc/passwd", "", "只允许 http 或 https 小红书 URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractXiaohongshuURL(tt.input)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != tt.wantURL {
				t.Fatalf("URL = %q, want %q", got, tt.wantURL)
			}
		})
	}
}

func TestFeedIDFromXiaohongshuURL(t *testing.T) {
	for _, tt := range []struct {
		raw  string
		want string
	}{
		{"https://www.xiaohongshu.com/explore/note1?xsec_token=t", "note1"},
		{"https://www.xiaohongshu.com/discovery/item/note2", "note2"},
		{"https://www.xiaohongshu.com/user/profile/user1", ""},
	} {
		parsed, err := url.Parse(tt.raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := feedIDFromXiaohongshuURL(parsed); got != tt.want {
			t.Fatalf("feedIDFromXiaohongshuURL(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestRedactXiaohongshuURL(t *testing.T) {
	parsed, err := url.Parse("https://www.xiaohongshu.com/explore/note1?xsec_token=secret&xsec_source=pc#fragment")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := redactXiaohongshuURL(parsed), "https://www.xiaohongshu.com/explore/note1"; got != want {
		t.Fatalf("redacted URL = %q, want %q", got, want)
	}
}

func TestLimitInitialCommentsKeepsFirstTen(t *testing.T) {
	comments := xiaohongshu.CommentList{List: make([]xiaohongshu.Comment, 12)}
	for index := range comments.List {
		comments.List[index].ID = string(rune('a' + index))
	}

	got := limitInitialComments(comments)
	if len(got.List) != 10 {
		t.Fatalf("comment count = %d, want 10", len(got.List))
	}
	if !got.HasMore {
		t.Fatal("comments beyond the initial ten must set has_more")
	}
	if got.List[0].ID != "a" || got.List[9].ID != "j" {
		t.Fatalf("expected first ten comments to remain in order, got first=%q tenth=%q", got.List[0].ID, got.List[9].ID)
	}
}

func TestSummarizeFeedVideoKeepsOnlyBasicMetadata(t *testing.T) {
	video := &xiaohongshu.VideoDetail{
		Capa: xiaohongshu.VideoCapability{Duration: 42},
		Subtitles: map[string][]xiaohongshu.VideoSubtitle{
			"zh-CN": []xiaohongshu.VideoSubtitle{{Language: "zh-CN"}},
			"en":    []xiaohongshu.VideoSubtitle{{Language: "en"}},
		},
	}

	got := summarizeFeedVideo(video)
	if got == nil || got.Duration != 42 {
		t.Fatalf("video summary = %#v, want duration 42", got)
	}
	if len(got.SubtitleLanguages) != 2 || got.SubtitleLanguages[0] != "en" || got.SubtitleLanguages[1] != "zh-CN" {
		t.Fatalf("subtitle languages = %#v, want sorted language names only", got.SubtitleLanguages)
	}
	if summarizeFeedVideo(nil) != nil {
		t.Fatal("nil video must remain nil")
	}
}
