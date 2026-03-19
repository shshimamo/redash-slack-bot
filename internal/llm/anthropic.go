package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

var _ Client = (*anthropicClient)(nil)

// anthropicClient は Anthropic API / AWS Bedrock 共通の実装
type anthropicClient struct {
	client *anthropic.Client
}

// NewAnthropicClient は新しい Anthropic クライアントを作成
func NewAnthropicClient(apiKey string) Client {
	c := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &anthropicClient{client: &c}
}

// invoke は LLM にプロンプトを送信して応答を取得
func (c *anthropicClient) invoke(ctx context.Context, model, systemPrompt, userMessage string) (string, error) {
	message, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userMessage)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to invoke model: %w", err)
	}

	if len(message.Content) == 0 {
		return "", fmt.Errorf("empty response from model")
	}

	for _, block := range message.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("no text content in response")
}

// AnalyzeResults はクエリ結果を分析して要約を生成
func (c *anthropicClient) AnalyzeResults(ctx context.Context, model string, results map[string]string, systemPrompt, schemaInfo, documentInfo string) (string, error) {
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

	return c.invoke(ctx, model, systemPrompt, sb.String())
}
