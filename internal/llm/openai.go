package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

var _ Client = (*openAIClient)(nil)

// openAIClient は OpenAI API の実装
type openAIClient struct {
	client *openai.Client
}

// NewOpenAIClient は新しい OpenAI クライアントを作成
func NewOpenAIClient(apiKey string) Client {
	c := openai.NewClient(option.WithAPIKey(apiKey))
	return &openAIClient{client: &c}
}

// AnalyzeResults はクエリ結果を分析して要約を生成
func (c *openAIClient) AnalyzeResults(ctx context.Context, model string, results map[string]string, systemPrompt, schemaInfo, documentInfo string) (string, error) {
	if schemaInfo != "" {
		systemPrompt += "\n\n" + schemaInfo
	}
	if documentInfo != "" {
		systemPrompt += "\n\n" + documentInfo
	}

	var sb strings.Builder
	sb.WriteString("クエリ結果:\n")
	for name, result := range results {
		sb.WriteString(fmt.Sprintf("\n【%s】\n%s\n", name, result))
	}

	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:     openai.ChatModel(model),
		MaxTokens: openai.Int(4096),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(sb.String()),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to invoke model: %w", err)
	}

	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("empty response from model")
	}

	return completion.Choices[0].Message.Content, nil
}
