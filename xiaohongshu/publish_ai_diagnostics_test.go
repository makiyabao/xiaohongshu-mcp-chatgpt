// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
)

func requireAICode(t *testing.T, err error, code string) *aiDeclarationError {
	t.Helper()
	var failure *aiDeclarationError
	if !errors.As(err, &failure) || failure.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
	return failure
}

func TestAIDeclarationFixedCodesSurviveBrowserStep(t *testing.T) {
	for _, code := range []string{"declaration_entry_not_found", "declaration_entry_ambiguous", "declaration_entry_click_failed", "ai_option_not_found", "ai_option_ambiguous", "ai_option_click_failed", "ai_option_not_selected_after_click", "conflicting_selection_detected", "ai_state_read_failed", "settings_expand_failed", "declaration_entry_not_found_after_expand", "declaration_entry_not_clickable"} {
		t.Run(code, func(t *testing.T) {
			cause := aiDeclarationFailure(code, errors.New(`{"token":"SYNTHETIC_PRIVATE","Cookie":"SYNTHETIC_PRIVATE"}`))
			err := &BrowserStepError{Cause: cause}
			if !strings.Contains(err.Error(), code) || strings.Contains(err.Error(), "SYNTHETIC_PRIVATE") {
				t.Fatal("lost code or leaked underlying cause")
			}
		})
	}
	// Arbitrary errors resembling a code still go through the generic allowlist.
	err := &BrowserStepError{Cause: errors.New("ai_option_not_found Bearer SYNTHETIC_PRIVATE")}
	if strings.Contains(err.Error(), "SYNTHETIC_PRIVATE") {
		t.Fatal("raw text bypassed allowlist")
	}
}

func TestAIDeclarationSnapshotRejectsFreeFormAttributes(t *testing.T) {
	s := aiDeclarationSnapshot{Options: []aiControlDiagnostic{{Tag: "SYNTHETIC_PRIVATE", Role: "SYNTHETIC_PRIVATE", Type: "SYNTHETIC_PRIVATE", AriaChecked: "SYNTHETIC_PRIVATE"}}, Settings: []aiControlDiagnostic{{Tag: "SYNTHETIC_PRIVATE", Role: "SYNTHETIC_PRIVATE", Type: "SYNTHETIC_PRIVATE", AriaChecked: "SYNTHETIC_PRIVATE"}}}
	s.sanitize()
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), "SYNTHETIC_PRIVATE") {
		t.Fatal("attribute leak")
	}
}

