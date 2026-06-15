package providers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/smithy-go"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// Bedrock implements Provider using the AWS Bedrock Runtime Converse API.
type Bedrock struct {
	client            *bedrockruntime.Client
	region            string // resolved region used by the client (for logs)
	staticCredentials bool   // true when both access key id and secret were set in config at construction
}

// NewBedrock builds a Bedrock Runtime client.
//
// Static keys: only if both aws_access_key_id and aws_secret_access_key are non-empty in config.
// Any other case (including only one of the two — common when the secret is not stored in DB and
// credentials come from IAM/IRSA or AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY in the environment)
// uses the AWS SDK default credential chain. Region comes from config, else AWS_REGION / AWS_DEFAULT_REGION.
func NewBedrock(pc config.ProviderConfig) (*Bedrock, error) {
	accessKey := strings.TrimSpace(pc.AwsAccessKeyID)
	secretKey := strings.TrimSpace(pc.AwsSecretAccessKey)
	region := strings.TrimSpace(pc.AwsRegion)
	if region == "" {
		region = strings.TrimSpace(os.Getenv("AWS_REGION"))
	}
	if region == "" {
		region = strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION"))
	}
	if region == "" {
		return nil, fmt.Errorf("aws_bedrock: aws_region is required (or set AWS_REGION / AWS_DEFAULT_REGION)")
	}

	var awsCfg aws.Config
	var err error
	if accessKey != "" && secretKey != "" {
		awsCfg, err = awsconfig.LoadDefaultConfig(context.Background(),
			awsconfig.WithRegion(region),
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		)
	} else {
		awsCfg, err = awsconfig.LoadDefaultConfig(context.Background(),
			awsconfig.WithRegion(region),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("aws_bedrock: aws config: %w", err)
	}
	staticCreds := accessKey != "" && secretKey != ""
	return &Bedrock{client: bedrockruntime.NewFromConfig(awsCfg), region: region, staticCredentials: staticCreds}, nil
}

// SetClient replaces the Bedrock runtime client (tests).
func (b *Bedrock) SetClient(c *bedrockruntime.Client) {
	b.client = c
}

// ChatCompletion calls Bedrock Converse with OpenAI-shaped messages mapped to Bedrock format.
func (b *Bedrock) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if b.client == nil {
		return nil, fmt.Errorf("aws_bedrock: client not initialized")
	}
	modelID := strings.TrimSpace(req.ProviderModelID)
	if modelID == "" {
		modelID = strings.TrimSpace(req.Model)
	}
	if modelID == "" {
		return nil, &UpstreamError{StatusCode: 400, Body: `{"error":"provider_model_id is required for Bedrock models"}`}
	}

	systemBlocks, messages, err := bedrockOpenAIMessagesToConverse(req.Messages)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, &UpstreamError{StatusCode: 400, Body: `{"error":"at least one user or assistant message is required"}`}
	}

	in := &bedrockruntime.ConverseInput{
		ModelId:  aws.String(modelID),
		Messages: messages,
	}
	if len(systemBlocks) > 0 {
		in.System = systemBlocks
	}
	if inf := bedrockInferenceConfig(req); inf != nil {
		in.InferenceConfig = inf
	}

	out, err := b.client.Converse(ctx, in)
	if err != nil {
		slog.WarnContext(ctx, "bedrock Converse failed",
			"error", err,
			"model_id", modelID,
			"region", b.region,
			"static_credentials", b.staticCredentials,
		)
		return nil, mapBedrockError(err)
	}
	return bedrockConverseToChatResponse(out, req.Model, req.Messages)
}

// ChatCompletionStream calls Bedrock ConverseStream and maps events to the gateway's
// StreamEvent format, reusing the shared SSE renderer.
//
// Per SPEC_118: inferenceConfig is intentionally NOT sent for streaming.
func (b *Bedrock) ChatCompletionStream(ctx context.Context, req ChatRequest) (*StreamResponse, error) {
	if b.client == nil {
		return nil, fmt.Errorf("aws_bedrock: client not initialized")
	}
	modelID := strings.TrimSpace(req.ProviderModelID)
	if modelID == "" {
		modelID = strings.TrimSpace(req.Model)
	}
	if modelID == "" {
		return nil, &UpstreamError{StatusCode: 400, Body: `{"error":"provider_model_id is required for Bedrock models"}`}
	}

	systemBlocks, messages, err := bedrockOpenAIMessagesToConverse(req.Messages)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, &UpstreamError{StatusCode: 400, Body: `{"error":"at least one user or assistant message is required"}`}
	}

	in := &bedrockruntime.ConverseStreamInput{
		ModelId:  aws.String(modelID),
		Messages: messages,
	}
	if len(systemBlocks) > 0 {
		in.System = systemBlocks
	}
	// Note: inferenceConfig (temperature, top_p, etc.) is intentionally omitted per SPEC_118.
	// Bedrock streaming works correctly without it, and adding it causes failures on some models.

	out, err := b.client.ConverseStream(ctx, in)
	if err != nil {
		slog.WarnContext(ctx, "bedrock ConverseStream init failed",
			"error", err,
			"model_id", modelID,
			"region", b.region,
			"provider", "bedrock",
		)
		return nil, mapBedrockError(err)
	}

	ch := make(chan StreamEvent, 32)
	go runBedrockStreamLoop(ctx, out.GetStream(), ch, modelID, b.region)
	return &StreamResponse{Events: ch}, nil
}

// runBedrockStreamLoop reads Bedrock stream events and emits StreamEvents on ch.
// It is extracted as a standalone function for testability.
// Closes ch and the stream when done.
func runBedrockStreamLoop(ctx context.Context, stream *bedrockruntime.ConverseStreamEventStream, ch chan<- StreamEvent, modelID, region string) {
	defer close(ch)
	defer stream.Close()

	var capturedUsage *Usage

	for event := range stream.Events() {
		switch v := event.(type) {
		case *brtypes.ConverseStreamOutputMemberContentBlockDelta:
			if v == nil {
				continue
			}
			textDelta, ok := v.Value.Delta.(*brtypes.ContentBlockDeltaMemberText)
			if !ok || textDelta.Value == "" {
				continue
			}
			select {
			case ch <- StreamEvent{Type: "delta", Content: textDelta.Value}:
			case <-ctx.Done():
				return
			}

		case *brtypes.ConverseStreamOutputMemberMetadata:
			if v == nil || v.Value.Usage == nil {
				continue
			}
			u := v.Value.Usage
			captured := &Usage{}
			if u.InputTokens != nil {
				captured.PromptTokens = int(*u.InputTokens)
			}
			if u.OutputTokens != nil {
				captured.CompletionTokens = int(*u.OutputTokens)
			}
			if u.TotalTokens != nil {
				captured.TotalTokens = int(*u.TotalTokens)
			}
			capturedUsage = captured

		case *brtypes.ConverseStreamOutputMemberMessageStop:
			select {
			case ch <- StreamEvent{Type: "done", Usage: capturedUsage}:
			case <-ctx.Done():
			}
			return
		}
	}

	// Events channel exhausted — check for stream-level error.
	if err := stream.Err(); err != nil {
		slog.WarnContext(ctx, "bedrock stream error",
			"error", err,
			"model_id", modelID,
			"region", region,
			"provider", "bedrock",
		)
		select {
		case ch <- StreamEvent{Type: "error", Error: mapBedrockError(err)}:
		default:
		}
		return
	}

	// Stream ended without an explicit messageStop — emit done with any captured usage.
	select {
	case ch <- StreamEvent{Type: "done", Usage: capturedUsage}:
	case <-ctx.Done():
	}
}

