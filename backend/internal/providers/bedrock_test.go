package providers

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/smithy-go"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewBedrock_PartialConfigUsesDefaultChain(t *testing.T) {
	// Only access key in config (secret in env/IRSA) — must not fail startup.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Run("only access key id in config", func(t *testing.T) {
		_, err := NewBedrock(config.ProviderConfig{
			Type:           "aws_bedrock",
			AwsAccessKeyID: "AKIAIOSFODNN7EXAMPLE",
			AwsRegion:      "us-east-1",
		})
		require.NoError(t, err)
	})
	t.Run("only secret in config", func(t *testing.T) {
		_, err := NewBedrock(config.ProviderConfig{
			Type:               "aws_bedrock",
			AwsSecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			AwsRegion:          "us-east-1",
		})
		require.NoError(t, err)
	})
}

func TestNewBedrock_MissingRegion(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	_, err := NewBedrock(config.ProviderConfig{
		Type:               "aws_bedrock",
		AwsAccessKeyID:     "AKIA",
		AwsSecretAccessKey: "secret",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "aws_region")
}

func TestNewBedrock_DefaultCredentialChain_NoKeys(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	_, err := NewBedrock(config.ProviderConfig{
		Type:      "aws_bedrock",
		AwsRegion: "us-east-1",
	})
	require.NoError(t, err)
}

func TestNewBedrock_RegionFromEnv(t *testing.T) {
	t.Setenv("AWS_REGION", "eu-west-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	b, err := NewBedrock(config.ProviderConfig{Type: "aws_bedrock"})
	require.NoError(t, err)
	require.NotNil(t, b)
}

func TestBedrock_OpenAIMessagesToConverse(t *testing.T) {
	sys, msgs, err := bedrockOpenAIMessagesToConverse([]ChatMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hola"},
	})
	require.NoError(t, err)
	require.Len(t, sys, 1)
	require.Len(t, msgs, 1)
	_, ok := msgs[0].Content[0].(*brtypes.ContentBlockMemberText)
	require.True(t, ok)
}

func TestBedrock_OpenAIMessagesToConverse_Assistant(t *testing.T) {
	_, msgs, err := bedrockOpenAIMessagesToConverse([]ChatMessage{
		{Role: "user", Content: "Hi"},
		{Role: "assistant", Content: "Hello"},
		{Role: "user", Content: "Bye"},
	})
	require.NoError(t, err)
	require.Len(t, msgs, 3)
}

func TestBedrock_ConverseToChatResponse(t *testing.T) {
	out := &bedrockruntime.ConverseOutput{
		StopReason: brtypes.StopReasonEndTurn,
		Output: &brtypes.ConverseOutputMemberMessage{
			Value: brtypes.Message{
				Role: brtypes.ConversationRoleAssistant,
				Content: []brtypes.ContentBlock{
					&brtypes.ContentBlockMemberText{Value: "Hello world"},
				},
			},
		},
		Usage: &brtypes.TokenUsage{
			InputTokens:  aws.Int32(10),
			OutputTokens: aws.Int32(5),
			TotalTokens:  aws.Int32(15),
		},
	}
	reqMsgs := []ChatMessage{{Role: "user", Content: "Hi"}}
	resp, err := bedrockConverseToChatResponse(out, "my-model", reqMsgs)
	require.NoError(t, err)
	require.Equal(t, "my-model", resp.Model)
	require.Equal(t, "Hello world", resp.Choices[0].Message.Content)
	require.Equal(t, 10, resp.Usage.PromptTokens)
	require.Equal(t, 5, resp.Usage.CompletionTokens)
	require.Equal(t, 15, resp.Usage.TotalTokens)
}

func TestMapBedrockError_Throttling(t *testing.T) {
	err := mapBedrockError(&smithyGenericAPIError{code: "ThrottlingException", msg: "slow"})
	ue, ok := err.(*UpstreamError)
	require.True(t, ok)
	require.Equal(t, 429, ue.StatusCode)
}

func TestMapBedrockError_NonAPIErrorUsesMessage(t *testing.T) {
	err := mapBedrockError(fmt.Errorf("dial tcp: lookup bedrock"))
	ue, ok := err.(*UpstreamError)
	require.True(t, ok)
	require.Equal(t, 502, ue.StatusCode)
	require.Contains(t, ue.Body, "dial tcp")
}

func TestMapBedrockError_WrappedOperationError(t *testing.T) {
	inner := &smithyGenericAPIError{code: "Unknown", msg: "raw failure"}
	wrapped := fmt.Errorf("operation error: %w", inner)
	err := mapBedrockError(wrapped)
	ue, ok := err.(*UpstreamError)
	require.True(t, ok)
	require.Equal(t, 502, ue.StatusCode)
	require.Contains(t, ue.Body, "raw failure")
}

func TestMapBedrockError_EmptyErrorMessageFallback(t *testing.T) {
	err := mapBedrockError(&smithyGenericAPIError{code: "ValidationException", msg: ""})
	ue, ok := err.(*UpstreamError)
	require.True(t, ok)
	require.Equal(t, 400, ue.StatusCode)
	require.NotEmpty(t, ue.Body)
}

func TestMapBedrockError_OperationErrorType(t *testing.T) {
	op := &smithy.OperationError{
		ServiceID:     "Bedrock Runtime",
		OperationName: "Converse",
		Err:           errors.New("underlying transport failure"),
	}
	err := mapBedrockError(op)
	ue, ok := err.(*UpstreamError)
	require.True(t, ok)
	require.Equal(t, 502, ue.StatusCode)
	require.Contains(t, ue.Body, "underlying transport failure")
}

// smithyGenericAPIError implements smithy.APIError for tests.
type smithyGenericAPIError struct {
	code, msg string
}

func (e *smithyGenericAPIError) Error() string                 { return e.code + ": " + e.msg }
func (e *smithyGenericAPIError) ErrorCode() string             { return e.code }
func (e *smithyGenericAPIError) ErrorMessage() string          { return e.msg }
func (e *smithyGenericAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultServer }
