// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-rod/rod"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
	"github.com/xpzouying/xiaohongshu-mcp/internal/rodscope"
)

// Only code-owned error codes and messages may cross the diagnostic boundary.
// Causes remain available to errors.Is, but are never formatted into logs.
type aiDeclarationError struct {
	Code       string
	Cause      error
	Diagnostic aiDeclarationDiagnostic
}

func aiDeclarationMessage(code string) string {
	switch code {
	case "declaration_entry_not_found":
		return "找不到AI声明入口：未找到笔记内容声明"
	case "declaration_entry_ambiguous":
		return "AI声明入口存在多个候选，已中止"
	case "declaration_entry_click_failed":
		return "点击AI声明入口失败，已中止"
	case "settings_expand_failed":
		return "内容设置展开失败或无法确认展开状态，已中止"
	case "declaration_entry_not_found_after_expand":
		return "展开内容设置后仍找不到AI声明入口，已中止"
	case "declaration_entry_not_clickable":
		return "AI声明栏目存在但未找到可操作入口，已中止"
	case "ai_option_not_found":
		return "找不到AI声明选项：未找到笔记含AI合成内容"
	case "ai_option_ambiguous":
		return "AI声明选项存在多个独立候选，已中止"
	case "ai_option_click_failed":
		return "点击AI声明选项失败，已中止"
	case "ai_option_not_selected_after_click":
		return "AI声明点击后状态未生效，已中止发布"
	case "conflicting_selection_detected":
		return "AI声明状态冲突或无需声明仍同时选中，已中止"
	case "ai_state_read_failed":
		return "AI声明控件状态读取失败，已中止"
	default:
		return ""
	}
}

func (e *aiDeclarationError) Error() string {
	if message := aiDeclarationMessage(e.Code); message != "" {
		return e.Code + ": " + message
	}
	return "ai_state_read_failed"
}
func (e *aiDeclarationError) Unwrap() error { return e.Cause }

func aiDeclarationFailure(code string, cause error) error {
	var known *aiDeclarationError
	if errors.As(cause, &known) || errors.Is(cause, ErrBrowserRisk) || errors.Is(cause, ErrBrowserLogin) {
		return cause
	}
	return &aiDeclarationError{Code: code, Cause: cause}
}

// No text, id, class, URL, value, account data or page-controlled free-form
// strings. Strings below are finite enums, allowlisted in JS and again in Go.
type aiControlDiagnostic struct {
	Tag                string `json:"tag"`
	Role               string `json:"role"`
	Type               string `json:"type"`
	Checked            *bool  `json:"checked"`
	AriaChecked        string `json:"aria_checked"`
	Expanded           *bool  `json:"expanded"`
	Selected           *bool  `json:"selected"`
	Disabled           bool   `json:"disabled"`
	Visible            bool   `json:"visible"`
	ClickTargetVisible bool   `json:"click_target_visible"`
	Conflict           bool   `json:"conflict"`
}

type aiDeclarationSnapshot struct {
	EntryMatches       int                   `json:"entry_match_count"`
	AIMatches          int                   `json:"ai_match_count"`
	EntryCandidates    int                   `json:"entry_candidate_count"`
	AICandidates       int                   `json:"ai_candidate_count"`
	Entries            []aiControlDiagnostic `json:"entries"`
	Options            []aiControlDiagnostic `json:"options"`
	EntryTextMatches   []aiControlDiagnostic `json:"entry_text_matches"`
	AITextMatches      []aiControlDiagnostic `json:"ai_text_matches"`
	NoneSelected       bool                  `json:"no_declaration_selected"`
	SettingsCandidates int                   `json:"settings_candidate_count"`
	Settings           []aiControlDiagnostic `json:"settings"`
	DropdownMatches    int                   `json:"dropdown_description_count"`
	SummaryMatches     int                   `json:"declaration_summary_count"`
	DropdownIsAI       bool                  `json:"dropdown_description_is_ai"`
	SummaryIsAI        bool                  `json:"declaration_summary_is_ai"`
	OptionSVGCount     int                   `json:"option_svg_count"`
	DropdownSVGCount   int                   `json:"dropdown_svg_count"`
}

