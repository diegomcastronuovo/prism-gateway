package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// ── geminiGenerateContentWriter ───────────────────────────────────────────────
//
// geminiGenerateContentWriter wraps an http.ResponseWriter and intercepts every
// Write() call made by writeSSEStream. It parses Chat-format SSE events
// (data: <json>\n\n and data: [DONE]\n\n) and translates them into Gemini
// streaming SSE chunks:
//
//   - Per delta: data: {"candidates":[{"content":{"role":"model","parts":[{"text":"..."}]},"index":0}]}\n\n
//   - On [DONE]:  final chunk with usageMetadata then data: [DONE]\n\n
//
// No event: prefix is used — Gemini streaming uses only data: lines.
//
// The writer satisfies both http.ResponseWriter and ResponseSink.

type geminiGenerateContentWriter struct {
	w             http.ResponseWriter
	modelName     string
	buf           []byte        // partial-event accumulator
	completedOnce atomic.Bool   // guards against double [DONE] emission

	// token counts captured from the last usage-bearing chunk
	promptTokens     int
	candidatesTokens int
	totalTokens      int
}

func newGeminiGenerateContentWriter(w http.ResponseWriter, modelName string) *geminiGenerateContentWriter {
	return &geminiGenerateContentWriter{
		w:         w,
		modelName: modelName,
	}
}

// Header implements http.ResponseWriter.
func (gw *geminiGenerateContentWriter) Header() http.Header {
	return gw.w.Header()
}

// WriteHeader implements http.ResponseWriter.
func (gw *geminiGenerateContentWriter) WriteHeader(code int) {
	gw.w.WriteHeader(code)
}

// Write implements http.ResponseWriter. It buffers bytes until it sees a
// complete SSE event (terminated by \n\n) and then translates it.
func (gw *geminiGenerateContentWriter) Write(p []byte) (int, error) {
	gw.buf = append(gw.buf, p...)
	for {
		idx := bytes.Index(gw.buf, []byte("\n\n"))
		if idx < 0 {
			break
		}
		event := gw.buf[:idx]
		gw.buf = gw.buf[idx+2:]
		if err := gw.handleSSEEvent(event); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

// handleSSEEvent processes one complete SSE event (without the trailing \n\n).
func (gw *geminiGenerateContentWriter) handleSSEEvent(event []byte) error {
	line := bytes.TrimPrefix(bytes.TrimSpace(event), []byte("data: "))

	if bytes.Equal(line, []byte("[DONE]")) {
		return gw.emitFinalChunk()
	}

	// Attempt to parse the Chat chunk JSON.
	var chunk streamChunkPayload
	if err := json.Unmarshal(line, &chunk); err != nil {
		// Unrecognised chunk — pass through silently.
		return nil
	}

	// Also try to capture usage from the chunk if available (extended payload).
	var usageChunk struct {
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(line, &usageChunk) == nil && usageChunk.Usage != nil {
		gw.promptTokens = usageChunk.Usage.PromptTokens
		gw.candidatesTokens = usageChunk.Usage.CompletionTokens
		gw.totalTokens = usageChunk.Usage.TotalTokens
	}

	deltaText := ""
	if len(chunk.Choices) > 0 {
		deltaText = chunk.Choices[0].Delta.Content
	}

	return gw.emitDeltaChunk(deltaText)
}

// emitDeltaChunk emits a partial Gemini SSE chunk for a single delta.
func (gw *geminiGenerateContentWriter) emitDeltaChunk(text string) error {
	payload := map[string]interface{}{
		"candidates": []map[string]interface{}{
			{
				"content": map[string]interface{}{
					"role":  "model",
					"parts": []map[string]interface{}{{"text": text}},
				},
				"index": 0,
			},
		},
	}
	return gw.writeDataLine(payload)
}

// emitFinalChunk emits the terminal Gemini SSE chunk with usageMetadata and finishReason STOP,
// followed by the [DONE] sentinel.
func (gw *geminiGenerateContentWriter) emitFinalChunk() error {
	if !gw.completedOnce.CompareAndSwap(false, true) {
		return nil
	}

	payload := map[string]interface{}{
		"candidates": []map[string]interface{}{
			{
				"content": map[string]interface{}{
					"role":  "model",
					"parts": []map[string]interface{}{{"text": ""}},
				},
				"finishReason": "STOP",
				"index":        0,
			},
		},
		"usageMetadata": map[string]interface{}{
			"promptTokenCount":     gw.promptTokens,
			"candidatesTokenCount": gw.candidatesTokens,
			"totalTokenCount":      gw.totalTokens,
		},
	}
	if err := gw.writeDataLine(payload); err != nil {
		return err
	}

	_, err := fmt.Fprintf(gw.w, "data: [DONE]\n\n")
	if f, ok := gw.w.(http.Flusher); ok {
		f.Flush()
	}
	return err
}

func (gw *geminiGenerateContentWriter) writeDataLine(payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(gw.w, "data: %s\n\n", data)
	if f, ok := gw.w.(http.Flusher); ok {
		f.Flush()
	}
	return err
}

// ── ResponseSink interface compliance ─────────────────────────────────────────
//
// The orchestrator's main streaming path writes directly to OrchestratorInput.W
// (which is this geminiGenerateContentWriter), bypassing ResponseSink. These
// methods exist for interface compliance only.

// WriteChunk implements ResponseSink (interface compliance; not called by orchestrator streaming path).
func (gw *geminiGenerateContentWriter) WriteChunk(_ providers.StreamEvent) error {
	return nil
}

// WriteDone implements ResponseSink (interface compliance; not called by orchestrator streaming path).
// Defensively emits the final chunk if the [DONE] sentinel was never seen in Write().
func (gw *geminiGenerateContentWriter) WriteDone(_ *providers.Usage, _ string) error {
	return gw.emitFinalChunk()
}

// WriteError implements ResponseSink (interface compliance).
func (gw *geminiGenerateContentWriter) WriteError(_ error) {}

// ExtraHeaders implements ResponseSink.
func (gw *geminiGenerateContentWriter) ExtraHeaders() map[string]string {
	return map[string]string{
		"X-Selected-Model": gw.modelName,
	}
}
