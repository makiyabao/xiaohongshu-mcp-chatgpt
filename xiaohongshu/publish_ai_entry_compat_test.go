// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"encoding/json"
	"strings"
	"testing"
)

// All fixtures are served on loopback to a fresh browser profile. Nothing is
// uploaded or published; counters prove we never click headings/other options.
func TestOfflineAIEntryCompatibility(t *testing.T) {
	cases := []struct {
		name, markup, code                 string
		expands, entryClicks, optionClicks int
		unchanged                          bool
		mouseDownOnly                      bool
	}{
		{name: "note_declaration_button", markup: `<button data-entry>笔记内容声明</button>`, entryClicks: 1, optionClicks: 1},
		{name: "add_content_type_button", markup: `<button data-entry>添加内容类型声明</button>`, entryClicks: 1, optionClicks: 1},
		{name: "heading_separate_dropdown", markup: `<div><h3>笔记内容声明</h3><button data-entry aria-haspopup="listbox">▼</button></div>`, entryClicks: 1, optionClicks: 1},
		{name: "heading_icon_button", markup: `<div><h3>笔记内容声明</h3><button data-entry>▼</button></div>`, entryClicks: 1, optionClicks: 1},
		{name: "associated_dropdown", markup: `<h3 id="heading">笔记内容声明</h3><button data-entry aria-labelledby="heading" aria-expanded="false">▼</button>`, entryClicks: 1, optionClicks: 1},
		{name: "clickable_parent", markup: `<div data-entry role="button"><span>添加内容类型声明</span><i>▼</i></div>`, entryClicks: 1, optionClicks: 1},
		{name: "pointer_without_control_structure_is_not_enough", markup: `<div data-entry style="cursor:pointer"><span>添加内容类型声明</span></div>`, code: "declaration_entry_not_clickable"},
		{name: "web_dropdown_cursor_auto", markup: `<div class="d-select"><div data-entry class="d-select-main" style="cursor:auto"><div class="d-select-content"><div class="d-select-placeholder">添加内容类型声明</div></div><svg width="12" height="12"></svg></div></div>`, mouseDownOnly: true, entryClicks: 1, optionClicks: 1},
		{name: "heading_plus_web_dropdown", markup: `<section><h3>笔记内容声明</h3><div class="d-select"><div data-entry class="d-select-main"><div class="d-select-content"><div class="d-select-placeholder">请选择</div></div><svg width="12" height="12"></svg></div></div></section>`, entryClicks: 1, optionClicks: 1},
		{name: "two_labels_one_web_dropdown", markup: `<section><h3>笔记内容声明</h3><div class="d-select"><div data-entry class="d-select-main"><div class="d-select-content"><div class="d-select-placeholder">添加内容类型声明</div></div></div></div></section>`, entryClicks: 1, optionClicks: 1},
		{name: "associated_combobox_nested_button_deduplicated", markup: `<h3 id="heading">笔记内容声明</h3><div role="combobox" aria-labelledby="heading"><button data-entry aria-haspopup="listbox">添加内容类型声明</button></div>`, entryClicks: 1, optionClicks: 1},
		{name: "label_control_association", markup: `<label for="entry">笔记内容声明</label><button id="entry" data-entry>▼</button>`, entryClicks: 1, optionClicks: 1},
		{name: "two_labels_same_control", markup: `<div><h3>笔记内容声明</h3><button data-entry aria-haspopup="listbox"><span>添加内容类型声明</span></button></div>`, entryClicks: 1, optionClicks: 1},
		{name: "text_duplicates_one_control", markup: `<p>笔记内容声明</p><button data-entry>笔记内容声明</button><p>添加内容类型声明</p>`, entryClicks: 1, optionClicks: 1},
		{name: "collapsed_settings", markup: `<button id="settings" aria-controls="content" aria-expanded="false" onclick="window.expands++;this.setAttribute('aria-expanded','true');content.hidden=false">内容设置</button><div id="content" hidden><button data-entry>添加内容类型声明</button></div>`, expands: 1, entryClicks: 1, optionClicks: 1},
		{name: "collapsed_settings_without_aria", markup: `<div id="settings" role="button" onclick="window.expands++;content.hidden=false">内容设置</div><div id="content" hidden><button data-entry>添加内容类型声明</button></div>`, expands: 1, entryClicks: 1, optionClicks: 1},
		{name: "associated_accordion_without_role", markup: `<div onclick="window.expands++;content.hidden=false">内容设置</div><div id="content" hidden><button data-entry>添加内容类型声明</button></div>`, expands: 1, entryClicks: 1, optionClicks: 1},
		{name: "expanded_associated_accordion_without_role", markup: `<div onclick="window.wrongClicks++">内容设置</div><div><button data-entry>添加内容类型声明</button></div>`, entryClicks: 1, optionClicks: 1},
		{name: "collapsed_settings_web_dropdown", markup: `<button id="settings" aria-expanded="false" aria-controls="content" onclick="window.expands++;this.setAttribute('aria-expanded','true');content.hidden=false">内容设置</button><div id="content" hidden><div class="d-select"><div data-entry class="d-select-main"><div class="d-select-content"><div class="d-select-placeholder">添加内容类型声明</div></div></div></div></div>`, expands: 1, entryClicks: 1, optionClicks: 1},
		{name: "expanded_settings_not_toggled", markup: `<button id="settings" aria-expanded="true" aria-controls="content" onclick="window.wrongClicks++;content.hidden=true">内容设置</button><div id="content"><button data-entry>添加内容类型声明</button></div>`, entryClicks: 1, optionClicks: 1},
		{name: "expanded_settings_without_aria_not_toggled", markup: `<div role="button" onclick="window.wrongClicks++">内容设置</div><div><button data-entry>添加内容类型声明</button></div>`, entryClicks: 1, optionClicks: 1},
		{name: "delayed_entry_after_expand", markup: `<button id="settings" aria-expanded="false" onclick="window.expands++;this.setAttribute('aria-expanded','true');setTimeout(()=>content.hidden=false,300)">内容设置</button><div id="content" hidden><div><h3>笔记内容声明</h3><button data-entry role="combobox">▼</button></div></div>`, expands: 1, entryClicks: 1, optionClicks: 1},
		{name: "native_details_settings", markup: `<details><summary onclick="window.expands++">内容设置</summary><button data-entry>添加内容类型声明</button></details>`, expands: 1, entryClicks: 1, optionClicks: 1},
		{name: "missing_after_expand", markup: `<button aria-expanded="false" onclick="window.expands++;this.setAttribute('aria-expanded','true')">内容设置</button>`, expands: 1, code: "declaration_entry_not_found_after_expand"},
		{name: "expansion_no_effect", markup: `<button aria-expanded="false" onclick="window.expands++">内容设置</button>`, expands: 1, code: "settings_expand_failed"},
		{name: "settings_ambiguous", markup: `<button aria-expanded="false" onclick="window.expands++">内容设置</button><button aria-expanded="false" onclick="window.expands++">内容设置</button>`, code: "settings_expand_failed"},
		{name: "plain_heading_never_clicked", markup: `<h3>笔记内容声明</h3><button onclick="window.wrongClicks++">发布设置</button>`, code: "declaration_entry_not_clickable"},
		{name: "heading_with_click_handler_is_not_a_dropdown", markup: `<h3 style="cursor:pointer" onclick="window.wrongClicks++">笔记内容声明</h3>`, code: "declaration_entry_not_clickable"},
		{name: "class_without_dropdown_parts_rejected", markup: `<div class="d-select" onclick="window.wrongClicks++">添加内容类型声明</div>`, code: "declaration_entry_not_clickable"},
		{name: "unrelated_empty_button_never_clicked", markup: `<section><h3>笔记内容声明</h3><button onclick="window.wrongClicks++"><svg width="12" height="12"></svg></button></section>`, code: "declaration_entry_not_clickable"},
		{name: "hidden_associated_control_does_not_fall_back", markup: `<section><h3 id="heading">笔记内容声明</h3><button hidden data-entry aria-labelledby="heading">▼</button><button aria-haspopup="listbox" onclick="window.wrongClicks++">▼</button></section>`, code: "declaration_entry_not_clickable"},
		{name: "two_web_dropdowns_remain_ambiguous", markup: `<section><h3>笔记内容声明</h3><div class="d-select"><div data-entry class="d-select-main"><div class="d-select-content"><div class="d-select-placeholder">请选择</div></div></div></div><div class="d-select"><div data-entry class="d-select-main"><div class="d-select-content"><div class="d-select-placeholder">请选择</div></div></div></div></section>`, code: "declaration_entry_ambiguous"},
		{name: "entry_disabled", markup: `<button data-entry disabled>笔记内容声明</button>`, code: "declaration_entry_not_clickable"},
		{name: "distinct_entries_ambiguous", markup: `<button data-entry>笔记内容声明</button><button data-entry>添加内容类型声明</button>`, code: "declaration_entry_ambiguous"},
		{name: "two_adjacent_dropdowns_ambiguous", markup: `<div><h3>笔记内容声明</h3><button data-entry aria-haspopup="listbox">▼</button><button data-entry role="combobox">▼</button></div>`, code: "declaration_entry_ambiguous"},
		{name: "selected_state_not_confirmed", markup: `<button data-entry>添加内容类型声明</button>`, entryClicks: 1, optionClicks: 1, unchanged: true, code: "ai_option_not_selected_after_click"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			change := `ai.setAttribute('aria-checked','true');none.setAttribute('aria-checked','false');document.querySelectorAll('.d-select-description').forEach(e=>e.textContent='笔记含AI合成内容');summary.textContent='笔记含AI合成内容';`
			if tc.unchanged {
				change = ""
			}
			entryEvent := "click"
			if tc.mouseDownOnly {
				entryEvent = "mousedown"
			}
			page, done := offlinePage(t, tc.markup+`<div id="panel" hidden>
<div id="none" role="radio" aria-checked="true" onclick="window.wrongClicks++">无需声明</div>
<div id="ai" role="radio" aria-checked="false">含有 AI 合成内容</div></div>
<label>原创声明<input type="checkbox" onclick="window.wrongClicks++"></label>
<span class="permission-info-declaration"><span id="summary" class="permission-info-text"></span></span>
<button id="publish" onclick="window.published=1">发布</button><script>
window.expands=0;window.entryClicks=0;window.optionClicks=0;window.wrongClicks=0;
document.querySelectorAll('[data-entry]').forEach(e=>{const echo=document.createElement('span');echo.className='d-select-description';(e.closest('.d-select')?.querySelector('.d-select-content')||e).append(echo);e.addEventListener('`+entryEvent+`',()=>{window.entryClicks++;panel.hidden=false})});
ai.onclick=()=>{window.optionClicks++;`+change+`};</script>`)
			defer done()
			ui := &rodAIDeclarationUI{page: page}
			err := ensureAIGeneratedDeclaration(ui, true)
			if tc.code != "" {
				requireAICode(t, err, tc.code)
			} else if err != nil {
				t.Fatal(err)
			}
			res, evalErr := page.Eval(`() => JSON.stringify({expands:window.expands,entryClicks:window.entryClicks,optionClicks:window.optionClicks,wrongClicks:window.wrongClicks,selected:ai.getAttribute('aria-checked')==='true'})`)
			if evalErr != nil {
				t.Fatal(evalErr)
			}
			var got struct {
				Expands, EntryClicks, OptionClicks, WrongClicks int
				Selected                                        bool
			}
			if err := json.Unmarshal([]byte(res.Value.Str()), &got); err != nil {
				t.Fatal(err)
			}
			if got.Expands != tc.expands || got.EntryClicks != tc.entryClicks || got.OptionClicks != tc.optionClicks || got.WrongClicks != 0 {
				t.Fatalf("unsafe or repeated interaction: %+v", got)
			}
			if tc.code == "" && !got.Selected {
				t.Fatal("AI selection not confirmed")
			}
			if tc.expands == 1 && !ui.diagnostic.SettingsExpandAttempted {
				t.Fatal("missing expansion diagnostic")
			}
			if tc.expands == 1 && tc.code != "settings_expand_failed" && !ui.diagnostic.SettingsExpandCompleted {
				t.Fatal("missing expansion readback")
			}
			if tc.expands == 1 && ui.diagnostic.SettingsExpandCompleted {
				before, after := ui.diagnostic.SettingsBefore, ui.diagnostic.SettingsAfter
				if before == nil || after == nil || before.Settings[0].Expanded == nil || *before.Settings[0].Expanded {
					t.Fatal("missing collapsed state before expansion")
				}
				if after.Settings[0].Expanded == nil || !*after.Settings[0].Expanded {
					t.Fatal("missing expanded state after expansion")
				}
			}
			assertNoVisibilityFixturePublish(t, page)
		})
	}
}