// Only the two observed web UI echoes establish success. Native/ARIA flags,
// classes and icon changes remain diagnostics, never confirmation fallbacks.
func (s *aiDeclarationSnapshot) confirmed() bool {
	return s != nil && s.DropdownMatches == 1 && s.SummaryMatches == 1 && s.DropdownIsAI && s.SummaryIsAI
}

type aiDeclarationDiagnostic struct {
	EntryClickAttempts      int                    `json:"entry_click_attempts"`
	OptionClickAttempted    bool                   `json:"option_click_attempted"`
	OptionClickCompleted    bool                   `json:"option_click_completed"`
	Before                  *aiDeclarationSnapshot `json:"before,omitempty"`
	After                   *aiDeclarationSnapshot `json:"after,omitempty"`
	Last                    *aiDeclarationSnapshot `json:"last,omitempty"`
	SelectionChanged        bool                   `json:"selection_changed"`
	SettingsExpandAttempted bool                   `json:"settings_expand_attempted"`
	SettingsExpandCompleted bool                   `json:"settings_expand_completed"`
	SettingsBefore          *aiDeclarationSnapshot `json:"settings_before,omitempty"`
	SettingsAfter           *aiDeclarationSnapshot `json:"settings_after,omitempty"`
	OptionSVGDelta          int                    `json:"option_svg_delta"`
	DropdownSVGDelta        int                    `json:"dropdown_svg_delta"`
}

type rodAIDeclarationUI struct {
	page       *rod.Page
	diagnostic aiDeclarationDiagnostic
}

func aiEnum(value string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return "other"
}

func (s *aiDeclarationSnapshot) sanitize() {
	for _, list := range []*[]aiControlDiagnostic{&s.Entries, &s.Options, &s.EntryTextMatches, &s.AITextMatches, &s.Settings} {
		if len(*list) > 8 {
			*list = (*list)[:8]
		}
		for i := range *list {
			v := &(*list)[i]
			v.Tag = aiEnum(v.Tag, "input", "label", "button", "span", "div", "p", "li", "a", "h1", "h2", "h3", "h4", "h5", "h6", "dt", "legend", "summary")
			v.Role = aiEnum(v.Role, "radio", "checkbox", "button", "option", "combobox", "none")
			v.Type = aiEnum(v.Type, "radio", "checkbox", "button", "none")
			v.AriaChecked = aiEnum(v.AriaChecked, "true", "false", "mixed", "absent")
		}
	}
}

func (ui *rodAIDeclarationUI) inspect(page *rod.Page) (*aiDeclarationSnapshot, error) {
	if err := pageGuard(page); err != nil {
		return nil, err
	}
	res, err := page.Eval("() => { const r = (" + aiDeclarationResolverJS + ")(); return JSON.stringify(r.snapshot); }")
	if err != nil {
		return nil, aiDeclarationFailure("ai_state_read_failed", err)
	}
	var s aiDeclarationSnapshot
	if err = json.Unmarshal([]byte(res.Value.Str()), &s); err != nil {
		return nil, aiDeclarationFailure("ai_state_read_failed", err)
	}
	s.sanitize()
	ui.diagnostic.Last = &s
	if ui.diagnostic.OptionClickAttempted {
		ui.diagnostic.After = &s
		before := ui.diagnostic.Before
		if before != nil {
			ui.diagnostic.SelectionChanged = before.confirmed() != s.confirmed()
			ui.diagnostic.OptionSVGDelta = s.OptionSVGCount - before.OptionSVGCount
			ui.diagnostic.DropdownSVGDelta = s.DropdownSVGCount - before.DropdownSVGCount
		}
	} else {
		ui.diagnostic.Before = &s
	}
	return &s, nil
}

