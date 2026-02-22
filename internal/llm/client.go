package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Client は Anthropic API クライアント
type Client struct {
	client *anthropic.Client
}

// NewClient は新しい Anthropic クライアントを作成
func NewClient(apiKey string) *Client {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	return &Client{
		client: &client,
	}
}

// Invoke は LLM にプロンプトを送信して応答を取得
func (c *Client) Invoke(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	message, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-haiku-20240307"),
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
func (c *Client) AnalyzeResults(ctx context.Context, results map[string]string, schemaInfo string) (string, error) {
	systemPrompt := `あなたはエラー調査アシスタントです。
クエリ結果をもとに、エラーや異常の有無を調査し、簡潔に報告してください。

回答の構成：
1. エラー・異常の有無（まず最初に明示する）
2. エラーがある場合：
   - エラーメッセージの内容
   - 問題のあるレコードやカラム（IDや識別子を示す）
   - NULL・欠損・不整合など異常な値の指摘
3. エラーがない場合：正常である旨を一言で

回答のガイドライン：
- 簡潔でわかりやすい日本語で回答
- 個別レコードの詳細な値の羅列は不要
- 複数のデータソースがある場合は横断的に分析する`

	if schemaInfo != "" {
		systemPrompt += "\n\n" + schemaInfo
	}

	var sb strings.Builder
	sb.WriteString("クエリ結果:\n")
	for name, result := range results {
		sb.WriteString(fmt.Sprintf("\n【%s】\n%s\n", name, result))
	}

	return c.Invoke(ctx, systemPrompt, sb.String())
}
