package httpapi

import (
	"context"
	"log/slog"
	"testing"
)

func TestRedactAttr_Email(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple email",
			input:    "user@example.com",
			expected: "[REDACTED-EMAIL]",
		},
		{
			name:     "email in sentence",
			input:    "Contact john.doe@company.org for details",
			expected: "Contact [REDACTED-EMAIL] for details",
		},
		{
			name:     "multiple emails",
			input:    "user1@test.com and user2@test.com",
			expected: "[REDACTED-EMAIL] and [REDACTED-EMAIL]",
		},
		{
			name:     "no email",
			input:    "no email here",
			expected: "no email here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := slog.String("test", tt.input)
			result := redactAttr(attr)
			if result.Value.String() != tt.expected {
				t.Errorf("redactAttr() = %q, want %q", result.Value.String(), tt.expected)
			}
		})
	}
}

func TestRedactAttr_Phone(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "phone with dashes",
			input:    "555-123-4567",
			expected: "[REDACTED-PHONE]",
		},
		{
			name:     "phone with dots",
			input:    "555.123.4567",
			expected: "[REDACTED-PHONE]",
		},
		{
			name:     "phone no separator",
			input:    "5551234567",
			expected: "[REDACTED-PHONE]",
		},
		{
			name:     "phone in sentence",
			input:    "Call me at 555-123-4567 tomorrow",
			expected: "Call me at [REDACTED-PHONE] tomorrow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := slog.String("test", tt.input)
			result := redactAttr(attr)
			if result.Value.String() != tt.expected {
				t.Errorf("redactAttr() = %q, want %q", result.Value.String(), tt.expected)
			}
		})
	}
}

func TestRedactAttr_CreditCard(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "credit card with spaces",
			input:    "4111 1111 1111 1111",
			expected: "[REDACTED-CC]",
		},
		{
			name:     "credit card with dashes",
			input:    "4111-1111-1111-1111",
			expected: "[REDACTED-CC]",
		},
		{
			name:     "credit card no separator",
			input:    "4111111111111111",
			expected: "[REDACTED-CC]",
		},
		{
			name:     "credit card in sentence",
			input:    "My card is 4111-1111-1111-1111 please charge it",
			expected: "My card is [REDACTED-CC] please charge it",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := slog.String("test", tt.input)
			result := redactAttr(attr)
			if result.Value.String() != tt.expected {
				t.Errorf("redactAttr() = %q, want %q", result.Value.String(), tt.expected)
			}
		})
	}
}

func TestRedactAttr_NonString(t *testing.T) {
	// Non-string attributes should pass through unchanged
	attr := slog.Int("count", 42)
	result := redactAttr(attr)
	if result.Value.Kind() != slog.KindInt64 {
		t.Errorf("redactAttr() changed non-string attribute kind")
	}
	if result.Value.Int64() != 42 {
		t.Errorf("redactAttr() modified non-string attribute value")
	}
}

func TestFilterSafeAttrs(t *testing.T) {
	attrs := []slog.Attr{
		slog.String("request_id", "123"),
		slog.String("tenant", "tenant_a"),
		slog.String("model", "gpt-4"),
		slog.String("provider", "openai"),
		slog.String("error", "should be filtered"),
		slog.String("reason", "should be filtered"),
		slog.Int("latency_ms", 100),
		slog.Float64("cost_usd", 0.01),
	}

	safe := filterSafeAttrs(attrs)

	// Expected: 6 safe attributes (request_id, tenant, model, provider, latency_ms, cost_usd)
	if len(safe) != 6 {
		t.Errorf("filterSafeAttrs() returned %d attributes, want 6", len(safe))
	}

	// Check that "error" and "reason" were filtered out
	for _, attr := range safe {
		if attr.Key == "error" || attr.Key == "reason" {
			t.Errorf("filterSafeAttrs() did not filter out %q", attr.Key)
		}
	}
}

func TestLogWithMode_MetadataOnly(t *testing.T) {
	// Create a test logger that captures output
	// In real tests, you might use a buffer or mock logger
	// For now, just verify it doesn't panic
	log := slog.Default()
	ctx := context.Background()

	attrs := []slog.Attr{
		slog.String("request_id", "123"),
		slog.String("error", "this should be filtered"),
	}

	// Should not panic
	logWithMode(ctx, log, LogModeMetadataOnly, slog.LevelInfo, "test", attrs...)
}

func TestLogWithMode_Redacted(t *testing.T) {
	log := slog.Default()
	ctx := context.Background()

	attrs := []slog.Attr{
		slog.String("message", "Contact user@example.com for details"),
	}

	// Should not panic
	logWithMode(ctx, log, LogModeRedacted, slog.LevelInfo, "test", attrs...)
}

func TestLogWithMode_Full(t *testing.T) {
	log := slog.Default()
	ctx := context.Background()

	attrs := []slog.Attr{
		slog.String("sensitive", "user@example.com"),
	}

	// Should not panic
	logWithMode(ctx, log, LogModeFull, slog.LevelInfo, "test", attrs...)
}
