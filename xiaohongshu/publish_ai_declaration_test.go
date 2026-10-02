// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"errors"
	"strings"
	"testing"
)

type fakeAIDeclarationUI struct {
	states          []bool
	openFound       bool
	openErr         error
	optionFound     bool
	optionErr       error
	stateErr        error
	openCalls       int
	selectCalls     int
	stateCheckCalls int
}

func (f *fakeAIDeclarationUI) IsAIContentDeclared() (bool, error) {
	f.stateCheckCalls++
	if f.stateErr != nil {
		return false, f.stateErr
	}
	if len(f.states) == 0 {
		return false, nil
	}
	i := f.stateCheckCalls - 1
	if i >= len(f.states) {
		i = len(f.states) - 1
	}
	return f.states[i], nil
}

func (f *fakeAIDeclarationUI) OpenDeclarationEntry() (bool, error) {
	f.openCalls++
	return f.openFound, f.openErr
}

func (f *fakeAIDeclarationUI) SelectAIContentDeclaration() (bool, error) {
	f.selectCalls++
	return f.optionFound, f.optionErr
}

func TestAIGeneratedFalseDoesNotTouchDeclarationUI(t *testing.T) {
	ui := &fakeAIDeclarationUI{}
	if err := ensureAIGeneratedDeclaration(ui, false); err != nil {
		t.Fatal(err)
	}
	if ui.stateCheckCalls+ui.openCalls+ui.selectCalls != 0 {
		t.Fatalf("disabled AI declaration must not inspect or change the control: %+v", ui)
	}
}

func TestAIGeneratedTrueSelectsAndVerifies(t *testing.T) {
	ui := &fakeAIDeclarationUI{states: []bool{false, true}, openFound: true, optionFound: true}
	if err := ensureAIGeneratedDeclaration(ui, true); err != nil {
		t.Fatal(err)
	}
	if ui.openCalls != 1 || ui.selectCalls != 1 || ui.stateCheckCalls != 2 {
		t.Fatalf("expected open, select, and final state verification; got %+v", ui)
	}
}

func TestAIGeneratedTrueMissingEntryFails(t *testing.T) {
	ui := &fakeAIDeclarationUI{states: []bool{false}}
	err := ensureAIGeneratedDeclaration(ui, true)
	if err == nil || !strings.Contains(err.Error(), "找不到AI声明入口") {
		t.Fatalf("expected missing-entry error, got %v", err)
	}
	if ui.selectCalls != 0 {
		t.Fatal("must not select or continue when declaration entry is missing")
	}
}

func TestAIGeneratedTrueClickFailureStops(t *testing.T) {
	ui := &fakeAIDeclarationUI{states: []bool{false}, openFound: true, openErr: errors.New("click rejected")}
	err := ensureAIGeneratedDeclaration(ui, true)
	if err == nil || !strings.Contains(err.Error(), "点击AI声明入口失败") {
		t.Fatalf("expected click-failure error, got %v", err)
	}
	if ui.selectCalls != 0 {
		t.Fatal("must not continue after the entry click fails")
	}
}

func TestAIGeneratedTrueUnconfirmedStateStops(t *testing.T) {
	ui := &fakeAIDeclarationUI{states: []bool{false, false}, openFound: true, optionFound: true}
	err := ensureAIGeneratedDeclaration(ui, true)
	if err == nil || !strings.Contains(err.Error(), "AI声明点击后状态未生效") {
		t.Fatalf("expected ineffective-state error, got %v", err)
	}
}