func aiDeclarationPause(ctx context.Context) error {
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Poll only for appearance; ambiguity is never resolved by choosing the first.
// Child contexts bind Rod input devices as well as Eval/Element calls.
func (ui *rodAIDeclarationUI) waitOption() (*aiDeclarationSnapshot, error) {
	ctx, cancel := context.WithTimeout(ui.page.GetContext(), 6*time.Second)
	defer cancel()
	end := rodscope.Bind(ui.page, ctx)
	defer end()
	p := ui.page.Context(ctx)
	for {
		s, err := ui.inspect(p)
		if err != nil {
			return nil, err
		}
		count := s.AICandidates
		if count > 1 {
			return nil, aiDeclarationFailure("ai_option_ambiguous", nil)
		}
		if count == 1 {
			return s, nil
		}
		if err = aiDeclarationPause(ctx); err != nil {
			return nil, aiDeclarationFailure("ai_option_not_found", err)
		}
	}
}

func (ui *rodAIDeclarationUI) IsAIContentDeclared() (bool, error) {
	ctx, cancel := context.WithTimeout(ui.page.GetContext(), 2*time.Second)
	defer cancel()
	end := rodscope.Bind(ui.page, ctx)
	defer end()
	s, err := ui.inspect(ui.page.Context(ctx))
	if err != nil {
		return false, err
	}
	return s.confirmed(), nil
}

// The panel may close, remove its options, or render the two echoes separately.
// Poll both in a single snapshot without reopening or clicking any option again.
func (ui *rodAIDeclarationUI) waitConfirmed() error {
	ctx, cancel := context.WithTimeout(ui.page.GetContext(), 6*time.Second)
	defer cancel()
	end := rodscope.Bind(ui.page, ctx)
	defer end()
	p := ui.page.Context(ctx)
	failure := func(cause error) error {
		if errors.Is(cause, ErrBrowserRisk) || errors.Is(cause, ErrBrowserLogin) {
			return cause
		}
		return &aiDeclarationError{Code: "ai_option_not_selected_after_click", Cause: cause}
	}
	for {
		s, err := ui.inspect(p)
		if err != nil {
			return failure(err)
		}
		if s.confirmed() {
			return nil
		}
		if err = aiDeclarationPause(ctx); err != nil {
			return failure(err)
		}
	}
}

func (ui *rodAIDeclarationUI) OpenDeclarationEntry() (bool, error) {
	ctx, cancel := context.WithTimeout(ui.page.GetContext(), 6*time.Second)
	defer cancel()
	end := rodscope.Bind(ui.page, ctx)
	defer end()
	p := ui.page.Context(ctx)
	var last *aiDeclarationSnapshot
	entryTimeout := func(cause error) error {
		code := "declaration_entry_not_found"
		if ui.diagnostic.SettingsExpandCompleted {
			code = "declaration_entry_not_found_after_expand"
		}
		if last != nil && last.EntryMatches > 0 {
			code = "declaration_entry_not_clickable"
		}
		return aiDeclarationFailure(code, cause)
	}
	for {
		s, err := ui.inspect(p)
		if err != nil {
			// A deadline may expire inside Eval rather than the polling pause.
			// Use the last successful structure snapshot for the same entry error;
			// never translate a login/risk error into a missing-control error.
			if (ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded)) && last != nil && !errors.Is(err, ErrBrowserRisk) && !errors.Is(err, ErrBrowserLogin) {
				cause := ctx.Err()
				if cause == nil {
					// The CDP child deadline can fire just before the parent timer.
					cause = context.DeadlineExceeded
				}
				return false, entryTimeout(cause)
			}
			return false, err
		}
		last = s
		if s.confirmed() {
			return true, nil // Already declared: no entry or option toggle.
		}
		// A visible option means the panel is already open; never toggle it shut.
		if s.AICandidates > 1 {
			return false, aiDeclarationFailure("ai_option_ambiguous", nil)
		}
		if s.AICandidates == 1 {
			return true, nil
		}
		if s.EntryCandidates > 1 {
			return false, aiDeclarationFailure("declaration_entry_ambiguous", nil)
		}
		if s.EntryCandidates == 1 {
			if s.Entries[0].Disabled || !s.Entries[0].ClickTargetVisible {
				return false, aiDeclarationFailure("declaration_entry_not_clickable", nil)
			}
			return true, ui.clickCandidate(true)
		}
		if !ui.diagnostic.SettingsExpandAttempted {
			collapsed := false
			for _, control := range s.Settings {
				collapsed = collapsed || (control.Expanded != nil && !*control.Expanded)
			}
			if collapsed {
				if s.SettingsCandidates != 1 {
					return false, aiDeclarationFailure("settings_expand_failed", nil)
				}
				if err = ui.expandSettings(p, s); err != nil {
					return false, err
				}
				continue // Re-resolve after the one permitted expansion, not a click retry.
			}
		}
		if err = aiDeclarationPause(ctx); err != nil {
			return false, entryTimeout(err)
		}
	}
}

