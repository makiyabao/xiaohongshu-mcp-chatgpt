// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package main

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO7+2X8AAAAASUVORK5CYII="

func TestDownloadFeedImageReturnsDetectedMimeAndDimensions(t *testing.T) {
	png, err := base64.StdEncoding.DecodeString(tinyPNGBase64)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain") // 检测必须以二进制内容为准，不信任响应头。
		_, _ = w.Write(png)
	}))
	defer server.Close()

	result, err := downloadFeedImage(context.Background(), server.URL+"/image", server.Client())
	if err != nil {
		t.Fatalf("downloadFeedImage() error = %v", err)
	}
	if result.mime != "image/png" || result.width != 1 || result.height != 1 {
		t.Fatalf("unexpected image metadata: mime=%s size=%dx%d", result.mime, result.width, result.height)
	}
}

func TestDownloadFeedImageRejectsNonHTTPURL(t *testing.T) {
	for _, tc := range []struct{ url, message string }{
		{"file:///tmp/image.png", "图片地址格式无效"},
		{"ftp://example.com/image.png", "仅允许 http 或 https"},
	} {
		_, err := downloadFeedImage(context.Background(), tc.url, newFeedImageHTTPClient())
		if err == nil || !strings.Contains(err.Error(), tc.message) {
			t.Fatalf("expected rejection for %q, got %v", tc.url, err)
		}
	}
}

func TestDownloadFeedImageRejectsOversizeResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "15728641")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := downloadFeedImage(context.Background(), server.URL+"/image", server.Client())
	if err == nil || !strings.Contains(err.Error(), "超过 15MB") {
		t.Fatalf("expected size rejection, got %v", err)
	}
}
