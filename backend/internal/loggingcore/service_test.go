package loggingcore

import (
	"context"
	"log/slog"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

type testWriter struct {
	last             storage.RequestLog
	lastConversation storage.ConversationLog
	lastCompliance   storage.ComplianceEventLog
	n                int
	conversationWrites int
	complianceWrites int
}

func (w *testWriter) LogRequest(_ context.Context, rl storage.RequestLog) error {
	w.last = rl
	w.n++
	return nil
}

func (w *testWriter) LogComplianceEvent(_ context.Context, ev storage.ComplianceEventLog) error {
	w.lastCompliance = ev
	w.complianceWrites++
	return nil
}

func (w *testWriter) LogConversation(_ context.Context, row storage.ConversationLog) error {
	w.lastConversation = row
	w.conversationWrites++
	return nil
}

func TestService_LogWithContext_ConversationWritesRequestLog(t *testing.T) {
	w := &testWriter{}
	s := New(slog.Default(), w)
	rl := storage.RequestLog{TenantID: "t1", RequestID: "req-1", Status: "ok"}
	err := s.LogWithContext(context.Background(), LogEvent{
		Type:      EventTypeConversation,
		TenantID:  "t1",
		RequestID: "req-1",
		Payload: map[string]interface{}{
			"request_log": rl,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.n != 1 {
		t.Fatalf("expected 1 write, got %d", w.n)
	}
	if w.last.TenantID != "t1" || w.last.RequestID != "req-1" {
		t.Fatalf("unexpected request_log written: %+v", w.last)
	}
}

func TestService_LogWithContext_ConversationWritesConversationLog(t *testing.T) {
	w := &testWriter{}
	s := New(slog.Default(), w)
	err := s.LogWithContext(context.Background(), LogEvent{
		Type:      EventTypeConversation,
		TenantID:  "t1",
		RequestID: "req-1",
		Payload: map[string]interface{}{
			"conversation_log": storage.ConversationLog{
				TenantID:      "t1",
				RequestID:     "req-1",
				PromptPreview: "hello",
				ResponsePreview: "world",
				LoggingMode:   "metadata_only",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.conversationWrites != 1 {
		t.Fatalf("expected 1 conversation write, got %d", w.conversationWrites)
	}
}

func TestService_LogWithContext_UnsupportedTypeFails(t *testing.T) {
	w := &testWriter{}
	s := New(slog.Default(), w)
	err := s.LogWithContext(context.Background(), LogEvent{
		Type:      "other",
		TenantID:  "t1",
		RequestID: "req-1",
		Payload:   map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
	if w.n != 0 {
		t.Fatalf("expected no writes, got %d", w.n)
	}
}

func TestService_LogWithContext_ComplianceWritesEvent(t *testing.T) {
	w := &testWriter{}
	s := New(slog.Default(), w)
	ev := storage.ComplianceEventLog{
		TenantID:    "t1",
		RequestID:   "req-1",
		EventType:   "pii_blocked",
		ActionTaken: "blocked",
	}
	err := s.LogWithContext(context.Background(), LogEvent{
		Type:      EventTypeCompliance,
		TenantID:  "t1",
		RequestID: "req-1",
		Payload: map[string]interface{}{
			"compliance_event": ev,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.complianceWrites != 1 {
		t.Fatalf("expected 1 compliance write, got %d", w.complianceWrites)
	}
	if w.lastCompliance.EventType != "pii_blocked" || w.lastCompliance.ActionTaken != "blocked" {
		t.Fatalf("unexpected compliance event written: %+v", w.lastCompliance)
	}
}