func TestOfflineAIExpansionDiagnosticIsAllowlisted(t *testing.T) {
	page, done := offlinePage(t, `<title>SYNTHETIC_ACCOUNT</title><textarea>SYNTHETIC_BODY</textarea>`)
	defer done()
	untrusted := &aiDeclarationSnapshot{Settings: []aiControlDiagnostic{{Tag: "SYNTHETIC_TOKEN", Role: "SYNTHETIC_TOKEN", Type: "SYNTHETIC_TOKEN", AriaChecked: "SYNTHETIC_TOKEN"}}}
	failure := &aiDeclarationError{Code: "settings_expand_failed", Diagnostic: aiDeclarationDiagnostic{SettingsBefore: untrusted, SettingsAfter: untrusted}}
	diagnostic := captureBrowserDiagnostic(page, "publish.ai_declaration", "AI declaration", failure)
	encoded, err := json.Marshal(diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"SYNTHETIC_ACCOUNT", "SYNTHETIC_BODY", "SYNTHETIC_TOKEN"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("expansion diagnostic leaked private fixture")
		}
	}
	if diagnostic.ErrorCode != "settings_expand_failed" || diagnostic.AIDeclaration.SettingsBefore.Settings[0].Role != "other" {
		t.Fatal("expansion diagnostic not sanitized")
	}
}