func bedrockInferenceConfig(req ChatRequest) *brtypes.InferenceConfiguration {
	var inf brtypes.InferenceConfiguration
	nonEmpty := false
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		v := int32(*req.MaxTokens)
		inf.MaxTokens = aws.Int32(v)
		nonEmpty = true
	}
	if req.Temperature != nil {
		t := float32(*req.Temperature)
		inf.Temperature = aws.Float32(t)
		nonEmpty = true
	}
	if req.TopP != nil {
		p := float32(*req.TopP)
		inf.TopP = aws.Float32(p)
		nonEmpty = true
	}
	if !nonEmpty {
		return nil
	}
	return &inf
}

func bedrockOpenAIMessagesToConverse(msgs []ChatMessage) ([]brtypes.SystemContentBlock, []brtypes.Message, error) {
	var sysParts []string
	var out []brtypes.Message

	for _, msg := range msgs {
		switch msg.Role {
		case "system":
			t := textFromChatMessage(msg)
			if t != "" {
				sysParts = append(sysParts, t)
			}
		case "user", "assistant":
			role := brtypes.ConversationRoleUser
			if msg.Role == "assistant" {
				role = brtypes.ConversationRoleAssistant
			}
			text := textFromChatMessage(msg)
			if text == "" && len(msg.ContentBlocks) == 0 {
				continue
			}
			if text == "" && len(msg.ContentBlocks) > 0 {
				return nil, nil, &UpstreamError{StatusCode: 400, Body: `{"error":"non-text message content is not supported for Bedrock in this gateway version"}`}
			}
			block := &brtypes.ContentBlockMemberText{Value: text}
			out = append(out, brtypes.Message{
				Role:    role,
				Content: []brtypes.ContentBlock{block},
			})
		default:
			// tool / unknown: fold into user text for safety
			t := textFromChatMessage(msg)
			if t == "" {
				continue
			}
			block := &brtypes.ContentBlockMemberText{Value: t}
			out = append(out, brtypes.Message{
				Role:    brtypes.ConversationRoleUser,
				Content: []brtypes.ContentBlock{block},
			})
		}
	}

	var system []brtypes.SystemContentBlock
	if len(sysParts) > 0 {
		system = []brtypes.SystemContentBlock{
			&brtypes.SystemContentBlockMemberText{Value: strings.Join(sysParts, "\n\n")},
		}
	}
	return system, out, nil
}

func textFromChatMessage(msg ChatMessage) string {
	if len(msg.ContentBlocks) == 0 {
		return msg.Content
	}
	var b strings.Builder
	for _, cb := range msg.ContentBlocks {
		if cb.Type == "text" && cb.Text != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(cb.Text)
		}
	}
	if b.Len() > 0 {
		return b.String()
	}
	return msg.Content
}

func bedrockConverseToChatResponse(out *bedrockruntime.ConverseOutput, gatewayModelName string, reqMsgs []ChatMessage) (*ChatResponse, error) {
	if out == nil {
		return nil, &UpstreamError{StatusCode: 502, Body: "empty bedrock response"}
	}
	text, err := extractBedrockAssistantText(out)
	if err != nil {
		return nil, err
	}

	finish := "stop"
	switch out.StopReason {
	case brtypes.StopReasonMaxTokens:
		finish = "length"
	case brtypes.StopReasonContentFiltered:
		finish = "content_filter"
	}

	promptTok, completionTok, totalTok := 0, 0, 0
	if out.Usage != nil {
		if out.Usage.InputTokens != nil {
			promptTok = int(*out.Usage.InputTokens)
		}
		if out.Usage.OutputTokens != nil {
			completionTok = int(*out.Usage.OutputTokens)
		}
		if out.Usage.TotalTokens != nil {
			totalTok = int(*out.Usage.TotalTokens)
		}
	}
	if promptTok == 0 {
		promptTok = estimatePromptTokensFromMessages(reqMsgs)
	}
	if completionTok == 0 {
		completionTok = EstimateTokens(text)
	}
	if totalTok == 0 {
		totalTok = promptTok + completionTok
	}

	id, _ := awsmiddleware.GetRequestIDMetadata(out.ResultMetadata)
	if id == "" {
		id = fmt.Sprintf("bedrock-%d", time.Now().UnixNano())
	}

	return &ChatResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   gatewayModelName,
		Choices: []ChatChoice{
			{Index: 0, Message: ChatMessage{Role: "assistant", Content: text}, FinishReason: finish},
		},
		Usage: Usage{
			PromptTokens:     promptTok,
			CompletionTokens: completionTok,
			TotalTokens:      totalTok,
		},
	}, nil
}