// Only a positively identified collapsed section can be expanded. Interaction
// is attempted once per declaration flow, including the verification reopen.
func (ui *rodAIDeclarationUI) expandSettings(page *rod.Page, before *aiDeclarationSnapshot) error {
	ctx, cancel := context.WithTimeout(page.GetContext(), 2*time.Second)
	defer cancel()
	end := rodscope.Bind(page, ctx)
	defer end()
	p := page.Context(ctx)
	ui.diagnostic.SettingsExpandAttempted = true
	ui.diagnostic.SettingsBefore = before
	elem, err := p.ElementByJS(rod.Eval("() => { const a=(" + aiDeclarationResolverJS + ")().settings; return a.length===1 && !a[0].state.disabled && a[0].state.expanded===false ? a[0].target : null; }"))
	if err != nil {
		return aiDeclarationFailure("settings_expand_failed", err)
	}
	if err = humanize.Click(elem); err != nil {
		return aiDeclarationFailure("settings_expand_failed", err)
	}
	for {
		s, inspectErr := ui.inspect(p)
		if inspectErr != nil {
			return aiDeclarationFailure("settings_expand_failed", inspectErr)
		}
		ui.diagnostic.SettingsAfter = s
		if s.EntryCandidates > 0 || s.AICandidates > 0 || (s.SettingsCandidates == 1 && s.Settings[0].Expanded != nil && *s.Settings[0].Expanded) {
			ui.diagnostic.SettingsExpandCompleted = true
			return nil
		}
		if err = aiDeclarationPause(ctx); err != nil {
			return aiDeclarationFailure("settings_expand_failed", err)
		}
	}
}

func (ui *rodAIDeclarationUI) SelectAIContentDeclaration() (bool, error) {
	if ui.diagnostic.OptionClickAttempted {
		return false, aiDeclarationFailure("ai_option_not_selected_after_click", nil)
	}
	if _, err := ui.waitOption(); err != nil {
		return false, err
	}
	return true, ui.clickCandidate(false)
}

func (ui *rodAIDeclarationUI) clickCandidate(entry bool) error {
	ctx, cancel := context.WithTimeout(ui.page.GetContext(), 4*time.Second)
	defer cancel()
	end := rodscope.Bind(ui.page, ctx)
	defer end()
	p := ui.page.Context(ctx)
	key, code := "options", "ai_option_click_failed"
	if entry {
		key, code = "entries", "declaration_entry_click_failed"
	}
	// Re-resolve immediately before interaction, including uniqueness. DOM changes
	// never permit falling back to the first element of an ambiguous list.
	elem, err := p.ElementByJS(rod.Eval("() => {const r=(" + aiDeclarationResolverJS + ")();const a=r." + key + "; if(a.length!==1 || a[0].state.disabled)return null; if('" + key + "'==='entries')window.__xhsMCPAIDeclarationRoot=r.declarationRoot; return a[0].target;}"))
	if err != nil {
		return aiDeclarationFailure(code, err)
	}
	if entry {
		ui.diagnostic.EntryClickAttempts++
	} else {
		ui.diagnostic.OptionClickAttempted = true
	}
	err = humanize.Click(elem)
	if !entry && err == nil {
		ui.diagnostic.OptionClickCompleted = true
	}
	// Capture best-effort after-state even if click failed. No retry of interaction.
	_, stateErr := ui.inspect(p)
	if err != nil {
		return aiDeclarationFailure(code, err)
	}
	if stateErr != nil {
		if !entry && !errors.Is(stateErr, ErrBrowserRisk) && !errors.Is(stateErr, ErrBrowserLogin) {
			// Best-effort click diagnostics are not a success check. The separate
			// bounded confirmation poll must decide whether both echoes appear.
			return nil
		}
		return stateErr
	}
	return nil
}

