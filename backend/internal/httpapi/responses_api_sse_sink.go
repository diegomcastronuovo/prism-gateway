package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// ── responsesAPIWriter ────────────────────────────────────────────────────────
//
// responsesAPIWriter wraps an http.ResponseWriter and intercepts every
// Write() call made by writeSSEStream. It parses Chat-format SSE events
// (data: <json>\n\n and data: [DONE]\n\n) and translates them into the
// Responses API SSE event sequence:
//
//  1. response.created       (once, on first Write)
//  2. response.output_item.added (once, on first Write)
//  3. response.output_text.delta (per content chunk)
//  4. response.output_text.done  (on [DONE])
//  5. response.completed         (on [DONE])
//
// The writer satisfies both http.ResponseWriter and ResponseSink so it can
// be passed as OrchestratorInput.W and OrchestratorInput.Sink.
//
// All state is per-request; no shared mutable state.

type responsesAPIWriter struct {
	w              http.ResponseWriter
	modelName      string
	itemID         string
	buf            []byte          // partial-event accumulator
	firstWriteDone bool            // response.created + output_item.added emitted
	accumulatedText strings.Builder // for response.output_text.done
	completedOnce  atomic.Bool    // guards defensively against double response.completed
}

func newResponsesAPIWriter(w http.ResponseWriter, modelName string) *responsesAPIWriter {
	return &responsesAPIWriter{
		w:         w,
		modelName: modelName,
		itemID:    "msg_" + generateShortID(),
	}
}

// generateShortID returns a short pseudo-unique string for item IDs.
// Uses a monotonic counter to avoid importing crypto/rand.
var shortIDCounter atomic.Int64

func generateShortID() string {
	n := shortIDCounter.Add(1)
	return fmt.Sprintf("%016x", n)
}

// Header implements http.ResponseWriter.
func (rw *responsesAPIWriter) Header() http.Header {
	return rw.w.Header()
}

// WriteHeader implements http.ResponseWriter.
func (rw *responsesAPIWriter) WriteHeader(code int) {
	rw.w.WriteHeader(code)
}

// Write implements http.ResponseWriter. It buffers bytes until it sees a
// complete SSE event (terminated by \n\n) and then translates it.
func (rw *responsesAPIWriter) Write(p []byte) (int, error) {
	rw.buf = append(rw.buf, p...)
	for {
		// SSE events end with \n\n.
		idx := bytes.Index(rw.buf, []byte("\n\n"))
		if idx < 0 {
			break
		}
		event := rw.buf[:idx]
		rw.buf = rw.buf[idx+2:]
		if err := rw.handleSSEEvent(event); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

// handleSSEEvent processes one complete SSE event (without the trailing \n\n).
func (rw *responsesAPIWriter) handleSSEEvent(event []byte) error {
	// Strip the "data: " prefix.
	line := bytes.TrimPrefix(bytes.TrimSpace(event), []byte("data: "))

	if bytes.Equal(line, []byte("[DONE]")) {
		return rw.emitDone()
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

	// On the first content-bearing write, emit response.created + output_item.added.
	if !rw.firstWriteDone {
		rw.firstWriteDone = true
		if err := rw.emitCreated(); err != nil {
			return err
		}
		if err := rw.emitOutputItemAdded(); err != nil {
			return err
		}
	}

	// Accumulate text for the done event.
	rw.accumulatedText.WriteString(deltaText)

	return rw.emitDelta(deltaText)
}

// ── SSE event emitters ────────────────────────────────────────────────────────

func (rw *responsesAPIWriter) emitCreated() error {
	return rw.writeSSEEvent("response.created", map[string]interface{}{
		"type":   "response.created",
		"status": "in_progress",
	})
}

func (rw *responsesAPIWriter) emitOutputItemAdded() error {
	return rw.writeSSEEvent("response.output_item.added", map[string]interface{}{
		"type":         "response.output_item.added",
		"output_index": 0,
		"item": map[string]interface{}{
			"type":    "message",
			"id":      rw.itemID,
			"role":    "assistant",
			"status":  "in_progress",
			"content": []interface{}{},
		},
	})
}

func (rw *responsesAPIWriter) emitDelta(delta string) error {
	return rw.writeSSEEvent("response.output_text.delta", map[string]interface{}{
		"type":          "response.output_text.delta",
		"item_id":       rw.itemID,
		"output_index":  0,
		"content_index": 0,
		"delta":         delta,
	})
}

func (rw *responsesAPIWriter) emitDone() error {
	// Ensure response.created + output_item.added fire even if no deltas were received.
	if !rw.firstWriteDone {
		rw.firstWriteDone = true
		if err := rw.emitCreated(); err != nil {
			return err
		}
		if err := rw.emitOutputItemAdded(); err != nil {
			return err
		}
	}

	if err := rw.writeSSEEvent("response.output_text.done", map[string]interface{}{
		"type":          "response.output_text.done",
		"item_id":       rw.itemID,
		"output_index":  0,
		"content_index": 0,
		"text":          rw.accumulatedText.String(),
	}); err != nil {
		return err
	}

	return rw.emitCompleted()
}

func (rw *responsesAPIWriter) emitCompleted() error {
	if !rw.completedOnce.CompareAndSwap(false, true) {
		return nil // already emitted
	}
	return rw.writeSSEEvent("response.completed", map[string]interface{}{
		"type":   "response.completed",
		"status": "completed",
	})
}

func (rw *responsesAPIWriter) writeSSEEvent(eventType string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(rw.w, "event: %s\ndata: %s\n\n", eventType, data)
	if f, ok := rw.w.(http.Flusher); ok {
		f.Flush()
	}
	return err
}

// ── ResponseSink interface compliance ─────────────────────────────────────────
//
// The orchestrator's main streaming path writes directly to OrchestratorInput.W
// (which is this responsesAPIWriter), bypassing ResponseSink. These methods
// exist for interface compliance only.

// WriteChunk implements ResponseSink (interface compliance; not called by orchestrator streaming path).
func (rw *responsesAPIWriter) WriteChunk(_ providers.StreamEvent) error {
	return nil
}

// WriteDone implements ResponseSink (interface compliance; not called by orchestrator streaming path).
// Defensively emits response.completed if the [DONE] sentinel was never seen in Write().
func (rw *responsesAPIWriter) WriteDone(_ *providers.Usage, _ string) error {
	return rw.emitCompleted()
}

// WriteError implements ResponseSink (interface compliance).
func (rw *responsesAPIWriter) WriteError(_ error) {}

// ExtraHeaders implements ResponseSink.
func (rw *responsesAPIWriter) ExtraHeaders() map[string]string {
	return map[string]string{
		"X-Selected-Model": rw.modelName,
	}
}
