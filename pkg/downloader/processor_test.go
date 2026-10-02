// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package downloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateLocalImagesRejectsMissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "foo.jpg")
	err := ValidateLocalImages([]string{missing})
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("expected a clear error containing the missing path %q, got %v", missing, err)
	}
}

func TestValidateLocalImagesRejectsNonRegularFile(t *testing.T) {
	dir := t.TempDir()
	err := ValidateLocalImages([]string{dir})
	if err == nil || !strings.Contains(err.Error(), "普通文件") {
		t.Fatalf("expected non-regular file error, got %v", err)
	}
}

func TestValidateLocalImagesAcceptsReadableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLocalImages([]string{path}); err != nil {
		t.Fatalf("expected readable regular file, got %v", err)
	}
}

func TestProcessImagesPreflightsEveryLocalPathBeforeURLDownload(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "foo.jpg")
	processor := NewImageProcessor()
	_, err := processor.ProcessImages([]string{"https://example.invalid/should-not-download.jpg", missing})
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("expected missing local path to fail before network download, got %v", err)
	}
}