// DOM text is used transiently only to associate fixed declaration/settings
// labels with controls. Bare headings never become click targets.
// No page text/attributes are returned. Repeated spans/labels referring to the
// same control collapse to one candidate; separate controls remain ambiguous.
const aiDeclarationResolverJS = `() => {
 const norm=s=>(s||'').replace(/\s+/g,'');
 const visible=e=>{if(!e||e.closest('[contenteditable],textarea,script,style,[hidden],[aria-hidden="true"]'))return false;for(let p=e.parentElement;p;p=p.parentElement)if(p.matches('details:not([open])')&&![...p.children].find(c=>c.matches('summary'))?.contains(e))return false;const r=e.getBoundingClientRect(),s=getComputedStyle(e);return r.width>0&&r.height>0&&s.visibility!=='hidden'&&s.display!=='none'};
 const radio='input[type="radio"],input[type="checkbox"],[role="radio"],[role="checkbox"]';
 const bool=s=>s==='true'?true:s==='false'?false:null;
 const enumOf=(v,allowed)=>allowed.includes(v)?v:'other';
 const text=e=>norm(e.innerText||'');
 const matches=(e,label)=>text(e)===label||norm([...e.childNodes].filter(n=>n.nodeType===Node.TEXT_NODE).map(n=>n.textContent).join(''))===label;
 const expanded=e=>{
   const explicit=bool(e.getAttribute('aria-expanded'));if(explicit!==null)return explicit;
   if(e.matches('summary')&&e.parentElement?.matches('details'))return e.parentElement.open;
   const ids=(e.getAttribute('aria-controls')||'').split(/\s+/).filter(Boolean),nodes=ids.map(id=>document.getElementById(id));
   if(nodes.length&&nodes.every(Boolean))return nodes.every(visible);
   // Non-ARIA accordions: accept only a directly associated sibling containing
   // a fixed declaration heading, never an arbitrary hidden region elsewhere.
   const sibling=e.nextElementSibling;
   if(text(e)==='内容设置'&&sibling){
     const headings=[sibling,...sibling.querySelectorAll('h1,h2,h3,h4,h5,h6,label,span,div,p,dt,legend,button,[role="button"],[role="combobox"]')];
     if(headings.some(h=>!h.closest('[contenteditable],textarea,script,style')&&['笔记内容声明','添加内容类型声明'].includes(norm([...h.childNodes].filter(n=>n.nodeType===Node.TEXT_NODE).map(n=>n.textContent).join('')))))return visible(sibling);
   }
   return null;
 };
 const state=(target,control)=>{
   const c=control||target, native=c.matches('input[type="radio"],input[type="checkbox"]');
   const checked=native?!!c.checked:null, aria=c.getAttribute('aria-checked');
   const explicit=bool(c.getAttribute('aria-selected'));
   const data=c.getAttribute('data-state');
   const values=[checked,bool(aria),explicit,data==='checked'?true:data==='unchecked'?false:null,target===c?null:bool(target.getAttribute('aria-checked'))].filter(v=>v!==null);
   return {tag:enumOf(c.tagName.toLowerCase(),['input','label','button','span','div','p','li','a','h1','h2','h3','h4','h5','h6','dt','legend','summary']),role:enumOf(c.getAttribute('role')||'none',['radio','checkbox','button','option','combobox','none']),type:enumOf(c.getAttribute('type')||'none',['radio','checkbox','button','none']),checked,aria_checked:enumOf(aria||'absent',['true','false','mixed','absent']),expanded:expanded(target),selected:values.length?values[0]:null,disabled:!!(c.matches(':disabled')||target.closest('[aria-disabled="true"]')||c.closest('[aria-disabled="true"]')),visible:visible(c),click_target_visible:visible(target),conflict:(values.includes(true)&&values.includes(false))||aria==='mixed'};
 };
 const elements=[...document.querySelectorAll('button,label,span,div,p,li,a,h1,h2,h3,h4,h5,h6,dt,legend,summary,input[type="radio"],input[type="checkbox"],[role="radio"],[role="checkbox"],[role="button"],[role="combobox"],[aria-expanded]')];
 const entryLabels=['笔记内容声明','添加内容类型声明'];
 const interactive='button,[role="button"],[role="combobox"],[aria-expanded],[aria-haspopup],summary,a[href]';
 const safeEntry=e=>visible(e)&&!e.matches(radio)&&!e.closest(radio)&&!(/含有AI合成内容|原创声明/.test(text(e)))&&!(text(e)==='无需声明'&&!e.matches('[aria-haspopup],[role="combobox"]'));
 // The web creator dropdown has neither a native input nor ARIA attributes.
 // Recognize its multi-part trigger structure, not a class or text node alone.
 // Return the main hit area: the placeholder itself is not the trigger.
 const webDropdown=e=>{
   const root=e.matches('.d-select')?e:e.closest('.d-select');
   if(!root||root.closest(radio))return null;
   const mains=[...root.children].filter(c=>c.matches('.d-select-main'));
   if(mains.length!==1)return null;
   const main=mains[0],contents=[...main.querySelectorAll('.d-select-content')];
   if(contents.length!==1)return null;
   const content=contents[0];
   if(!content.querySelector('.d-select-placeholder')&&!main.querySelector('svg,[aria-haspopup],[role="combobox"]')&&!root.querySelector('[role="listbox"],.d-options-wrapper'))return null;
   return main;
 };
 const canonicalEntry=e=>{
   const semantic=e.closest('[role="combobox"]')||e.closest('[aria-haspopup="listbox"],[aria-haspopup="menu"]');
   if(semantic&&safeEntry(semantic)){
     // A labelled combobox wrapper may be wider than its actual trigger. Both
     // the heading association and nested trigger must resolve to that button,
     // not to an empty point in the wrapper's centre.
     if(!semantic.matches('button,[role="button"]')){
       const buttons=[...semantic.querySelectorAll('button,[role="button"]')].filter(b=>safeEntry(b)&&!b.closest('[role="listbox"],[role="menu"],'+radio));
       if(buttons.length===1)return buttons[0];
     }
     return semantic;
   }
   return webDropdown(e)||e;
 };
 const resolveEntry=labels=>{
   const leaves=elements.filter(e=>visible(e)&&labels.some(label=>matches(e,label))&&![...e.children].some(c=>visible(c)&&labels.some(label=>matches(c,label))));
   for(const e of elements)if(visible(e)&&e.matches(interactive)&&labels.includes(norm(e.getAttribute('aria-label')))&&!leaves.includes(e))leaves.push(e);
   const candidates=[],seen=new Set();
   const add=e=>{if(!e)return;e=canonicalEntry(e);if(!safeEntry(e)||seen.has(e))return;seen.add(e);candidates.push({target:e,control:e,state:state(e,e)});};
   for(const leaf of leaves){
     // Explicit accessible association wins over proximity. Do not choose a
     // random neighbouring button if the heading's associated control is hidden.
     const associated=elements.filter(e=>e.matches(interactive)&&((leaf.id&&(e.getAttribute('aria-labelledby')||'').split(/\s+/).includes(leaf.id))||(leaf.matches('label')&&leaf.control===e)));
     if(associated.length){associated.forEach(add);continue;}
     let own=null;
     for(let p=leaf,n=0;p&&p!==document.body&&n<4;p=p.parentElement,n++){
       if(/含有AI合成内容|无需声明|原创声明/.test(text(p)))break;
       if(labels===entryLabels&&text(p).includes('内容设置'))break;
       if(labels!==entryLabels&&entryLabels.some(label=>text(p).includes(label)))break;
       if(labels===entryLabels){const trigger=webDropdown(p);if(trigger){own=trigger;break;}}
       // A heading/placeholder, onclick or cursor:pointer alone does not prove
       // a dropdown. Require semantics or the validated web trigger structure.
       if(p.matches(interactive)){own=p;break;}
       // Keep non-ARIA accordion support only when a directly associated
       // declaration section proves its expanded/collapsed state. This does
       // not turn a bare declaration heading into an entry click target.
       if(labels!==entryLabels&&expanded(p)!==null&&(typeof p.onclick==='function'||getComputedStyle(p).cursor==='pointer')){own=p;break;}
     }
     if(own){add(own);continue;}
     // A separate dropdown can be a sibling of a heading. Require dropdown
     // semantics, stay within a small row, and stop before unrelated settings.
     for(let row=leaf.parentElement,n=0;row&&row!==document.body&&n<2;row=row.parentElement,n++){
       if(/原创声明|含有AI合成内容/.test(text(row)))break;
       if(labels===entryLabels&&text(row).includes('内容设置'))break;
       if(labels!==entryLabels&&entryLabels.some(label=>text(row).includes(label)))break;
       const controls=[...row.querySelectorAll('button,[role="button"],[role="combobox"],[aria-haspopup],[aria-expanded],.d-select')].filter(e=>safeEntry(e)&&(e.matches('[role="combobox"],[aria-haspopup],[aria-expanded]')||/^[▼▾⌄∨]$/.test(text(e))||(labels===entryLabels&&webDropdown(e))));
       if(controls.length){controls.forEach(add);break;}
     }
   }
   return {matches:leaves.length,leaves,candidates};
 };
 const resolve=label=>{
   const leaves=elements.filter(e=>visible(e)&&matches(e,label)&&![...e.children].some(c=>visible(c)&&matches(c,label)));
   // Accessible-name controls may have no literal text node.
   for(const e of elements)if(visible(e)&&e.matches(radio)&&norm(e.getAttribute('aria-label'))===label&&!leaves.includes(e))leaves.push(e);
   const candidates=[],seen=new Set();
   for(const leaf of leaves){
     let target=leaf,control=null;
     {
       let found=false;
       for(let p=leaf,n=0;p&&p!==document.body&&n<6;p=p.parentElement,n++){
         const other=label==='无需声明'?/含有AI合成内容|原创声明/:/无需声明|原创声明/;
         if(other.test(text(p)))break;
         const controls=[...p.querySelectorAll(radio)].filter(c=>!c.parentElement?.closest(radio));
         if(controls.length>1){for(const c of controls)if(!seen.has(c)){seen.add(c);candidates.push({target:p,control:c,state:state(p,c)});}break;}
         if(p.matches(radio)){control=p;target=p;found=true;break;}
         // Observed web option: a real dropdown row with no checked/ARIA.
         // This permits clicking it, but never establishes selection success.
         if(p.matches('.d-option')&&leaf.closest('.d-options-wrapper,[role="listbox"]')){control=p;target=p;found=true;break;}
         if(p.matches('label')&&p.control&&p.control.matches(radio)){control=p.control;target=p;found=true;break;}
         if(controls.length===1){control=controls[0];target=p;found=true;break;}
         if(p.matches('[aria-checked],[aria-selected],[data-state]')){control=p;target=p;found=true;break;}
       }
       // Plain prose is not a selectable declaration. Keep text-match count for diagnosis.
       if(!found)continue;
       if(control&&!control.matches('input')){const native=control.querySelectorAll('input[type="radio"],input[type="checkbox"]');if(native.length===1)control=native[0];if(native.length>1){for(const c of native)if(!seen.has(c)){seen.add(c);candidates.push({target,control:c,state:state(target,c)});}continue;}}
       // A visible label is the safe click target for a hidden native input.
       if(control&&control.labels){const labels=[...control.labels].filter(l=>visible(l)&&text(l).includes(label));if(labels.length===1)target=labels[0];}
     }
     const identity=control||target;
     if(seen.has(identity))continue;
     seen.add(identity);candidates.push({target,control,state:state(target,control)});
   }
   return {matches:leaves.length,leaves,candidates};
 };
 const entry=resolveEntry(entryLabels),settings=resolveEntry(['内容设置']),none=resolve('无需声明');
 const aiParts=['笔记含AI合成内容','含有AI合成内容'].map(resolve),ai={matches:aiParts.reduce((n,r)=>n+r.matches,0),leaves:[...new Set(aiParts.flatMap(r=>r.leaves))],candidates:[]};
 const aiSeen=new Set();for(const c of aiParts.flatMap(r=>r.candidates)){const id=c.control||c.target;if(!aiSeen.has(id)){aiSeen.add(id);ai.candidates.push(c);}}
 // Pin the actual declaration trigger during entry interaction. Do not let an
 // unrelated dropdown echo satisfy confirmation after the DOM changes.
 const rootOf=e=>e?.closest('.d-select,[role="combobox"]')||e;
 const pinned=window.__xhsMCPAIDeclarationRoot;
 let declarationRoot=pinned?(pinned.isConnected?pinned:null):entry.candidates.length===1?rootOf(entry.candidates[0].target):null;
 if(!pinned&&!declarationRoot){
   const roots=[...new Set([...document.querySelectorAll('.d-select-description')].filter(e=>visible(e)&&(e.innerText||'').trim()==='笔记含AI合成内容').map(e=>e.closest('.d-select')).filter(r=>r&&r.querySelector('.d-select-main .d-select-content')))];
   if(roots.length===1)declarationRoot=roots[0];
 }
 const descriptions=declarationRoot?[...declarationRoot.querySelectorAll('.d-select-description')].filter(visible):[];
 const summaries=[...document.querySelectorAll('.permission-info-declaration .permission-info-text')].filter(visible);
 const isAI=nodes=>nodes.length===1&&(nodes[0].innerText||'').trim()==='笔记含AI合成内容';
 // Count icons even when the popup is hidden; counts are diagnostic only.
 const aiPopups=[...document.querySelectorAll('.d-options-wrapper,[role="listbox"]')].filter(e=>['笔记含AI合成内容','含有AI合成内容'].some(label=>norm(e.textContent).includes(label)));
 const optionSVGCount=aiPopups.reduce((n,e)=>n+e.querySelectorAll('svg').length,0);
 return {declarationRoot,entries:entry.candidates,settings:settings.candidates,options:ai.candidates,snapshot:{entry_match_count:entry.matches,ai_match_count:ai.matches,entry_candidate_count:entry.candidates.length,ai_candidate_count:ai.candidates.length,settings_candidate_count:settings.candidates.length,settings:settings.candidates.slice(0,8).map(c=>c.state),entries:entry.candidates.slice(0,8).map(c=>c.state),options:ai.candidates.slice(0,8).map(c=>c.state),entry_text_matches:entry.leaves.slice(0,8).map(e=>state(e,e)),ai_text_matches:ai.leaves.slice(0,8).map(e=>state(e,e)),no_declaration_selected:none.candidates.some(c=>c.state.selected===true),dropdown_description_count:descriptions.length,declaration_summary_count:summaries.length,dropdown_description_is_ai:isAI(descriptions),declaration_summary_is_ai:isAI(summaries),option_svg_count:optionSVGCount,dropdown_svg_count:declarationRoot?declarationRoot.querySelectorAll('svg').length:0}};
}`
