// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFeedImageResultKeepsImageOutOfStructuredContent(t *testing.T) {
	encodedImage := tinyPNGBase64
	result := &MCPToolResult{
		Content: []MCPContent{
			{Type: "text", Text: "第 1 张，共 1 张；MIME: image/png"},
			{Type: "image", MimeType: "image/png", Data: encodedImage},
		},
		StructuredContent: map[string]any{"imageIndex": 1, "imageCount": 1, "width": 1, "height": 1},
		Meta:              map[string]any{feedImageWidgetResultMetaKey: map[string]any{"imageContentIndex": 1}},
	}

	converted := convertToMCPResult(result)
	if image, ok := converted.Content[1].(*mcp.ImageContent); !ok || image.MIMEType != "image/png" {
		t.Fatal("standard MCP ImageContent was not preserved")
	}
	if got := converted.Meta[feedImageWidgetResultMetaKey].(map[string]any)["imageContentIndex"]; got != 1 {
		t.Fatalf("unexpected widget image pointer: %v", got)
	}

	wire, err := json.Marshal(converted)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(wire), encodedImage); count != 1 {
		t.Fatalf("image base64 appears %d times in MCP response, want 1", count)
	}
	if strings.Contains(string(wire), "\"base64\"") {
		t.Fatal("structuredContent must not contain a base64 field")
	}
	if _, err := base64.StdEncoding.DecodeString(encodedImage); err != nil {
		t.Fatal(err)
	}
}

func TestModelOnlyFeedImageResultRemovesWidgetMetadata(t *testing.T) {
	withWidget := &MCPToolResult{
		Content: []MCPContent{
			{Type: "text", Text: "第 1 张，共 1 张；MIME: image/png"},
			{Type: "image", MimeType: "image/png", Data: tinyPNGBase64},
		},
		StructuredContent: map[string]any{"imageIndex": 1, "imageCount": 1, "width": 1, "height": 1},
		Meta:              map[string]any{feedImageWidgetResultMetaKey: map[string]any{"imageContentIndex": 1}},
	}

	modelOnly := modelOnlyFeedImageResult(withWidget)
	if modelOnly == withWidget {
		t.Fatal("model-only result must not mutate the display result")
	}
	if modelOnly.Meta != nil {
		t.Fatal("model-only result must not expose widget metadata")
	}
	converted := convertToMCPResult(modelOnly)
	if _, ok := converted.Content[1].(*mcp.ImageContent); !ok {
		t.Fatal("model-only result must retain standard MCP ImageContent")
	}
	if len(converted.Meta) != 0 {
		t.Fatal("model-only converted result must not include widget metadata")
	}
}
