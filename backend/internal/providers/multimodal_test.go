package providers

import (
	"encoding/json"
	"testing"
)

// ── ChatMessage.MarshalJSON ───────────────────────────────────────────────────

func TestChatMessage_MarshalJSON_TextOnly(t *testing.T) {
	msg := ChatMessage{Role: "user", Content: "hello"}
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var got map[string]interface{}
	json.Unmarshal(b, &got)

	if got["role"] != "user" {
		t.Errorf("role: want user, got %v", got["role"])
	}
	if got["content"] != "hello" {
		t.Errorf("content: want string 'hello', got %T %v", got["content"], got["content"])
	}
}

func TestChatMessage_MarshalJSON_Multimodal(t *testing.T) {
	msg := ChatMessage{
		Role:    "user",
		Content: "describe this",
		ContentBlocks: []MessageContentBlock{
			{Type: "text", Text: "describe this"},
			{Type: "image_url", ImageURL: &ImageURLData{URL: "https://example.com/img.jpg"}},
		},
	}
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var got map[string]interface{}
	json.Unmarshal(b, &got)

	blocks, ok := got["content"].([]interface{})
	if !ok {
		t.Fatalf("content should be an array for multimodal, got %T", got["content"])
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(blocks))
	}

	// First block: text
	textBlock := blocks[0].(map[string]interface{})
	if textBlock["type"] != "text" || textBlock["text"] != "describe this" {
		t.Errorf("unexpected text block: %v", textBlock)
	}

	// Second block: image_url
	imgBlock := blocks[1].(map[string]interface{})
	if imgBlock["type"] != "image_url" {
		t.Errorf("expected image_url block, got type=%v", imgBlock["type"])
	}
	imgURLData := imgBlock["image_url"].(map[string]interface{})
	if imgURLData["url"] != "https://example.com/img.jpg" {
		t.Errorf("unexpected image url: %v", imgURLData["url"])
	}
}

// ── Anthropic translateRequest ────────────────────────────────────────────────

func TestAnthropic_TranslateRequest_TextOnly(t *testing.T) {
	a := NewAnthropic("https://api.anthropic.com", "key")
	req := ChatRequest{
		Model: "claude-3-5-sonnet",
		Messages: []ChatMessage{
			{Role: "user", Content: "hello"},
		},
	}

	anthReq := a.translateRequest(req)

	if len(anthReq.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(anthReq.Messages))
	}

	// Content should be a JSON string (not an array)
	var content interface{}
	json.Unmarshal(anthReq.Messages[0].Content, &content)
	if _, ok := content.(string); !ok {
		t.Errorf("text-only message should have string content, got %T: %v", content, content)
	}
}

func TestAnthropic_TranslateRequest_Multimodal(t *testing.T) {
	a := NewAnthropic("https://api.anthropic.com", "key")
	req := ChatRequest{
		Model: "claude-3-5-sonnet",
		Messages: []ChatMessage{
			{
				Role:    "user",
				Content: "what is this?",
				ContentBlocks: []MessageContentBlock{
					{Type: "text", Text: "what is this?"},
					{Type: "image_url", ImageURL: &ImageURLData{URL: "https://example.com/cat.jpg"}},
				},
			},
		},
	}

	anthReq := a.translateRequest(req)

	if len(anthReq.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(anthReq.Messages))
	}

	// Content should be a JSON array of blocks
	var blocks []map[string]interface{}
	if err := json.Unmarshal(anthReq.Messages[0].Content, &blocks); err != nil {
		t.Fatalf("content should be a JSON array for multimodal, unmarshal error: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 content blocks, got %d: %s", len(blocks), anthReq.Messages[0].Content)
	}

	// Text block
	if blocks[0]["type"] != "text" || blocks[0]["text"] != "what is this?" {
		t.Errorf("unexpected text block: %v", blocks[0])
	}

	// Image block (Anthropic format: type=image, source.type=url)
	if blocks[1]["type"] != "image" {
		t.Errorf("expected type=image for Anthropic image block, got %v", blocks[1]["type"])
	}
	source := blocks[1]["source"].(map[string]interface{})
	if source["type"] != "url" || source["url"] != "https://example.com/cat.jpg" {
		t.Errorf("unexpected image source: %v", source)
	}
}

