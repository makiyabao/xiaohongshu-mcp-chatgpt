// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticAllowlistSyntheticSecrets(t *testing.T) {
	for _, raw := range []string{
		`{"api_key":"SYNTHETIC_PRIVATE_MARKER"}`, `{"cookie":"\\\"SYNTHETIC_PRIVATE_MARKER"}`,
		`Authorization: Bearer SYNTHETIC_PRIVATE_MARKER`, `Cookie: session=SYNTHETIC_PRIVATE_MARKER`,
		`https://www.xiaohongshu.com/user/profile/SYNTHETIC_PRIVATE_MARKER?token=SYNTHETIC_PRIVATE_MARKER#SYNTHETIC_PRIVATE_MARKER`,
		`SYNTHETIC_PRIVATE_MARKER 的主页`, `评论正文 SYNTHETIC_PRIVATE_MARKER`,
	} {
		if strings.Contains(safeDiagnosticText(raw), "SYNTHETIC_PRIVATE_MARKER") {
			t.Fatal("free text persisted")
		}
		if strings.Contains(safeDiagnosticLabel("publish."+raw), "SYNTHETIC_PRIVATE_MARKER") {
			t.Fatal("dynamic label persisted")
		}
	}
	for _, path := range []string{"/user/profile/SYNTHETIC_PRIVATE_MARKER", "/explore/SYNTHETIC_PRIVATE_MARKER", "/publish/SYNTHETIC_PRIVATE_MARKER"} {
		got := safeDiagnosticURL("https://user:pass@www.xiaohongshu.com" + path + "?xsec_token=SYNTHETIC_PRIVATE_MARKER#secret")
		if strings.Contains(got, "SYNTHETIC_PRIVATE_MARKER") || strings.ContainsAny(got, "?#@") {
			t.Fatal("URL path/query leak")
		}
	}
}

func TestOfflineDiagnosticAccountTitleAndValuesNeverPersist(t *testing.T) {
	// Linux Chrome creates Unix sockets under TMPDIR (108-byte path limit).
	// The isolated Docker fixture already owns /tmp; avoid a long test-name path.
	if runtime.GOOS == "windows" {
		t.Setenv("TMP", t.TempDir())
		t.Setenv("TEMP", os.Getenv("TMP"))
		t.Setenv("TMPDIR", os.Getenv("TMP"))
	}
	p, _ := offlinePage(t, `<title>SYNTHETIC_PRIVATE_MARKER 的主页</title><input value="SYNTHETIC_PRIVATE_MARKER"><article>SYNTHETIC_PRIVATE_MARKER</article><script>window.secret="SYNTHETIC_PRIVATE_MARKER"</script>`)
	d := captureBrowserDiagnostic(p, "publish.synthetic", "SYNTHETIC_PRIVATE_MARKER")
	data, err := os.ReadFile(d.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "SYNTHETIC_PRIVATE_MARKER") {
		t.Fatal("account/title/input/page text leaked")
	}
	var saved map[string]interface{}
	if json.Unmarshal(data, &saved) != nil {
		t.Fatal("invalid diagnostic")
	}
	if saved["title"] != "unknown" || saved["screenshot"] != "content_redacted_layout_only" {
		t.Fatal("diagnostic not categorical/masked")
	}
}

func TestOfflineOrdinarySecurityTutorialIsNotRisk(t *testing.T) {
	p, _ := offlinePage(t, `<article class="captcha-tutorial">普通教程：安全验证、人机验证、滑动验证的概念。操作频繁、访问异常只是说明文字。</article>`)
	if err := pageGuard(p); err != nil {
		t.Fatal("ordinary note rejected", err)
	}
	_, err := p.Eval(`() => {const control=document.createElement('div');control.className='verify-container';control.textContent='安全验证';document.body.appendChild(control)}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := pageGuard(p); !errors.Is(err, ErrBrowserRisk) {
		t.Fatal("real visible verification control missed", err)
	}
}

func TestOfflineProfileTabWaitsForTargetGeneration(t *testing.T) {
	for _, mode := range []string{"active-delayed", "data-delayed", "same-slot-old-data"} {
		t.Run(mode, func(t *testing.T) {
			// Old notes intentionally remain in both Vue state and DOM. The active tab
			// can change before its data; readiness must wait for the delayed commit.
			html := `<button class="reds-tab-item sub-tab-list" onclick="switchTab()">收藏</button><div class="note-item">old fixture note remains</div><script>
		window.__INITIAL_STATE__={user:{userPageData:{value:{basicInfo:{},interactions:[]}},notes:{value:[[{id:'old-note'}]]},activeTab:{value:{query:'note',index:0}}}};
		function switchTab(){window.commit=false;const u=window.__INITIAL_STATE__.user;
		ACTIVATE
		setTimeout(()=>{u.activeTab.value={query:'fav',index:1};u.notes.value[1]=[{id:'target-fav-note'}];window.commit=true},700);}
		</script>`
			activate := ""
			if mode == "data-delayed" {
				activate = `u.activeTab.value={query:'fav',index:1};`
			}
			if mode == "same-slot-old-data" {
				activate = `u.activeTab.value={query:'fav',index:1};u.notes.value[1]=u.notes.value[0];`
			}
			html = strings.Replace(html, "ACTIVATE", activate, 1)
			p, _ := offlinePage(t, html)
			a := NewUserProfileAction(p)
			if err := a.selectTab(p.GetContext(), p, TabFavorites); err != nil {
				t.Fatal(err)
			}
			committed, err := p.Eval(`() => window.commit`)
			if err != nil || !committed.Value.Bool() {
				t.Fatal("tab returned before target commit", err)
			}
			data, err := a.extractUserProfileData(p, TabFavorites)
			if err != nil || len(data.Feeds) != 1 || data.Feeds[0].ID != "target-fav-note" {
				t.Fatal("returned previous tab data", err)
			}
		})
	}
}

func TestOfflineProfileTabPreservesCachedTarget(t *testing.T) {
	p, _ := offlinePage(t, `<button class="reds-tab-item sub-tab-list" onclick="window.__INITIAL_STATE__.user.activeTab.value={query:'fav',index:1}">收藏</button><script>window.__INITIAL_STATE__={user:{userPageData:{value:{basicInfo:{},interactions:[]}},notes:{value:[[{id:'old'}],[{id:'cached-fav'}]]},activeTab:{value:{query:'note',index:0}}}}</script>`)
	start := time.Now()
	a := NewUserProfileAction(p)
	if err := a.selectTab(p.GetContext(), p, TabFavorites); err != nil {
		t.Fatal(err)
	}
	data, err := a.extractUserProfileData(p, TabFavorites)
	if err != nil || len(data.Feeds) != 1 || data.Feeds[0].ID != "cached-fav" || time.Since(start) > 5*time.Second {
		t.Fatal("cached target not usable", err)
	}
}
