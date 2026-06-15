package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// SSEResponseSink implements ResponseSink by writing OpenAI-compatible
// Server-Sent Events to an http.ResponseWriter.
//
// The streaming event loop lives in writeSSEStream (streaming_handler.go),
// which this sink delegates to via DriveStream. The ResponseSink interface
// methods (WriteChunk, WriteDone, WriteError) exist to satisfy the interface
// but are not used by the SSE path — DriveStream owns the event loop.
type SSEResponseSink struct {
	w            http.ResponseWriter
	extraHeaders map[string]string
}

// newSSEResponseSink creates a new SSEResponseSink. SSE-specific headers are
// NOT set here — they are written by DriveStream/writeSSEStream before any data.
func newSSEResponseSink(w http.ResponseWriter, modelName string) *SSEResponseSink {
	return &SSEResponseSink{
		w: w,
		extraHeaders: map[string]string{
			"X-Selected-Model": modelName,
		},
	}
}

// ExtraHeaders returns headers to be copied to http.ResponseWriter before any
// data is written (e.g. X-Selected-Model for streaming responses).
func (s *SSEResponseSink) ExtraHeaders() map[string]string {
	return s.extraHeaders
}

// WriteChunk implements ResponseSink. Writes a single SSE data event.
// For the SSE path this is provided for interface compliance; in practice the
// event loop in writeSSEStream drives writing directly to avoid extra allocations.
func (s *SSEResponseSink) WriteChunk(event providers.StreamEvent) error {
	payload, done := streamEventToSSEPayload(event)
	if done || len(payload) == 0 {
		return nil
	}
	_, err := fmt.Fprintf(s.w, "data: %s\n\n", payload)
	if f, ok := s.w.(http.Flusher); ok {
		f.Flush()
	}
	return err
}

// WriteDone implements ResponseSink. Writes the SSE [DONE] terminator.
func (s *SSEResponseSink) WriteDone(_ *providers.Usage, _ string) error {
	_, err := fmt.Fprintf(s.w, "data: [DONE]\n\n")
	if f, ok := s.w.(http.Flusher); ok {
		f.Flush()
	}
	return err
}

// WriteError implements ResponseSink. For SSE, by the time an error is known
// the HTTP headers are already sent, so this is a no-op on the wire.
func (s *SSEResponseSink) WriteError(_ error) {}

// Writer returns the underlying http.ResponseWriter. Used by writeSSEStream to
// write directly when invoked via the Orchestrator path.
func (s *SSEResponseSink) Writer() http.ResponseWriter {
	return s.w
}

// driveStream is the SSE event loop. It reads events from streamResp and writes
// SSE-formatted chunks to the sink. It blocks until the stream completes, the
// client disconnects, or the upstream closes.
//
// All logging callbacks are passed explicitly so this function has no hidden
// dependencies on Handlers or Orchestrator.
func driveStream(
	ctx context.Context,
	sink *SSEResponseSink,
	streamResp *providers.StreamResponse,
	onDone func(usage *providers.Usage, output string),
	onError func(err error),
) {
	for {
		select {
		case <-ctx.Done():
			onError(ctx.Err())
			return
		case event, ok := <-streamResp.Events:
			if !ok {
				// Channel closed without [DONE] — treat as completed.
				_ = sink.WriteDone(nil, "")
				onDone(nil, "")
				return
			}
			if event.Type == "error" && event.Error != nil {
				onError(event.Error)
				return
			}
			_, done := streamEventToSSEPayload(event)
			if done {
				_ = sink.WriteDone(event.Usage, "")
				onDone(event.Usage, "")
				return
			}
			_ = sink.WriteChunk(event)
		}
	}
}
