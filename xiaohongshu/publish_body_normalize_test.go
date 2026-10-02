// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import "testing"

func TestNormalizePublishBody(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"line endings and trim", "  第一行\r\n第二行\r第三行\n  ", "第一行 第二行 第三行"},
		{"consecutive unicode whitespace", "甲\u00a0\u3000\t乙", "甲 乙"},
		{"invisible editor separators", "甲\u200b乙\ufeff丙\u2060丁", "甲乙丙丁"},
		{"punctuation is meaningful", "“AI”，ＡＩ。", "“AI”，ＡＩ。"},
		{"emoji joiner stays", "👩‍💻", "👩‍💻"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizePublishBody(tc.input); got != tc.want {
				t.Errorf("normalization mismatch: got length %d, want length %d", len([]rune(got)), len([]rune(tc.want)))
			}
		})
	}
}

func TestFirstDifferentRune(t *testing.T) {
	if got := firstDifferentRune("甲乙丙", "甲乙丁"); got != 2 {
		t.Fatalf("got %d", got)
	}
	if got := firstDifferentRune("甲乙", "甲乙丙"); got != 2 {
		t.Fatalf("got %d", got)
	}
	if got := firstDifferentRune("甲乙", "甲乙"); got != -1 {
		t.Fatalf("got %d", got)
	}
}
