// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/xpzouying/xiaohongshu-mcp/configs"
	"github.com/xpzouying/xiaohongshu-mcp/internal/rodscope"
)

type browserDiagnostic struct {
	Step                     string `json:"step"`
	Waiting                  string `json:"waiting"`
	URL                      string `json:"url"`
	Title                    string `json:"title"`
	ReadyState               string `json:"ready_state,omitempty"`
	Elements, Inputs, Images int
	Artifact                 string                   `json:"-"`
	Screenshot               string                   `json:"screenshot"`
	Layout                   []diagnosticRect         `json:"layout,omitempty"`
	HasUploadContent         bool                     `json:"has_upload_content"`
	HasEditor                bool                     `json:"has_editor"`
	ErrorCode                string                   `json:"error_code,omitempty"`
	AIDeclaration            *aiDeclarationDiagnostic `json:"ai_declaration,omitempty"`
}

type diagnosticRect struct{ X, Y, W, H int }

// Never dump HTML, text, input values, cookies or page state. Screenshots are
// fully obscured before saving: a fail-closed privacy guarantee even for text in
// canvases, shadow DOM, QR codes and images. Layout counts are retained in JSON.
// A raw screenshot is held only in memory and is never returned to the model.
func redactDiagnosticPNG(raw []byte, layout ...diagnosticRect) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	redacted := image.NewRGBA(src.Bounds())
	draw.Draw(redacted, redacted.Bounds(), image.NewUniform(color.RGBA{R: 90, G: 90, B: 90, A: 255}), image.Point{}, draw.Src)
	// Keep only control geometry, never text/image pixels. This lets a timeout
	// report distinguish an empty page from an editor/overlay without persisting
	// account details, a draft, credentials, QR codes or uploaded photographs.
	for _, box := range layout {
		r := image.Rect(box.X, box.Y, box.X+box.W, box.Y+box.H).Intersect(redacted.Bounds())
		if r.Empty() {
			continue
		}
		ink := image.NewUniform(color.RGBA{R: 180, G: 180, B: 180, A: 255})
		for _, edge := range []image.Rectangle{
			image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1),
			image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y),
			image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y),
			image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y),
		} {
			draw.Draw(redacted, edge, ink, image.Point{}, draw.Src)
		}
	}
	var b bytes.Buffer
	err = png.Encode(&b, redacted)
	return b.Bytes(), err
}

func captureBrowserDiagnostic(page *rod.Page, step, waiting string, declaration ...*aiDeclarationError) browserDiagnostic {
	d := browserDiagnostic{Step: safeDiagnosticLabel(step), Waiting: safeDiagnosticLabel(waiting), URL: "[unavailable]", Title: "unknown", Screenshot: "unavailable"}
	if len(declaration) > 0 && declaration[0] != nil && aiDeclarationMessage(declaration[0].Code) != "" {
		d.ErrorCode = declaration[0].Code
		safe := declaration[0].Diagnostic
		for _, snapshot := range []*aiDeclarationSnapshot{safe.Before, safe.After, safe.Last, safe.SettingsBefore, safe.SettingsAfter} {
			if snapshot != nil {
				snapshot.sanitize()
			}
		}
		d.AIDeclaration = &safe
		if step == "publish.ai_declaration" || step == "publish_video.ai_declaration" {
			d.Step = step
			d.Waiting = "ai_declaration_control_checked"
		}
	}
	// The failing step may already be canceled; diagnostics have their own short,
	// read-only budget. This does not extend the operation or trigger recovery.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p := page.Context(ctx)
	end := rodscope.Bind(page, ctx)
	defer end()
	if info, err := p.Info(); err == nil {
		d.URL = safeDiagnosticURL(info.URL)
		if parsed, err := url.Parse(info.URL); err == nil {
			d.Title = diagnosticPageState(parsed.Path)
		}
	}
	res, err := p.Eval(`() => JSON.stringify({ready_state:document.readyState, Elements:document.querySelectorAll('*').length, Inputs:document.querySelectorAll('input,textarea,[contenteditable]').length, Images:document.querySelectorAll('img,canvas,video,iframe').length, has_upload_content:!!document.querySelector('div.upload-content'), has_editor:!!document.querySelector('[contenteditable]'), layout:Array.from(document.querySelectorAll('input,textarea,button,[contenteditable],img,canvas,video,iframe,div.upload-content')).slice(0,80).map(el=>{const r=el.getBoundingClientRect(); return {X:Math.round(r.x),Y:Math.round(r.y),W:Math.round(r.width),H:Math.round(r.height)}})})`)
	if err == nil {
		var state struct {
			ReadyState               string `json:"ready_state"`
			Elements, Inputs, Images int
			Layout                   []diagnosticRect `json:"layout"`
			HasUploadContent         bool             `json:"has_upload_content"`
			HasEditor                bool             `json:"has_editor"`
		}
		if json.Unmarshal([]byte(res.Value.Str()), &state) == nil {
			switch state.ReadyState {
			case "loading", "interactive", "complete":
				d.ReadyState = state.ReadyState
			}
			d.Elements, d.Inputs, d.Images = state.Elements, state.Inputs, state.Images
			d.HasEditor, d.HasUploadContent = state.HasEditor, state.HasUploadContent
			if len(state.Layout) > 80 {
				state.Layout = state.Layout[:80]
			}
			d.Layout = state.Layout
		}
	}
	dir, err := os.MkdirTemp(filepath.Join(configs.GetImagesPath(), "browser_diagnostics"), "step-")
	if err != nil {
		if os.MkdirAll(filepath.Join(configs.GetImagesPath(), "browser_diagnostics"), 0700) != nil {
			return d
		}
		dir, err = os.MkdirTemp(filepath.Join(configs.GetImagesPath(), "browser_diagnostics"), "step-")
	}
	if err != nil {
		return d
	}
	_ = os.Chmod(dir, 0700)
	if shot, err := p.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng}); err == nil {
		if masked, err := redactDiagnosticPNG(shot, d.Layout...); err == nil {
			if os.WriteFile(filepath.Join(dir, "screenshot.redacted.png"), masked, 0600) == nil {
				d.Screenshot = "content_redacted_layout_only"
			}
		}
	}
	d.Artifact = filepath.Join(dir, "state.json")
	if data, err := json.MarshalIndent(d, "", "  "); err == nil {
		_ = os.WriteFile(d.Artifact, data, 0600)
	}
	return d
}

// Step/selector labels are code-owned. Unknown/dynamic strings are not saved.
func safeDiagnosticLabel(label string) string {
	for _, r := range label {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._- #[]>:=+()", r) {
			return "omitted"
		}
	}
	if len(label) > 100 {
		return "omitted"
	}
	// Only fixed step namespaces and existing code-owned wait descriptions.
	for _, prefix := range []string{"fixture.", "profile.", "publish.", "user_profile."} {
		if strings.HasPrefix(label, prefix) {
			return strings.TrimSuffix(prefix, ".")
		}
	}
	return "state_or_control"
}
