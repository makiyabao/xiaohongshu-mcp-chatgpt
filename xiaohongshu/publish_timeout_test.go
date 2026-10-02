// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package xiaohongshu

import (
	"context"
	"testing"
	"time"
)

func TestUploadWaitBudgetFitsPublishDeadline(t *testing.T) {
	longCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	budget, err := uploadWaitBudget(longCtx, 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if budget != 60*time.Second {
		t.Fatalf("five-minute publish context should permit the 60s per-image wait, got %s", budget)
	}

	shortCtx, shortCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shortCancel()
	shortBudget, err := uploadWaitBudget(shortCtx, 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if shortBudget <= 0 || shortBudget >= 10*time.Second {
		t.Fatalf("per-image wait should be clipped to remaining parent deadline, got %s", shortBudget)
	}
}
