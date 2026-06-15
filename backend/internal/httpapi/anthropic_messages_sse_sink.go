package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// ── anthropicMessagesWriter ───────────────────────────────────────────────────
//
// anthropicMessagesWriter wraps an http.ResponseWriter and intercepts every
// Write() call made by writeSSEStream. It parses Chat-format SSE events
// (data: <json>\n\n and data: [DONE]\n\n) and translates them into the
// Anthropic Messages API SSE event sequence:
//
//  1. message_start          (once, on first Write)
//  2. content_block_start    (once, on first Write)
//  3. ping                   (once, on first Write)
//  4. content_block_delta    (per content chunk — delta.type: "text_delta")
//  5. content_block_stop     (on [DONE])
//  6. message_delta          (on [DONE] — contains stop_reason and output token count)
//  7. message_stop           (on [DONE])
//
// The writer satisfies both http.ResponseWriter and ResponseSink so it can be
// passed as OrchestratorInput.W and OrchestratorInput.Sink.
//
// All state is per-request; no shared mutable state.

type anthropicMessagesWriter struct {
	w               http.ResponseWriter
	modelName       string
	msgID           string
	buf             []byte          // partial-event accumulator
	firstWriteDone  bool            // message_start + content_block_start + ping emitted
	accumulatedText strings.Builder // for output token count in message_delta
	outputTokens    int             // populated from usage in [DONE] chunk when available
	completedOnce   atomic.Bool     // guards against double message_stop
}

func newAnthropicMessagesWriter(w http.ResponseWriter, modelName string) *anthropicMessagesWriter {
	return &anthropicMessagesWriter{
		w:         w,
		modelName: modelName,
		msgID:     fmt.Sprintf("msg_%d", time.Now().UnixNano()),
	}
}

// Header implements http.ResponseWriter.
func (aw *anthropicMessagesWriter) Header() http.Header {
	return aw.w.Header()
}

// WriteHeader implements http.ResponseWriter.
func (aw *anthropicMessagesWriter) WriteHeader(code int) {
	aw.w.WriteHeader(code)
}

// Write implements http.ResponseWriter. It buffers bytes until it sees a
// complete SSE event (terminated by \n\n) and then translates it.
func (aw *anthropicMessagesWriter) Write(p []byte) (int, error) {
	aw.buf = append(aw.buf, p...)
	for {
		// SSE events end with \n\n.
		idx := bytes.Index(aw.buf, []byte("\n\n"))
		if idx < 0 {
			break
		}
		event := aw.buf[:idx]
		aw.buf = aw.buf[idx+2:]
		if err := aw.handleSSEEvent(event); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

// handleSSEEvent processes one complete SSE event (without the trailing \n\n).
func (aw *anthropicMessagesWriter) handleSSEEvent(event []byte) error {
	// Strip the "data: " prefix.
	line := bytes.TrimPrefix(bytes.TrimSpace(event), []byte("data: "))

	if bytes.Equal(line, []byte("[DONE]")) {
		return aw.emitDone()
	}

	// Attempt to parse the Chat chunk JSON.
	var chunk streamChunkPayload
	if err := json.Unmarshal(line, &chunk); err != nil {
		// Not a recognised chunk — pass through silently (e.g. comment lines).
		return nil
	}

	// Extract delta content from the first choice.
	deltaText := ""
	if len(chunk.Choices) > 0 {
		deltaText = chunk.Choices[0].Delta.Content
	}

	// On the first content-bearing write, emit message_start + content_block_start + ping.
	if !aw.firstWriteDone {
		aw.firstWriteDone = true
		if err := aw.emitMessageStart(); err != nil {
			return err
		}
		if err := aw.emitContentBlockStart(); err != nil {
			return err
		}
		if err := aw.emitPing(); err != nil {
			return err
		}
	}

	// Accumulate text for output token estimation.
	aw.accumulatedText.WriteString(deltaText)

	return aw.emitDelta(deltaText)
}

// ── SSE event emitters ────────────────────────────────────────────────────────

func (aw *anthropicMessagesWriter) emitMessageStart() error {
	return aw.writeSSEEvent("message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":    aw.msgID,
			"type":  "message",
			"role":  "assistant",
			"model": aw.modelName,
			"usage": map[string]interface{}{
				"input_tokens": 0,
			},
		},
	})
}

func (aw *anthropicMessagesWriter) emitContentBlockStart() error {
	return aw.writeSSEEvent("content_block_start", map[string]interface{}{
		"type":  "content_block_start",
		"index": 0,
		"content_block": map[string]interface{}{
			"type": "text",
			"text": "",
		},
	})
}

func (aw *anthropicMessagesWriter) emitPing() error {
	return aw.writeSSEEvent("ping", map[string]interface{}{
		"type": "ping",
	})
}

func (aw *anthropicMessagesWriter) emitDelta(delta string) error {
	return aw.writeSSEEvent("content_block_delta", map[string]interface{}{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]interface{}{
			"type": "text_delta",
			"text": delta,
		},
	})
}

func (aw *anthropicMessagesWriter) emitDone() error {
	// Ensure message_start + content_block_start + ping fire even if no deltas were received.
	if !aw.firstWriteDone {
		aw.firstWriteDone = true
		if err := aw.emitMessageStart(); err != nil {
			return err
		}
		if err := aw.emitContentBlockStart(); err != nil {
			return err
		}
		if err := aw.emitPing(); err != nil {
			return err
		}
	}

	if err := aw.writeSSEEvent("content_block_stop", map[string]interface{}{
		"type":  "content_block_stop",
		"index": 0,
	}); err != nil {
		return err
	}

	outputTokens := aw.outputTokens
	if outputTokens == 0 {
		// Rough estimation: 1 token ≈ 4 chars.
		outputTokens = (aw.accumulatedText.Len() + 3) / 4
	}

	if err := aw.writeSSEEvent("message_delta", map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
		},
		"usage": map[string]interface{}{
			"output_tokens": outputTokens,
		},
	}); err != nil {
		return err
	}

	return aw.emitMessageStop()
}

func (aw *anthropicMessagesWriter) emitMessageStop() error {
	if !aw.completedOnce.CompareAndSwap(false, true) {
		return nil // already emitted
	}
	return aw.writeSSEEvent("message_stop", map[string]interface{}{
		"type": "message_stop",
	})
}

func (aw *anthropicMessagesWriter) writeSSEEvent(eventType string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(aw.w, "event: %s\ndata: %s\n\n", eventType, data)
	if f, ok := aw.w.(http.Flusher); ok {
		f.Flush()
	}
	return err
}

// ── ResponseSink interface compliance ─────────────────────────────────────────
//
// The orchestrator's main streaming path writes directly to OrchestratorInput.W
// (which is this anthropicMessagesWriter), bypassing ResponseSink. These methods
// exist for interface compliance only.

// WriteChunk implements ResponseSink (interface compliance; not called by orchestrator streaming path).
func (aw *anthropicMessagesWriter) WriteChunk(_ providers.StreamEvent) error {
	return nil
}

// WriteDone implements ResponseSink (interface compliance; not called by orchestrator streaming path).
// Defensively emits message_stop if the [DONE] sentinel was never seen in Write().
func (aw *anthropicMessagesWriter) WriteDone(_ *providers.Usage, _ string) error {
	return aw.emitMessageStop()
}

// WriteError implements ResponseSink (interface compliance).
func (aw *anthropicMessagesWriter) WriteError(_ error) {}

// ExtraHeaders implements ResponseSink.
func (aw *anthropicMessagesWriter) ExtraHeaders() map[string]string {
	return map[string]string{
		"X-Selected-Model": aw.modelName,
	}
}
