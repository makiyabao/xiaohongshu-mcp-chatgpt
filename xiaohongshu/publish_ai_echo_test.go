// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// Model the observed web dropdown, not a hypothetical native radio. Fixtures
// have no account, Cookie or user text and never call a platform endpoint.
func aiWebEchoFixture(script string) string {
	return `<div class="d-select" id="declaration">
 <div class="d-select-main" id="entry"><div class="d-select-content">
  <div class="d-select-description" id="description"></div>
  <div class="d-select-placeholder" id="placeholder">添加内容类型声明</div>
 </div></div></div>
 <div class="d-options-wrapper" id="panel" hidden><div class="d-options">
  <div class="d-option d-option-content" id="ai"><span>笔记含AI合成内容</span></div>
  <div class="d-option d-option-content" onclick="window.wrongClicks++">无需声明</div>
 </div></div>
 <span class="permission-info-declaration"><span class="permission-info-text" id="summary"></span></span>
 <label>原创声明<input id="original" type="checkbox" onclick="window.wrongClicks++"></label>
 <button id="publish" onclick="window.published=1">发布</button>
 <script>window.entryClicks=0;window.optionClicks=0;window.wrongClicks=0;
 entry.onclick=()=>{window.entryClicks++;panel.hidden=!panel.hidden};
 window.setDescription=()=>{description.textContent='笔记含AI合成内容';placeholder.hidden=true};
 window.setSummary=()=>{summary.textContent='笔记含AI合成内容'};
 ai.onclick=()=>{window.optionClicks++;panel.hidden=true;` + script + `};</script>`
}

func TestOfflineAIWebDualEchoConfirmation(t *testing.T) {
	cases := []struct {
		name, script string
		pass         bool
	}{
		{"both_exact", `setDescription();setSummary()`, true},
		{"description_only", `setDescription()`, false},
		{"summary_only", `setSummary()`, false},
		{"wrong_option", `description.textContent='无需声明';placeholder.hidden=true;summary.textContent='无需声明'`, false},
		{"delayed_echoes", `setTimeout(setDescription,150);setTimeout(setSummary,350)`, true},
		{"closed_without_echoes", ``, false},
		{"svg_only", `ai.insertAdjacentHTML('beforeend','<svg><path/></svg>')`, false},
		{"native_aria_classes_only", `ai.setAttribute('aria-checked','true');ai.setAttribute('aria-selected','true');ai.classList.add('selected');ai.insertAdjacentHTML('beforeend','<input type="checkbox" checked>')`, false},
		{"wrong_summary_with_correct_description", `setDescription();summary.textContent='无需声明'`, false},
		{"wrong_description_with_correct_summary", `description.textContent='无需声明';placeholder.hidden=true;setSummary()`, false},
		{"duplicate_summary", `setDescription();setSummary();summary.parentElement.insertAdjacentHTML('beforeend','<span class="permission-info-text">笔记含AI合成内容</span>')`, false},
		{"internal_whitespace_not_exact", `description.textContent='笔记含 AI 合成内容';placeholder.hidden=true;setSummary()`, false},
		{"echoes_never_simultaneous", `setDescription();setTimeout(()=>{description.textContent='无需声明';setSummary()},180)`, false},
		{"original_only", `original.checked=true`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, done := offlinePage(t, aiWebEchoFixture(tc.script))
			defer done()
			ui := &rodAIDeclarationUI{page: page}
			err := ensureAIGeneratedDeclaration(ui, true)
			if tc.pass {
				if err != nil {
					t.Fatal(err)
				}
				if ui.diagnostic.After == nil || !ui.diagnostic.After.confirmed() {
					t.Fatal("success without both exact echoes in one snapshot")
				}
			} else {
				requireAICode(t, err, "ai_option_not_selected_after_click")
			}
			result, evalErr := page.Eval(`() => JSON.stringify({entryClicks,optionClicks,wrongClicks,published:window.published||0,original:original.checked})`)
			if evalErr != nil {
				t.Fatal(evalErr)
			}
			var got struct {
				EntryClicks, OptionClicks, WrongClicks, Published int
				Original                                          bool
			}
			if err := json.Unmarshal([]byte(result.Value.Str()), &got); err != nil {
				t.Fatal(err)
			}
			if got.EntryClicks != 1 || got.OptionClicks != 1 || got.WrongClicks != 0 || got.Published != 0 {
				t.Fatalf("unexpected interaction counters: %+v", got)
			}
			if tc.name != "original_only" && got.Original {
				t.Fatal("AI declaration changed original declaration")
			}
			if tc.name == "svg_only" && ui.diagnostic.OptionSVGDelta != 1 {
				t.Fatal("SVG diagnostic missing; icon must not establish success")
			}
		})
	}
}

func TestOfflineAIWebAlreadyConfirmedDoesNotToggle(t *testing.T) {
	page, done := offlinePage(t, aiWebEchoFixture(``)+`<script>setDescription();setSummary()</script>`)
	defer done()
	ui := &rodAIDeclarationUI{page: page}
	if err := ensureAIGeneratedDeclaration(ui, true); err != nil {
		t.Fatal(err)
	}
	if ui.diagnostic.EntryClickAttempts != 0 || ui.diagnostic.OptionClickAttempted {
		t.Fatal("already confirmed state must be idempotent")
	}
}

func TestOfflineAIWebConfirmationCancellationStops(t *testing.T) {
	page, done := offlinePage(t, aiWebEchoFixture(``))
	defer done()
	ctx, cancel := context.WithCancel(page.GetContext())
	cancel()
	ui := &rodAIDeclarationUI{page: page.Context(ctx)}
	start := time.Now()
	requireAICode(t, ui.waitConfirmed(), "ai_option_not_selected_after_click")
	if time.Since(start) > time.Second {
		t.Fatal("confirmation ignored cancellation")
	}
}
