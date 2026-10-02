// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package downloader

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidatePublicHTTPSURLRejectsUnsafeSchemesAndHosts(t *testing.T) {
	for _, raw := range []string{
		"file:///etc/passwd",
		"http://example.com/image.jpg",
		"https://127.0.0.1/image.jpg",
		"https://169.254.169.254/latest/meta-data",
	} {
		if err := validatePublicHTTPSURL(context.Background(), raw); err == nil {
			t.Errorf("expected URL to be rejected: %q", raw)
		}
	}
}

func TestStagedImageStoreOnlyResolvesKnownUnexpiredHandles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chatgpt_test.jpg")
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &StagedImageStore{dir: dir, items: map[string]stagedImage{
		"img_valid": {path: path, expiresAt: time.Now().Add(time.Hour)},
	}}
	paths, err := store.Resolve([]string{"img_valid"})
	if err != nil || len(paths) != 1 || paths[0] != path {
		t.Fatalf("expected known handle to resolve to staged path, got %#v, %v", paths, err)
	}
	if _, err := store.Resolve([]string{"../../etc/passwd"}); err == nil {
		t.Fatal("arbitrary path-like value must not resolve")
	}
	if _, err := store.Resolve([]string{"img_missing"}); err == nil {
		t.Fatal("unknown handle must not resolve")
	}
}

func TestStagedImageStoreCleanupRemovesExpiredFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chatgpt_expired.jpg")
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &StagedImageStore{dir: dir, items: map[string]stagedImage{
		"img_expired": {path: path, expiresAt: time.Now().Add(-time.Second)},
	}}
	store.cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected expired staged image to be removed, stat error=%v", err)
	}
}
