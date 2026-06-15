package loggingcore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

const (
	EventTypeConversation = "conversation"
	EventTypeCompliance   = "compliance"
)

// LogEvent is the generic logging envelope used by handlers.
type LogEvent struct {
	Type      string
	TenantID  string
	RequestID string
	Payload   map[string]interface{}
}

// RequestLogWriter is the minimal persistence contract needed by this service.
type RequestLogWriter interface {
	LogRequest(ctx context.Context, log storage.RequestLog) error
}

// ComplianceEventWriter persists compliance_event_log rows.
type ComplianceEventWriter interface {
	LogComplianceEvent(ctx context.Context, event storage.ComplianceEventLog) error
}

// ConversationWriter persists conversation_log rows.
type ConversationWriter interface {
	LogConversation(ctx context.Context, row storage.ConversationLog) error
}

// Service is the central backend logging service.
// It decides event routing and performs a single write path for request_log.
type Service struct {
	log    *slog.Logger
	writer RequestLogWriter
}

func New(log *slog.Logger, writer RequestLogWriter) *Service {
	return &Service{log: log, writer: writer}
}

// Log is the API entrypoint required by SPEC_92.
func (s *Service) Log(event LogEvent) error {
	return s.LogWithContext(context.Background(), event)
}

// LogWithContext is the context-aware variant used by request handlers.
func (s *Service) LogWithContext(ctx context.Context, event LogEvent) error {
	switch event.Type {
	case EventTypeConversation:
		if _, ok := event.Payload["conversation_log"]; ok {
			w, ok := s.writer.(ConversationWriter)
			if !ok {
				return fmt.Errorf("conversation writer not available")
			}
			row, err := conversationLogFromPayload(event)
			if err != nil {
				return err
			}
			return w.LogConversation(ctx, row)
		}
		rl, err := requestLogFromPayload(event)
		if err != nil {
			return err
		}
		return s.writer.LogRequest(ctx, rl)
	case EventTypeCompliance:
		w, ok := s.writer.(ComplianceEventWriter)
		if !ok {
			return fmt.Errorf("compliance event writer not available")
		}
		ev, err := complianceEventFromPayload(event)
		if err != nil {
			return err
		}
		return w.LogComplianceEvent(ctx, ev)
	default:
		return fmt.Errorf("unsupported log event type: %s", event.Type)
	}
}

func requestLogFromPayload(event LogEvent) (storage.RequestLog, error) {
	var rl storage.RequestLog
	if event.Payload == nil {
		return rl, fmt.Errorf("missing payload")
	}
	raw, ok := event.Payload["request_log"]
	if !ok {
		return rl, fmt.Errorf("missing payload.request_log")
	}
	switch v := raw.(type) {
	case storage.RequestLog:
		rl = v
	case *storage.RequestLog:
		if v == nil {
			return rl, fmt.Errorf("payload.request_log is nil")
		}
		rl = *v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return rl, fmt.Errorf("marshal payload.request_log: %w", err)
		}
		if err := json.Unmarshal(b, &rl); err != nil {
			return rl, fmt.Errorf("unmarshal payload.request_log: %w", err)
		}
	}
	if rl.Timestamp.IsZero() {
		rl.Timestamp = time.Now().UTC()
	}
	if rl.TenantID == "" {
		rl.TenantID = event.TenantID
	}
	if rl.RequestID == "" {
		rl.RequestID = event.RequestID
	}
	return rl, nil
}

func complianceEventFromPayload(event LogEvent) (storage.ComplianceEventLog, error) {
	var ev storage.ComplianceEventLog
	if event.Payload == nil {
		return ev, fmt.Errorf("missing payload")
	}
	raw, ok := event.Payload["compliance_event"]
	if !ok {
		return ev, fmt.Errorf("missing payload.compliance_event")
	}
	switch v := raw.(type) {
	case storage.ComplianceEventLog:
		ev = v
	case *storage.ComplianceEventLog:
		if v == nil {
			return ev, fmt.Errorf("payload.compliance_event is nil")
		}
		ev = *v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ev, fmt.Errorf("marshal payload.compliance_event: %w", err)
		}
		if err := json.Unmarshal(b, &ev); err != nil {
			return ev, fmt.Errorf("unmarshal payload.compliance_event: %w", err)
		}
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
	}
	if ev.TenantID == "" {
		ev.TenantID = event.TenantID
	}
	if ev.RequestID == "" {
		ev.RequestID = event.RequestID
	}
	return ev, nil
}

func conversationLogFromPayload(event LogEvent) (storage.ConversationLog, error) {
	var row storage.ConversationLog
	if event.Payload == nil {
		return row, fmt.Errorf("missing payload")
	}
	raw, ok := event.Payload["conversation_log"]
	if !ok {
		return row, fmt.Errorf("missing payload.conversation_log")
	}
	switch v := raw.(type) {
	case storage.ConversationLog:
		row = v
	case *storage.ConversationLog:
		if v == nil {
			return row, fmt.Errorf("payload.conversation_log is nil")
		}
		row = *v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return row, fmt.Errorf("marshal payload.conversation_log: %w", err)
		}
		if err := json.Unmarshal(b, &row); err != nil {
			return row, fmt.Errorf("unmarshal payload.conversation_log: %w", err)
		}
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
	}
	if row.TenantID == "" {
		row.TenantID = event.TenantID
	}
	if row.RequestID == "" {
		row.RequestID = event.RequestID
	}
	return row, nil
}
