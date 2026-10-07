// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

// Keep the video publisher's existing selector unchanged. Image publication
// must fail closed when a requested non-public state cannot be verified.
func setImageVisibility(page *rod.Page, visibility string) error {
	if visibility == "" || visibility == "公开可见" {
		return nil
	}
	if visibility != "仅自己可见" && visibility != "仅互关好友可见" {
		return fmt.Errorf("不支持的可见范围: %s", visibility)
	}

	entry, err := page.Timeout(8 * time.Second).ElementByJS(rod.Eval(visibilityEntryJS))
	if err != nil {
		return fmt.Errorf("找不到可见范围入口，已中止发布: %w", err)
	}
	current, err := visibleSelection(entry)
	if err != nil {
		return fmt.Errorf("无法读取可见范围状态，已中止发布: %w", err)
	}
	if current == visibility {
		return nil
	}
	if err := humanize.Click(entry); err != nil {
		return fmt.Errorf("点击可见范围入口失败，已中止发布: %w", err)
	}
	option, err := page.Timeout(5 * time.Second).ElementByJS(rod.Eval(visibilityOptionJS, visibility))
	if err != nil {
		return fmt.Errorf("找不到可见范围选项，已中止发布: %w", err)
	}
	if err := humanize.Click(option); err != nil {
		return fmt.Errorf("选择可见范围失败，已中止发布: %w", err)
	}

	// UI updates may be asynchronous. Read only after the single option click;
	// never retry the click or continue to the publish button on uncertainty.
	deadline := time.Now().Add(1500 * time.Millisecond)
	for {
		selected, err := page.Eval("() => { const entry = (" + visibilityEntryJS + ")(); if (!entry) return ''; const allowed=['公开可见','仅自己可见','仅互关好友可见']; const exact=e=>(e.textContent||'').trim(); if(allowed.includes(exact(entry)))return exact(entry); const found=Array.from(entry.querySelectorAll('*')).filter(e=>allowed.includes(exact(e)) && e.getBoundingClientRect().width>0); const states=[...new Set(found.map(exact))]; return states.length===1 ? states[0] : ''; }")
		if err != nil {
			return fmt.Errorf("复核可见范围失败，已中止发布: %w", err)
		}
		if selected.Value.Str() == visibility {
			return nil
		}
		if time.Now().After(deadline) || page.GetContext().Err() != nil {
			return fmt.Errorf("可见范围未确认设为%s，已中止发布", visibility)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func visibleSelection(entry *rod.Element) (string, error) {
	value, err := entry.Eval(`() => {const allowed=['公开可见','仅自己可见','仅互关好友可见']; const text=(this.textContent||'').trim(); if(allowed.includes(text)) return text; const exact=el=>(el.textContent||'').trim(); const states=[...new Set(Array.from(this.querySelectorAll('*')).filter(el=>allowed.includes(exact(el)) && el.getBoundingClientRect().width>0).map(exact))]; return states.length===1 ? states[0] : '';}`)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value.Value.Str()), nil
}

// Class names are a compatibility hint only. Locate the visible control by
// its accessibility role or by a nearby visibility label and current state.
const visibilityEntryJS = `() => {
  const allowed=['公开可见','仅自己可见','仅互关好友可见'];
  const visible=e=>{if(!e || e.closest('[contenteditable]'))return false; const r=e.getBoundingClientRect(); const s=getComputedStyle(e); return r.width>0 && r.height>0 && s.display!=='none' && s.visibility!=='hidden'};
  const state=e=>allowed.includes((e.textContent||'').trim());
  const old=document.querySelector('div.permission-card-wrapper div.d-select-content');
  if(visible(old))return old;
  const controls='button,[role="combobox"],[aria-haspopup="listbox"],[aria-haspopup="menu"],[class*="select-content"],[class*="permission"],[class*="visibility"]';
  const labels=Array.from(document.querySelectorAll('label,span,div')).filter(e=>visible(e) && ['可见范围','谁可以看'].includes((e.textContent||'').trim()) && !Array.from(e.children).some(c=>['可见范围','谁可以看'].includes((c.textContent||'').trim())));
  const near=[];
  for(const label of labels){let parent=label.parentElement;for(let depth=0;parent && depth<3;depth++,parent=parent.parentElement){const matches=Array.from(parent.querySelectorAll(controls)).filter(e=>visible(e) && state(e));if(matches.length){near.push(...matches);break}}}
  const standalone=Array.from(document.querySelectorAll(controls)).filter(e=>visible(e) && state(e));
  const candidates=near.length?near:standalone;
  const unique=[...new Set(candidates)].filter(e=>!candidates.some(other=>other!==e && e.contains(other)));
  return unique.length===1?unique[0]:null;
}`

const visibilityOptionJS = `(target) => {
  const visible=e=>{if(!e || e.closest('[contenteditable]'))return false; const r=e.getBoundingClientRect(); const s=getComputedStyle(e); return r.width>0 && r.height>0 && s.display!=='none' && s.visibility!=='hidden'};
  const choices=Array.from(document.querySelectorAll('[role="option"],[role="menuitemradio"],[role="radio"],.custom-option,[class*="option"],li,button,label'))
    .filter(e=>visible(e) && (e.textContent||'').trim()===target);
  const unique=[...new Set(choices)].filter(e=>!choices.some(other=>other!==e && e.contains(other)));
  return unique.length===1?unique[0]:null;
}`