func TestAnthropic_TranslateRequest_MultipleImages(t *testing.T) {
	a := NewAnthropic("https://api.anthropic.com", "key")
	req := ChatRequest{
		Model: "claude-3-5-sonnet",
		Messages: []ChatMessage{
			{
				Role:    "user",
				Content: "compare",
				ContentBlocks: []MessageContentBlock{
					{Type: "text", Text: "compare"},
					{Type: "image_url", ImageURL: &ImageURLData{URL: "https://example.com/a.jpg"}},
					{Type: "image_url", ImageURL: &ImageURLData{URL: "https://example.com/b.jpg"}},
				},
			},
		},
	}

	anthReq := a.translateRequest(req)
	var blocks []map[string]interface{}
	json.Unmarshal(anthReq.Messages[0].Content, &blocks)

	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks (1 text + 2 images), got %d", len(blocks))
	}
	if blocks[1]["type"] != "image" || blocks[2]["type"] != "image" {
		t.Errorf("expected both image blocks to have type=image")
	}
}

// ── Gemini translateRequest ───────────────────────────────────────────────────

func TestGemini_TranslateRequest_TextOnly(t *testing.T) {
	g := NewGemini("https://generativelanguage.googleapis.com/v1beta", "key")
	req := ChatRequest{
		Model: "gemini-1.5-flash",
		Messages: []ChatMessage{
			{Role: "user", Content: "hello"},
		},
	}

	gemReq := g.translateRequest(req)

	if len(gemReq.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(gemReq.Contents))
	}
	parts := gemReq.Contents[0].Parts
	if len(parts) != 1 || parts[0].Text != "hello" {
		t.Errorf("expected single text part 'hello', got %+v", parts)
	}
	if parts[0].FileData != nil {
		t.Error("text-only message should have nil FileData")
	}
}

func TestGemini_TranslateRequest_Multimodal(t *testing.T) {
	g := NewGemini("https://generativelanguage.googleapis.com/v1beta", "key")
	req := ChatRequest{
		Model: "gemini-1.5-flash",
		Messages: []ChatMessage{
			{
				Role:    "user",
				Content: "describe this",
				ContentBlocks: []MessageContentBlock{
					{Type: "text", Text: "describe this"},
					{Type: "image_url", ImageURL: &ImageURLData{URL: "https://example.com/photo.jpg"}},
				},
			},
		},
	}

	gemReq := g.translateRequest(req)

	if len(gemReq.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(gemReq.Contents))
	}
	parts := gemReq.Contents[0].Parts
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts (text + file_data), got %d: %+v", len(parts), parts)
	}

	// Text part
	if parts[0].Text != "describe this" || parts[0].FileData != nil {
		t.Errorf("unexpected text part: %+v", parts[0])
	}

	// Image part (Gemini format: file_data.file_uri)
	if parts[1].FileData == nil {
		t.Fatalf("expected file_data part for image, got nil")
	}
	if parts[1].FileData.FileURI != "https://example.com/photo.jpg" {
		t.Errorf("unexpected file_uri: %v", parts[1].FileData.FileURI)
	}
	if parts[1].Text != "" {
		t.Errorf("image part should have empty text, got %q", parts[1].Text)
	}
}

func TestGemini_TranslateRequest_MultipleImages(t *testing.T) {
	g := NewGemini("https://generativelanguage.googleapis.com/v1beta", "key")
	req := ChatRequest{
		Model: "gemini-1.5-flash",
		Messages: []ChatMessage{
			{
				Role:    "user",
				Content: "compare",
				ContentBlocks: []MessageContentBlock{
					{Type: "text", Text: "compare"},
					{Type: "image_url", ImageURL: &ImageURLData{URL: "https://example.com/a.jpg"}},
					{Type: "image_url", ImageURL: &ImageURLData{URL: "https://example.com/b.jpg"}},
				},
			},
		},
	}

	gemReq := g.translateRequest(req)
	parts := gemReq.Contents[0].Parts

	if len(parts) != 3 {
		t.Fatalf("expected 3 parts (1 text + 2 images), got %d", len(parts))
	}
	if parts[1].FileData.FileURI != "https://example.com/a.jpg" {
		t.Errorf("unexpected first image URI: %v", parts[1].FileData.FileURI)
	}
	if parts[2].FileData.FileURI != "https://example.com/b.jpg" {
		t.Errorf("unexpected second image URI: %v", parts[2].FileData.FileURI)
	}
}
