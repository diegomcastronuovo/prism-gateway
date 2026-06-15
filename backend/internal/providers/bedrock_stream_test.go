package providers

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/require"
)

// mockConverseStreamReader implements bedrockruntime.ConverseStreamOutputReader
// for unit-testing runBedrockStreamLoop without real AWS calls.
type mockConverseStreamReader struct {
	events []brtypes.ConverseStreamOutput
	err    error
}

func (m *mockConverseStreamReader) Events() <-chan brtypes.ConverseStreamOutput {
	ch := make(chan brtypes.ConverseStreamOutput, len(m.events))
	for _, e := range m.events {
		ch <- e
	}
	close(ch)
	return ch
}

func (m *mockConverseStreamReader) Close() error { return nil }
func (m *mockConverseStreamReader) Err() error   { return m.err }

// makeTestStream creates a ConverseStreamEventStream backed by a mock reader.
func makeTestStream(events []brtypes.ConverseStreamOutput, streamErr error) *bedrockruntime.ConverseStreamEventStream {
	s := bedrockruntime.NewConverseStreamEventStream()
	s.Reader = &mockConverseStreamReader{events: events, err: streamErr}
	return s
}

// drainStream collects all StreamEvents produced by runBedrockStreamLoop.
func drainStream(ctx context.Context, stream *bedrockruntime.ConverseStreamEventStream) []StreamEvent {
	ch := make(chan StreamEvent, 64)
	runBedrockStreamLoop(ctx, stream, ch, "test-model", "us-east-1")
	var out []StreamEvent
	for e := range ch {
		out = append(out, e)
	}
	return out
}

// --- Tests ---

// TestBedrockStream_DeltaEvent verifies that a contentBlockDelta with text maps to a delta StreamEvent.
func TestBedrockStream_DeltaEvent(t *testing.T) {
	events := []brtypes.ConverseStreamOutput{
		&brtypes.ConverseStreamOutputMemberContentBlockDelta{
			Value: brtypes.ContentBlockDeltaEvent{
				ContentBlockIndex: aws.Int32(0),
				Delta:             &brtypes.ContentBlockDeltaMemberText{Value: "Hello"},
			},
		},
		&brtypes.ConverseStreamOutputMemberMessageStop{},
	}

	got := drainStream(context.Background(), makeTestStream(events, nil))
	require.Len(t, got, 2)
	require.Equal(t, "delta", got[0].Type)
	require.Equal(t, "Hello", got[0].Content)
	require.Equal(t, "done", got[1].Type)
}

// TestBedrockStream_MessageStopEmitsDone verifies that messageStop maps to done.
func TestBedrockStream_MessageStopEmitsDone(t *testing.T) {
	events := []brtypes.ConverseStreamOutput{
		&brtypes.ConverseStreamOutputMemberMessageStop{},
	}

	got := drainStream(context.Background(), makeTestStream(events, nil))
	require.Len(t, got, 1)
	require.Equal(t, "done", got[0].Type)
}

// TestBedrockStream_MetadataUsageCaptured verifies that metadata usage is attached to the done event.
func TestBedrockStream_MetadataUsageCaptured(t *testing.T) {
	events := []brtypes.ConverseStreamOutput{
		&brtypes.ConverseStreamOutputMemberContentBlockDelta{
			Value: brtypes.ContentBlockDeltaEvent{
				ContentBlockIndex: aws.Int32(0),
				Delta:             &brtypes.ContentBlockDeltaMemberText{Value: "La capital"},
			},
		},
		&brtypes.ConverseStreamOutputMemberMetadata{
			Value: brtypes.ConverseStreamMetadataEvent{
				Usage: &brtypes.TokenUsage{
					InputTokens:  aws.Int32(10),
					OutputTokens: aws.Int32(5),
					TotalTokens:  aws.Int32(15),
				},
				Metrics: &brtypes.ConverseStreamMetrics{},
			},
		},
		&brtypes.ConverseStreamOutputMemberMessageStop{},
	}

	got := drainStream(context.Background(), makeTestStream(events, nil))
	require.Len(t, got, 2) // delta + done

	doneEvent := got[len(got)-1]
	require.Equal(t, "done", doneEvent.Type)
	require.NotNil(t, doneEvent.Usage)
	require.Equal(t, 10, doneEvent.Usage.PromptTokens)
	require.Equal(t, 5, doneEvent.Usage.CompletionTokens)
	require.Equal(t, 15, doneEvent.Usage.TotalTokens)
}

