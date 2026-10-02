// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The URI is a widget cache key. Bump it when the HTML/JS contract changes.
const feedImageWidgetURI = "ui://xiaohongshu/feed-image-v2.html"

// registerFeedImageWidget exposes a self-contained MCP Apps widget. The widget
// receives only structured tool output and never fetches external image URLs.
func registerFeedImageWidget(server *mcp.Server) {
	server.AddResource(
		&mcp.Resource{
			URI:      feedImageWidgetURI,
			Name:     "feed-image-widget",
			Title:    "小红书图片查看器",
			MIMEType: "text/html;profile=mcp-app",
		},
		func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
				URI:      feedImageWidgetURI,
				MIMEType: "text/html;profile=mcp-app",
				Text:     feedImageWidgetHTML,
				Meta:     mcp.Meta{"ui": map[string]any{"prefersBorder": true}},
			}}}, nil
		},
	)
}

const feedImageWidgetHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <style>
    :root { color-scheme: light dark; }
    body { margin: 0; font: 14px/1.5 system-ui, -apple-system, "Segoe UI", sans-serif; color: CanvasText; background: Canvas; }
    main { padding: 12px; }
    #photo { display: none; width: 100%; max-height: 620px; object-fit: contain; border-radius: 10px; background: #f2f2f2; }
    #meta { margin-top: 8px; color: #666; }
    #status { color: #666; }
  </style>
</head>
<body>
  <main>
    <div id="status">正在等待图片结果…</div>
    <img id="photo" alt="小红书笔记图片">
    <div id="meta" aria-live="polite"></div>
  </main>
  <script>
    (() => {
      const photo = document.getElementById("photo");
      const meta = document.getElementById("meta");
      const status = document.getElementById("status");
      let objectURL = null;
      const initializeRequestId = 1;
      let initialized = false;

      function base64ToBlob(base64, mimeType) {
        const binary = atob(base64);
        const bytes = new Uint8Array(binary.length);
        for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
        return new Blob([bytes], { type: mimeType });
      }

      function toolResultEnvelope(notificationResult) {
        if (notificationResult && Array.isArray(notificationResult.content)) return notificationResult;
        const metadata = window.openai && window.openai.toolResponseMetadata;
        if (!metadata || typeof metadata !== "object") return notificationResult || null;
        return metadata.mcp_tool_result || metadata.call_tool_result || metadata;
      }

      function imageFromResult(result) {
        const meta = result && result._meta;
        const pointer = meta && meta["xiaohongshu/feed-image"];
        const index = pointer && pointer.imageContentIndex;
        const content = result && result.content;
        const image = Array.isArray(content) && Number.isInteger(index) ? content[index] : null;
        return image && image.type === "image" && image.data && image.mimeType ? image : null;
      }

      function render(structuredContent, notificationResult) {
        const image = imageFromResult(toolResultEnvelope(notificationResult));
        if (!image) {
          status.textContent = "未收到可显示的图片数据。";
          return;
        }
        try {
          const nextURL = URL.createObjectURL(base64ToBlob(image.data, image.mimeType));
          if (objectURL) URL.revokeObjectURL(objectURL);
          objectURL = nextURL;
          photo.onload = () => { status.style.display = "none"; photo.style.display = "block"; };
          photo.onerror = () => { status.style.display = "block"; status.textContent = "图片无法在当前组件中显示。"; };
          photo.src = nextURL;
          const size = structuredContent && structuredContent.width > 0 && structuredContent.height > 0 ? "；" + structuredContent.width + "×" + structuredContent.height : "";
          meta.textContent = "第 " + ((structuredContent && structuredContent.imageIndex) || 1) + " 张 / 共 " + ((structuredContent && structuredContent.imageCount) || 1) + " 张" + size;
        } catch (_) {
          status.textContent = "图片数据处理失败。";
        }
      }

      window.addEventListener("message", (event) => {
        if (event.source !== window.parent) return;
        const message = event.data;
        if (!message || message.jsonrpc !== "2.0") return;
        if (message.id === initializeRequestId && !initialized) {
          if (message.error) {
            status.textContent = "图片组件初始化失败。";
            return;
          }
          initialized = true;
          window.parent.postMessage({
            jsonrpc: "2.0",
            method: "ui/notifications/initialized"
          }, "*");
          return;
        }
        if (message.method === "ui/notifications/tool-result") render(message.params && message.params.structuredContent, message.params);
      }, { passive: true });

      window.parent.postMessage({
        jsonrpc: "2.0",
        id: initializeRequestId,
        method: "ui/initialize",
        params: {
          appInfo: { name: "xiaohongshu-feed-image", version: "1.0.0" },
          appCapabilities: {},
          protocolVersion: "2026-01-26"
        }
      }, "*");

      if (window.openai && window.openai.toolOutput) render(window.openai.toolOutput, null);
      window.addEventListener("beforeunload", () => { if (objectURL) URL.revokeObjectURL(objectURL); });
    })();
  </script>
</body>
</html>`
