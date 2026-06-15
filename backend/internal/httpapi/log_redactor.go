package httpapi

import (
	"context"
	"log/slog"
	"regexp"
)

// Redaction patterns (basic regex)
var (
	emailPattern      = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	phonePattern      = regexp.MustCompile(`\b\d{3}[-.]?\d{3}[-.]?\d{4}\b`)
	creditCardPattern = regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`)
)

type LogMode string

const (
	LogModeMetadataOnly LogMode = "metadata_only"
	LogModeRedacted     LogMode = "redacted"
	LogModeFull         LogMode = "full"
)

// logWithMode logs based on configured mode
func logWithMode(ctx context.Context, log *slog.Logger, mode LogMode, level slog.Level, msg string, attrs ...slog.Attr) {
	switch mode {
	case LogModeFull:
		// Log everything verbatim
		log.LogAttrs(ctx, level, msg, attrs...)

	case LogModeRedacted:
		// Apply PII redaction to string attributes
		redactedAttrs := make([]slog.Attr, len(attrs))
		for i, attr := range attrs {
			redactedAttrs[i] = redactAttr(attr)
		}
		log.LogAttrs(ctx, level, msg, redactedAttrs...)

	case LogModeMetadataOnly:
		// Only log safe metadata attributes
		safeAttrs := filterSafeAttrs(attrs)
		log.LogAttrs(ctx, level, msg, safeAttrs...)
	}
}

func redactAttr(attr slog.Attr) slog.Attr {
	if attr.Value.Kind() != slog.KindString {
		return attr
	}

	value := attr.Value.String()
	value = emailPattern.ReplaceAllString(value, "[REDACTED-EMAIL]")
	value = phonePattern.ReplaceAllString(value, "[REDACTED-PHONE]")
	value = creditCardPattern.ReplaceAllString(value, "[REDACTED-CC]")

	return slog.String(attr.Key, value)
}

func filterSafeAttrs(attrs []slog.Attr) []slog.Attr {
	safe := make([]slog.Attr, 0, len(attrs))

	safeKeys := map[string]bool{
		"request_id": true,
		"tenant":     true,
		"model":      true,
		"provider":   true,
		"status":     true,
		"latency_ms": true,
		"cost_usd":   true,
		"attempt":    true,
		"tokens":     true,
	}

	for _, attr := range attrs {
		if safeKeys[attr.Key] {
			safe = append(safe, attr)
		}
	}

	return safe
}