// TestBedrockStream_NonTextEventsSkipped verifies that non-text delta events and unknown
// events do not produce garbage chunks.
func TestBedrockStream_NonTextEventsSkipped(t *testing.T) {
	events := []brtypes.ConverseStreamOutput{
		// Empty text delta — should be skipped.
		&brtypes.ConverseStreamOutputMemberContentBlockDelta{
			Value: brtypes.ContentBlockDeltaEvent{
				ContentBlockIndex: aws.Int32(0),
				Delta:             &brtypes.ContentBlockDeltaMemberText{Value: ""},
			},
		},
		// MessageStart — unknown to our mapper, should be ignored.
		&brtypes.ConverseStreamOutputMemberMessageStart{},
		// ContentBlockStart — unknown, should be ignored.
		&brtypes.ConverseStreamOutputMemberContentBlockStart{},
		// Real delta followed by stop.
		&brtypes.ConverseStreamOutputMemberContentBlockDelta{
			Value: brtypes.ContentBlockDeltaEvent{
				ContentBlockIndex: aws.Int32(0),
				Delta:             &brtypes.ContentBlockDeltaMemberText{Value: "Hi"},
			},
		},
		&brtypes.ConverseStreamOutputMemberMessageStop{},
	}

	got := drainStream(context.Background(), makeTestStream(events, nil))
	// Only the non-empty delta and done should be emitted.
	require.Len(t, got, 2)
	require.Equal(t, "delta", got[0].Type)
	require.Equal(t, "Hi", got[0].Content)
	require.Equal(t, "done", got[1].Type)
}

// TestBedrockStream_StreamErrorSurfaced verifies that a stream-level error emits an error StreamEvent.
func TestBedrockStream_StreamErrorSurfaced(t *testing.T) {
	streamErr := errors.New("connection reset by peer")
	// No events, just an error after the channel closes.
	got := drainStream(context.Background(), makeTestStream(nil, streamErr))
	require.Len(t, got, 1)
	require.Equal(t, "error", got[0].Type)
	require.Error(t, got[0].Error)
}

// TestBedrockStream_MultipleDeltasOrdered verifies that multiple deltas arrive in order.
func TestBedrockStream_MultipleDeltasOrdered(t *testing.T) {
	words := []string{"La ", "capital ", "de ", "España ", "es ", "Madrid."}
	events := make([]brtypes.ConverseStreamOutput, 0, len(words)+1)
	for _, w := range words {
		events = append(events, &brtypes.ConverseStreamOutputMemberContentBlockDelta{
			Value: brtypes.ContentBlockDeltaEvent{
				ContentBlockIndex: aws.Int32(0),
				Delta:             &brtypes.ContentBlockDeltaMemberText{Value: w},
			},
		})
	}
	events = append(events, &brtypes.ConverseStreamOutputMemberMessageStop{})

	got := drainStream(context.Background(), makeTestStream(events, nil))
	require.Len(t, got, len(words)+1)
	for i, w := range words {
		require.Equal(t, "delta", got[i].Type)
		require.Equal(t, w, got[i].Content)
	}
	require.Equal(t, "done", got[len(got)-1].Type)
}

// TestBedrockStream_NoUsageWhenMetadataAbsent verifies that done event has nil Usage
// when no metadata event was received.
func TestBedrockStream_NoUsageWhenMetadataAbsent(t *testing.T) {
	events := []brtypes.ConverseStreamOutput{
		&brtypes.ConverseStreamOutputMemberContentBlockDelta{
			Value: brtypes.ContentBlockDeltaEvent{
				ContentBlockIndex: aws.Int32(0),
				Delta:             &brtypes.ContentBlockDeltaMemberText{Value: "hello"},
			},
		},
		&brtypes.ConverseStreamOutputMemberMessageStop{},
	}

	got := drainStream(context.Background(), makeTestStream(events, nil))
	doneEvent := got[len(got)-1]
	require.Equal(t, "done", doneEvent.Type)
	require.Nil(t, doneEvent.Usage, "usage should be nil when no metadata event was received")
}
