// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
)

func TestImageVisibilityOldControlSelectsPrivate(t *testing.T) {
	page, done := offlinePage(t, `<div class="permission-card-wrapper"><div class="d-select-content" id="choice">公开可见</div></div>
		<div class="d-options-wrapper" id="menu" hidden><div class="custom-option" id="private">仅自己可见</div></div>
		<button id="publish" onclick="window.published=(window.published||0)+1">发布</button>
		<script>choice.onclick=()=>{menu.hidden=false};private.onclick=()=>{choice.textContent='仅自己可见';menu.hidden=true}</script>`)
	defer done()
	if err := setImageVisibility(page, "仅自己可见"); err != nil {
		t.Fatal(err)
	}
	selected, err := page.Eval(`() => document.querySelector('#choice').textContent`)
	if err != nil || selected.Value.Str() != "仅自己可见" {
		t.Fatalf("private state not selected: %v", err)
	}
	assertNoVisibilityFixturePublish(t, page)
}

func TestImageVisibilityNewLabeledComboboxSelectsPrivate(t *testing.T) {
	page, done := offlinePage(t, `<section><span>可见范围</span><button role="combobox" aria-haspopup="listbox" id="choice">公开可见</button></section>
		<div role="listbox" id="menu" hidden><button role="option" id="private">仅自己可见</button></div>
		<button id="publish" onclick="window.published=(window.published||0)+1">发布</button>
		<script>choice.onclick=()=>{menu.hidden=false};private.onclick=()=>{setTimeout(()=>{choice.textContent='仅自己可见';menu.hidden=true},50)}</script>`)
	defer done()
	if err := setImageVisibility(page, "仅自己可见"); err != nil {
		t.Fatal(err)
	}
	assertNoVisibilityFixturePublish(t, page)
}

func TestImageVisibilityDelayedControlSelectsPrivate(t *testing.T) {
	page, done := offlinePage(t, `<section id="settings"><span>可见范围</span></section>
		<div role="listbox" id="menu" hidden><button role="option" id="private">仅自己可见</button></div>
		<button id="publish" onclick="window.published=(window.published||0)+1">发布</button>
		<script>setTimeout(()=>{
		  const choice=document.createElement('button'); choice.id='choice'; choice.setAttribute('role','combobox');
		  choice.textContent='公开可见'; choice.onclick=()=>{menu.hidden=false};
		  document.querySelector('#settings').append(choice);
		  document.querySelector('#private').onclick=()=>{choice.textContent='仅自己可见';menu.hidden=true};
		},100)</script>`)
	defer done()
	if err := setImageVisibility(page, "仅自己可见"); err != nil {
		t.Fatal(err)
	}
	assertNoVisibilityFixturePublish(t, page)
}

func TestImageVisibilityMissingControlStops(t *testing.T) {
	page, done := offlinePage(t, `<button id="publish" onclick="window.published=(window.published||0)+1">发布</button>`)
	defer done()
	err := setImageVisibility(page.Timeout(200*time.Millisecond), "仅自己可见")
	if err == nil || !strings.Contains(err.Error(), "找不到可见范围入口") {
		t.Fatalf("missing control must stop publication: %v", err)
	}
	assertNoVisibilityFixturePublish(t, page)
}

func TestImageVisibilityUnconfirmedStateStops(t *testing.T) {
	page, done := offlinePage(t, `<section><span>可见范围</span><button role="combobox" aria-haspopup="listbox" id="choice">公开可见</button></section>
		<div role="listbox" id="menu" hidden><button role="option" id="private">仅自己可见</button></div>
		<button id="publish" onclick="window.published=(window.published||0)+1">发布</button>
		<script>choice.onclick=()=>{menu.hidden=false};private.onclick=()=>{menu.hidden=true}</script>`)
	defer done()
	err := setImageVisibility(page, "仅自己可见")
	if err == nil || !strings.Contains(err.Error(), "未确认设为仅自己可见") {
		t.Fatalf("unconfirmed private selection must stop publication: %v", err)
	}
	assertNoVisibilityFixturePublish(t, page)
}

func TestImageVisibilityPublicKeepsExistingDefault(t *testing.T) {
	page, done := offlinePage(t, `<button id="publish" onclick="window.published=(window.published||0)+1">发布</button>`)
	defer done()
	if err := setImageVisibility(page, "公开可见"); err != nil {
		t.Fatal(err)
	}
	assertNoVisibilityFixturePublish(t, page)
}

func assertNoVisibilityFixturePublish(t *testing.T, page *rod.Page) {
	t.Helper()
	result, err := page.Eval(`() => window.published || 0`)
	if err != nil || result.Value.Int() != 0 {
		t.Fatalf("visibility selection must never click publish: %v", err)
	}
}