func extractBedrockAssistantText(out *bedrockruntime.ConverseOutput) (string, error) {
	if out.Output == nil {
		return "", &UpstreamError{StatusCode: 502, Body: "bedrock response missing output"}
	}
	v, ok := out.Output.(*brtypes.ConverseOutputMemberMessage)
	if !ok || v == nil {
		return "", fmt.Errorf("bedrock: unexpected converse output type %T", out.Output)
	}
	var b strings.Builder
	for _, block := range v.Value.Content {
		if tb, ok := block.(*brtypes.ContentBlockMemberText); ok {
			b.WriteString(tb.Value)
		}
	}
	return b.String(), nil
}

func estimatePromptTokensFromMessages(msgs []ChatMessage) int {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(textFromChatMessage(m))
	}
	return EstimateTokens(b.String())
}

func mapBedrockError(err error) error {
	if err == nil {
		return nil
	}
	var re *brtypes.ResourceNotFoundException
	if errors.As(err, &re) {
		return &UpstreamError{StatusCode: 404, Body: sanitizeBedrockErrMsg(re.Error())}
	}
	var te *brtypes.ThrottlingException
	if errors.As(err, &te) {
		return &UpstreamError{StatusCode: 429, Body: sanitizeBedrockErrMsg(te.Error())}
	}
	var ve *brtypes.ValidationException
	if errors.As(err, &ve) {
		return &UpstreamError{StatusCode: 400, Body: sanitizeBedrockErrMsg(ve.Error())}
	}
	var ae *brtypes.AccessDeniedException
	if errors.As(err, &ae) {
		return &UpstreamError{StatusCode: 403, Body: sanitizeBedrockErrMsg(ae.Error())}
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		msg := sanitizeBedrockErrMsg(apiErr.ErrorMessage())
		if msg == "" {
			msg = sanitizeBedrockErrMsg(apiErr.Error())
		}
		switch code {
		case "ThrottlingException", "TooManyRequestsException":
			return &UpstreamError{StatusCode: 429, Body: msg}
		case "AccessDeniedException", "UnauthorizedOperation":
			return &UpstreamError{StatusCode: 403, Body: msg}
		case "ValidationException":
			return &UpstreamError{StatusCode: 400, Body: msg}
		case "ResourceNotFoundException", "ModelNotFoundException":
			return &UpstreamError{StatusCode: 404, Body: msg}
		case "InvalidSignatureException", "UnrecognizedClientException", "InvalidClientTokenId":
			return &UpstreamError{StatusCode: 401, Body: msg}
		case "ServiceUnavailableException", "InternalServerException", "ModelTimeoutException":
			return &UpstreamError{StatusCode: 503, Body: msg}
		default:
			return &UpstreamError{StatusCode: 502, Body: msg}
		}
	}
	var opErr *smithy.OperationError
	if errors.As(err, &opErr) {
		return &UpstreamError{StatusCode: 502, Body: sanitizeBedrockErrMsg(opErr.Error())}
	}
	return &UpstreamError{StatusCode: 502, Body: sanitizeBedrockErrMsg(err.Error())}
}

func sanitizeBedrockErrMsg(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 2048 {
		s = s[:2048] + "..."
	}
	return s
}
