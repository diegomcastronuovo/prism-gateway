package httpapi

import "github.com/diegomcastronuovo/prism-gateway/internal/providers"

// ResponseSink is implemented by each endpoint handler to receive streaming
// chunks from the Orchestration Core.
//
// The core calls these methods in order; the handler decides how to write them
// to the client. This decouples the core from the wire protocol (SSE, WebSocket, etc.).
type ResponseSink interface {
	// WriteChunk is called for each StreamEvent of type "delta".
	// The handler converts it to the appropriate wire format.
	WriteChunk(event providers.StreamEvent) error

	// WriteDone is called when the provider has finished streaming.
	// Receives the final Usage if the provider reported it.
	WriteDone(usage *providers.Usage, finishReason string) error

	// WriteError is called if a non-EOF error occurs during streaming.
	// The handler may write an SSE error event or close the connection.
	// WriteError MUST NOT call WriteDone.
	WriteError(err error)

	// ExtraHeaders returns headers that the handler wants added to the response
	// BEFORE the core starts writing chunks (e.g. X-Selected-Model).
	ExtraHeaders() map[string]string
}
