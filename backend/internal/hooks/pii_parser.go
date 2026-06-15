package hooks

import (
	"encoding/json"
	"fmt"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// NormalizedKind is the canonical action type after parsing either Arkana Shield
// or the legacy webhook response format.
type NormalizedKind int

const (
	NormalizedAllow   NormalizedKind = iota
	NormalizedReject                 // Block the request with an HTTP error
	NormalizedModify                 // Replace messages (pre-request) or choices (post-response)
	NormalizedAllowPII               // Reroute to the configured PII-safe model
)

func (k NormalizedKind) String() string {
	switch k {
	case NormalizedAllow:
		return "allow"
	case NormalizedReject:
		return "reject"
	case NormalizedModify:
		return "modify"
	case NormalizedAllowPII:
		return "allow_pii"
	default:
		return "unknown"
	}
}

// NormalizedPIIAction is the unified internal representation produced by
// ParsePIIWebhookResponse, regardless of whether the response came from
// Arkana Shield or the legacy webhook format.
type NormalizedPIIAction struct {
	Kind             NormalizedKind
	RejectStatusCode int                     // HTTP status for Reject (0 → caller default)
	RejectBody       string                  // Human-readable reject message
	Reason           string                  // Informational reason from webhook
	ModifiedMessages []providers.ChatMessage // Pre-request Modify: replacement messages
	ModifiedChoices  []providers.ChatChoice  // Post-response Modify: replacement choices
}

// ParsePIIWebhookResponse auto-detects the webhook response format and returns
// a NormalizedPIIAction. It supports two formats:
//
//	Legacy:  action object contains an "allow" or "reject" key.
//	Arkana:  everything else, including empty action {} = no-op Allow.
//
// On any parse or validation error the caller must apply fail_open / fail_closed
// policy; errors are never swallowed here.
func ParsePIIWebhookResponse(raw []byte) (NormalizedPIIAction, error) {
	var wrapper struct {
		Action json.RawMessage `json:"action"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return NormalizedPIIAction{}, fmt.Errorf("unmarshal webhook response: %w", err)
	}
	if wrapper.Action == nil {
		return NormalizedPIIAction{}, fmt.Errorf("webhook response missing 'action' field")
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(wrapper.Action, &fields); err != nil {
		return NormalizedPIIAction{}, fmt.Errorf("unmarshal action object: %w", err)
	}

	// Presence of "allow" or "reject" keys identifies the legacy format.
	_, hasAllow := fields["allow"]
	_, hasReject := fields["reject"]
	if hasAllow || hasReject {
		return parseLegacyAction(fields)
	}
	return parseArkanaAction(fields)
}

// parseLegacyAction handles the pre-existing webhook format where the action
// is indicated by one of the named fields: allow, reject, body, allow_pii.
func parseLegacyAction(fields map[string]json.RawMessage) (NormalizedPIIAction, error) {
	count := 0
	for _, k := range []string{"allow", "reject", "body", "allow_pii"} {
		if _, ok := fields[k]; ok {
			count++
		}
	}
	if count == 0 {
		return NormalizedPIIAction{}, fmt.Errorf("legacy webhook response: no action field")
	}
	if count > 1 {
		return NormalizedPIIAction{}, fmt.Errorf("legacy webhook response: multiple actions present")
	}

	reason := extractReason(fields)

	if _, ok := fields["allow"]; ok {
		return NormalizedPIIAction{Kind: NormalizedAllow, Reason: reason}, nil
	}

	if _, ok := fields["allow_pii"]; ok {
		return NormalizedPIIAction{Kind: NormalizedAllowPII, Reason: reason}, nil
	}

	if raw, ok := fields["reject"]; ok {
		var reject WebhookReject
		if err := json.Unmarshal(raw, &reject); err != nil {
			return NormalizedPIIAction{}, fmt.Errorf("parse legacy reject: %w", err)
		}
		if reject.Response.Body == "" {
			return NormalizedPIIAction{}, fmt.Errorf("legacy reject requires non-empty response.body")
		}
		return NormalizedPIIAction{
			Kind:             NormalizedReject,
			RejectBody:       reject.Response.Body,
			RejectStatusCode: 400,
			Reason:           reason,
		}, nil
	}

	if raw, ok := fields["body"]; ok {
		var body WebhookBody
		if err := json.Unmarshal(raw, &body); err != nil {
			return NormalizedPIIAction{}, fmt.Errorf("parse legacy body: %w", err)
		}
		if len(body.Messages) == 0 && len(body.Choices) == 0 {
			return NormalizedPIIAction{}, fmt.Errorf("legacy body action requires messages or choices")
		}
		if reason == "" {
			reason = "content modified by webhook"
		}
		return NormalizedPIIAction{
			Kind:             NormalizedModify,
			ModifiedMessages: body.Messages,
			ModifiedChoices:  body.Choices,
			Reason:           reason,
		}, nil
	}

	return NormalizedPIIAction{}, fmt.Errorf("unknown legacy action")
}

// parseArkanaAction handles the Arkana Shield webhook response format where the
// action semantics are derived from the shape of the action object itself:
//
//	{}                                          → Allow (no-op)
//	{"allow_pii":{}}                            → AllowPII
//	{"status_code":N, "body":"...", "reason":""}→ Reject
//	{"body":"string"}                           → Reject (body string, default 403)
//	{"body":{"messages":[...]}, "reason":"..."}  → Modify
func parseArkanaAction(fields map[string]json.RawMessage) (NormalizedPIIAction, error) {
	reason := extractReason(fields)

	// Count non-reason fields to determine whether any action is present.
	actionFieldCount := 0
	for k := range fields {
		if k != "reason" {
			actionFieldCount++
		}
	}

	// Empty action (or reason-only) = Arkana no-op Allow.
	if actionFieldCount == 0 {
		return NormalizedPIIAction{Kind: NormalizedAllow, Reason: reason}, nil
	}

	// allow_pii: reroute to PII-safe model; must not coexist with other action fields.
	if _, ok := fields["allow_pii"]; ok {
		if _, hasBody := fields["body"]; hasBody {
			return NormalizedPIIAction{}, fmt.Errorf("Arkana: allow_pii cannot coexist with body")
		}
		if _, hasStatus := fields["status_code"]; hasStatus {
			return NormalizedPIIAction{}, fmt.Errorf("Arkana: allow_pii cannot coexist with status_code")
		}
		return NormalizedPIIAction{Kind: NormalizedAllowPII, Reason: reason}, nil
	}

	// Reject with an explicit status_code field.
	if statusRaw, hasStatus := fields["status_code"]; hasStatus {
		statusCode := 403
		_ = json.Unmarshal(statusRaw, &statusCode)
		rejectBody := ""
		if bodyRaw, ok := fields["body"]; ok {
			_ = json.Unmarshal(bodyRaw, &rejectBody)
		}
		return NormalizedPIIAction{
			Kind:             NormalizedReject,
			RejectStatusCode: statusCode,
			RejectBody:       rejectBody,
			Reason:           reason,
		}, nil
	}

	// body field: type determines the action (string → Reject, object → Modify).
	if bodyRaw, ok := fields["body"]; ok {
		switch firstNonWhitespace(bodyRaw) {
		case '"':
			var rejectBody string
			if err := json.Unmarshal(bodyRaw, &rejectBody); err != nil {
				return NormalizedPIIAction{}, fmt.Errorf("Arkana reject body: %w", err)
			}
			return NormalizedPIIAction{
				Kind:             NormalizedReject,
				RejectStatusCode: 403,
				RejectBody:       rejectBody,
				Reason:           reason,
			}, nil

		case '{':
			var body struct {
				Messages []providers.ChatMessage `json:"messages,omitempty"`
				Choices  []providers.ChatChoice  `json:"choices,omitempty"`
			}
			if err := json.Unmarshal(bodyRaw, &body); err != nil {
				return NormalizedPIIAction{}, fmt.Errorf("Arkana modify body: %w", err)
			}
			if len(body.Messages) == 0 && len(body.Choices) == 0 {
				return NormalizedPIIAction{}, fmt.Errorf("Arkana modify body requires messages or choices")
			}
			if reason == "" {
				reason = "content modified by webhook"
			}
			return NormalizedPIIAction{
				Kind:             NormalizedModify,
				ModifiedMessages: body.Messages,
				ModifiedChoices:  body.Choices,
				Reason:           reason,
			}, nil

		default:
			return NormalizedPIIAction{}, fmt.Errorf("Arkana body has unexpected JSON type")
		}
	}

	return NormalizedPIIAction{}, fmt.Errorf("unknown Arkana action")
}

// extractReason pulls the optional "reason" string from a parsed action field map.
func extractReason(fields map[string]json.RawMessage) string {
	if r, ok := fields["reason"]; ok {
		var reason string
		_ = json.Unmarshal(r, &reason)
		return reason
	}
	return ""
}

// firstNonWhitespace returns the first non-whitespace byte of b, or 0 if empty.
func firstNonWhitespace(b []byte) byte {
	for _, c := range b {
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return c
		}
	}
	return 0
}