func TestOfflineAIStructuralControls(t *testing.T) {
	cases := []struct{ name, html string }{
		{"wrapping_label_radio", `<label><input id="ai" type="radio" name="declaration"><span>含有 AI 合成内容</span></label>`},
		{"wrapping_label_checkbox", `<label><input id="ai" type="checkbox"><span>含有 AI 合成内容</span></label>`},
		{"external_label_hidden_radio", `<input id="ai" type="radio" style="display:none"><label for="ai"><span>含有 AI 合成内容</span></label>`},
		{"role_radio", `<div id="ai" role="radio" aria-checked="false" onclick="this.setAttribute('aria-checked','true')"><span>含有 AI 合成内容</span></div>`},
		{"role_checkbox", `<div id="ai" role="checkbox" aria-checked="false" onclick="this.setAttribute('aria-checked','true')">含有 AI 合成内容</div>`},
		{"role_wraps_native", `<label role="radio"><input id="ai" type="radio"><span>含有 AI 合成内容</span></label>`},
		{"aria_label", `<button id="ai" role="radio" aria-label="含有 AI 合成内容" aria-checked="false" onclick="this.setAttribute('aria-checked','true')">○</button>`},
		{"nested_text_same_control", `<div id="ai" role="radio" aria-checked="false" onclick="this.setAttribute('aria-checked','true')"><span>含有 AI 合成内容</span><span>含有 AI 合成内容</span></div>`},
		{"label_extra_description", `<label class="totally-new-class"><input id="ai" type="radio"><span>含有 AI 合成内容</span><small>合成信息说明</small></label>`},
		{"checked_native_idempotent", `<label><input id="ai" type="radio" checked onclick="throw Error('must not click')">含有 AI 合成内容</label>`},
		{"aria_selected", `<div id="ai" role="radio" aria-selected="false" onclick="this.setAttribute('aria-selected','true')">含有 AI 合成内容</div>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			initialEcho := ""
			if tc.name == "checked_native_idempotent" {
				initialEcho = "setAIEchoes();"
			}
			page, done := offlinePage(t, `<button><span>笔记内容声明</span><span id="description" class="d-select-description"></span></button>`+tc.html+`
<span class="permission-info-declaration"><span id="summary" class="permission-info-text"></span></span>
<button id="publish" onclick="window.published=1">发布</button><script>
window.setAIEchoes=()=>{description.textContent='笔记含AI合成内容';summary.textContent='笔记含AI合成内容'};
ai.addEventListener('click',setAIEchoes);`+initialEcho+`</script>`)
			defer done()
			ui := &rodAIDeclarationUI{page: page}
			if err := ensureAIGeneratedDeclaration(ui, true); err != nil {
				t.Fatal(err)
			}
			selected, err := ui.IsAIContentDeclared()
			if err != nil || !selected {
				t.Fatalf("not confirmed: %v", err)
			}
			if tc.name == "checked_native_idempotent" && ui.diagnostic.OptionClickAttempted {
				t.Fatal("clicked selected option")
			}
			if tc.name != "checked_native_idempotent" && !ui.diagnostic.SelectionChanged {
				t.Fatal("state transition missing")
			}
			assertNoVisibilityFixturePublish(t, page)
		})
	}
}

func TestOfflineAIAmbiguityNeverClicks(t *testing.T) {
	for _, tc := range []struct{ name, html, code string }{
		{"entries", `<button onclick="window.clicked=1">笔记内容声明</button><button onclick="window.clicked=1">笔记内容声明</button>`, "declaration_entry_ambiguous"},
		{"options", `<div role="radio" aria-checked="false" onclick="window.clicked=1">含有 AI 合成内容</div><div role="radio" aria-checked="false" onclick="window.clicked=1">含有 AI 合成内容</div>`, "ai_option_ambiguous"},
		{"different_radios", `<label><input type="radio" onclick="window.clicked=1">含有 AI 合成内容</label><label><input type="radio" onclick="window.clicked=1">含有 AI 合成内容</label>`, "ai_option_ambiguous"},
		{"one_label_two_controls", `<label><input type="radio" onclick="window.clicked=1"><input type="radio" onclick="window.clicked=1"><span>含有 AI 合成内容</span></label>`, "ai_option_ambiguous"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, done := offlinePage(t, tc.html)
			defer done()
			failure := requireAICode(t, ensureAIGeneratedDeclaration(&rodAIDeclarationUI{page: page}, true), tc.code)
			if failure.Diagnostic.Last == nil {
				t.Fatal("missing candidates")
			}
			res, err := page.Eval(`() => Boolean(window.clicked)`)
			if err != nil || res.Value.Bool() {
				t.Fatal("ambiguity triggered click")
			}
		})
	}
}

func TestOfflineAIDeclarationDelayedOptionBeyondThreeSeconds(t *testing.T) {
	page, done := offlinePage(t, `<button onclick="setTimeout(()=>panel.hidden=false,3200)"><span>笔记内容声明</span><span aria-hidden="true">▼</span><span id="description" class="d-select-description"></span></button><div id="panel" hidden><label><input type="radio" onclick="description.textContent='笔记含AI合成内容';summary.textContent='笔记含AI合成内容'">含有 AI 合成内容</label></div><span class="permission-info-declaration"><span id="summary" class="permission-info-text"></span></span>`)
	defer done()
	if err := BrowserStep(page, "publish.ai_declaration", "AI declaration", 20*time.Second, func(p *rod.Page) error { return ensureAIGeneratedDeclaration(&rodAIDeclarationUI{page: p}, true) }); err != nil {
		t.Fatal(err)
	}
}

func TestOfflineAIDeclarationDisabledClickFails(t *testing.T) {
	page, done := offlinePage(t, `<button>笔记内容声明</button><label><input type="radio" disabled>含有 AI 合成内容</label>`)
	defer done()
	requireAICode(t, ensureAIGeneratedDeclaration(&rodAIDeclarationUI{page: page}, true), "ai_option_click_failed")
}

func TestOfflineAIDeclarationDiagnosticHasOnlySafeStructure(t *testing.T) {
	// Linux browser socket names have a short path limit; use the isolated
	// runner's normal /tmp there, as in the existing diagnostic privacy tests.
	if runtime.GOOS == "windows" {
		t.Setenv("TMP", t.TempDir())
		t.Setenv("TEMP", os.Getenv("TMP"))
		t.Setenv("TMPDIR", os.Getenv("TMP"))
	}
	page, done := offlinePage(t, aiDeclarationFixture(`ai.onclick=()=>{window.aiClicks++};`)+`<title>SYNTHETIC_ACCOUNT</title><textarea>SYNTHETIC_BODY</textarea><input value="SYNTHETIC_TOKEN"><p>SYNTHETIC_PRIVATE_TEXT</p>`)
	defer done()
	err := BrowserStep(page, "publish.ai_declaration", "笔记内容声明", 20*time.Second, func(p *rod.Page) error { return ensureAIGeneratedDeclaration(&rodAIDeclarationUI{page: p}, true) })
	requireAICode(t, err, "ai_option_not_selected_after_click")
	var step *BrowserStepError
	if !errors.As(err, &step) {
		t.Fatal("missing step error")
	}
	b, readErr := os.ReadFile(step.Diagnostic)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, secret := range []string{"SYNTHETIC_ACCOUNT", "SYNTHETIC_BODY", "SYNTHETIC_TOKEN", "SYNTHETIC_PRIVATE_TEXT"} {
		if strings.Contains(string(b), secret) || strings.Contains(err.Error(), secret) {
			t.Fatal("private fixture data escaped")
		}
	}
	var d browserDiagnostic
	if err = json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	if d.ErrorCode != "ai_option_not_selected_after_click" || d.Step != "publish.ai_declaration" || d.AIDeclaration == nil {
		t.Fatal("lost typed diagnosis")
	}
	ai := d.AIDeclaration
	if !ai.OptionClickAttempted || !ai.OptionClickCompleted || ai.Before == nil || ai.After == nil || ai.SelectionChanged {
		t.Fatalf("missing/incorrect state transition: %+v", ai)
	}
	if ai.Before.AICandidates != 1 || ai.Before.EntryCandidates != 1 || ai.Before.Options[0].AriaChecked != "false" {
		t.Fatal("incorrect safe structure")
	}
	if d.Screenshot != "content_redacted_layout_only" {
		t.Fatal("screenshot privacy changed")
	}
}

func TestAIDeclarationCancellationStopsPolling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(aiDeclarationPause(ctx), context.Canceled) {
		t.Fatal("poll ignored cancellation")
	}
}
