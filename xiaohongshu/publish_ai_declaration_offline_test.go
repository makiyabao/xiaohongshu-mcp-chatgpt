// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"strconv"
	"strings"
	"testing"

	"github.com/go-rod/rod"
)

func TestOfflineAIDeclarationSelectsAndRechecksAfterPanelCloses(t *testing.T) {
	page, done := offlinePage(t, aiDeclarationFixture(`
    ai.onclick=()=>{window.aiClicks++;ai.setAttribute('aria-checked','true');none.setAttribute('aria-checked','false');setAIEchoes();panel.hidden=true};`))
	defer done()
	if err := ensureAIGeneratedDeclaration(&rodAIDeclarationUI{page: page}, true); err != nil {
		t.Fatal(err)
	}
	assertAIPageState(t, page, true, 1)
}

func TestOfflineAIDeclarationAlreadySelectedIsIdempotent(t *testing.T) {
	page, done := offlinePage(t, aiDeclarationFixture(`ai.setAttribute('aria-checked','true');none.setAttribute('aria-checked','false');setAIEchoes();`))
	defer done()
	if err := ensureAIGeneratedDeclaration(&rodAIDeclarationUI{page: page}, true); err != nil {
		t.Fatal(err)
	}
	assertAIPageState(t, page, true, 0)
}

func TestOfflineAIDeclarationMissingPanelEntryStops(t *testing.T) {
	page, done := offlinePage(t, `<button id="publish" onclick="window.published=1">发布</button>`)
	defer done()
	err := ensureAIGeneratedDeclaration(&rodAIDeclarationUI{page: page}, true)
	if err == nil || !strings.Contains(err.Error(), "找不到AI声明入口") {
		t.Fatalf("missing entry must stop: %v", err)
	}
	assertNoVisibilityFixturePublish(t, page)
}

func TestOfflineAIDeclarationPanelWithoutAIOptionStops(t *testing.T) {
	page, done := offlinePage(t, `<button id="entry" onclick="panel.hidden=false">笔记内容声明</button>
      <div id="panel" hidden><button role="radio" aria-checked="true">无需声明</button></div>
      <button id="publish" onclick="window.published=1">发布</button>`)
	defer done()
	err := ensureAIGeneratedDeclaration(&rodAIDeclarationUI{page: page}, true)
	if err == nil || !strings.Contains(err.Error(), "找不到AI声明选项") {
		t.Fatalf("missing AI option must stop: %v", err)
	}
	assertNoVisibilityFixturePublish(t, page)
}

func TestOfflineAIDeclarationClickWithoutSelectedStateStops(t *testing.T) {
	page, done := offlinePage(t, aiDeclarationFixture(`ai.onclick=()=>{window.aiClicks++};`))
	defer done()
	err := ensureAIGeneratedDeclaration(&rodAIDeclarationUI{page: page}, true)
	if err == nil || !strings.Contains(err.Error(), "状态未生效") {
		t.Fatalf("unconfirmed AI state must stop: %v", err)
	}
	assertAIPageState(t, page, false, 1)
}

func TestOfflineAIDeclarationOriginalCannotSubstitute(t *testing.T) {
	page, done := offlinePage(t, aiDeclarationFixture(`original.checked=true;ai.onclick=()=>{window.aiClicks++};`))
	defer done()
	err := ensureAIGeneratedDeclaration(&rodAIDeclarationUI{page: page}, true)
	if err == nil || !strings.Contains(err.Error(), "状态未生效") {
		t.Fatalf("original selection cannot substitute AI declaration: %v", err)
	}
	assertAIPageState(t, page, false, 1)
}

func TestOfflineAIDeclarationNoneCannotSubstitute(t *testing.T) {
	page, done := offlinePage(t, aiDeclarationFixture(`none.setAttribute('aria-checked','true');ai.onclick=()=>{window.aiClicks++};`))
	defer done()
	err := ensureAIGeneratedDeclaration(&rodAIDeclarationUI{page: page}, true)
	if err == nil || !strings.Contains(err.Error(), "状态未生效") {
		t.Fatalf("no-declaration selection cannot substitute AI declaration: %v", err)
	}
	assertAIPageState(t, page, false, 1)
}

func TestOfflineAIDeclarationConflictingNativeFlagsCannotConfirm(t *testing.T) {
	page, done := offlinePage(t, aiDeclarationFixture(`ai.setAttribute('aria-checked','true');none.setAttribute('aria-checked','true');ai.onclick=()=>{window.aiClicks++};`))
	defer done()
	err := ensureAIGeneratedDeclaration(&rodAIDeclarationUI{page: page}, true)
	if err == nil || !strings.Contains(err.Error(), "ai_option_not_selected_after_click") {
		t.Fatalf("native flags without dual echoes must stop: %v", err)
	}
	assertAIPageState(t, page, true, 1)
}

func TestOfflineAIDeclarationClassChangesStillWork(t *testing.T) {
	page, done := offlinePage(t, aiDeclarationFixture(`entry.className='redesigned-entry';ai.className='new-radio-style';ai.onclick=()=>{window.aiClicks++;ai.setAttribute('aria-checked','true');none.setAttribute('aria-checked','false');setAIEchoes()};`))
	defer done()
	if err := ensureAIGeneratedDeclaration(&rodAIDeclarationUI{page: page}, true); err != nil {
		t.Fatal(err)
	}
	assertAIPageState(t, page, true, 1)
}

func aiDeclarationFixture(script string) string {
	return `<button id="entry"><span>笔记内容声明</span><span id="description" class="d-select-description"></span></button>
      <div id="panel" hidden>
        <div id="none" role="radio" aria-checked="true"><span>无需声明</span></div>
        <div id="ai" role="radio" aria-checked="false"><span>含有 AI 合成内容</span></div>
      </div>
      <label>原创声明<input id="original" type="checkbox"></label>
      <span class="permission-info-declaration"><span id="summary" class="permission-info-text"></span></span>
      <button id="publish" onclick="window.published=1">发布</button>
      <script>window.aiClicks=0;window.setAIEchoes=()=>{description.textContent='笔记含AI合成内容';summary.textContent='笔记含AI合成内容'};entry.onclick=()=>{panel.hidden=!panel.hidden};` + script + `</script>`
}

func assertAIPageState(t *testing.T, page *rod.Page, selected bool, clicks int) {
	t.Helper()
	result, err := page.Eval(`() => JSON.stringify({selected:document.querySelector('#ai').getAttribute('aria-checked')==='true',clicks:window.aiClicks,published:window.published||0})`)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"selected":false,"clicks":0,"published":0}`
	if selected {
		want = strings.Replace(want, `"selected":false`, `"selected":true`, 1)
	}
	want = strings.Replace(want, `"clicks":0`, `"clicks":`+strconv.Itoa(clicks), 1)
	if result.Value.Str() != want {
		t.Fatalf("unexpected AI fixture state: %s (want %s)", result.Value.Str(), want)
	}
}
