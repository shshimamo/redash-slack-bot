package llm

import (
	"context"
	"encoding/json"
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

	// TextBlock から Text を取得
	for _, block := range message.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("no text content in response")
}

// SelectionResult は調査選択の結果
type SelectionResult struct {
	InvestigationName string                 `json:"investigation_name"`
	Parameters        map[string]interface{} `json:"parameters"`
	Reasoning         string                 `json:"reasoning"`
	CanHandle         bool                   `json:"can_handle"`
	Message           string                 `json:"message"`
}

// SelectQuery はユーザーのメッセージから実行すべき調査を判定
func (c *Client) SelectQuery(ctx context.Context, userMessage, queriesInfo, schemaInfo string) (*SelectionResult, error) {
	systemPrompt := `あなたは Redash クエリ選択アシスタントです。
ユーザーのリクエストを分析し、適切な調査を選択してください。

以下の情報を参考にしてください：

` + queriesInfo

	if schemaInfo != "" {
		systemPrompt += `

` + schemaInfo
	}

	systemPrompt += `

必ず以下のJSON形式で回答してください：

調査を選択する場合：
{
  "can_handle": true,
  "investigation_name": "調査の名前",
  "parameters": {"パラメータ名": 値},
  "reasoning": "選択理由の説明"
}

対応できない場合：
{
  "can_handle": false,
  "message": "ユーザーへのメッセージ"
}

注意：
- パラメータの値はユーザーのメッセージから抽出してください
- 適切な調査が見つからない場合は can_handle を false にしてください
- JSONのみを出力してください`

	response, err := c.Invoke(ctx, systemPrompt, userMessage)
	if err != nil {
		return nil, err
	}

	// JSONを抽出（余分なテキストがある場合に対応）
	response = extractJSON(response)

	var result SelectionResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("failed to parse selection result: %w, response: %s", err, response)
	}

	return &result, nil
}

// extractJSON はレスポンスからJSON部分を抽出
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start != -1 && end != -1 && end > start {
		return s[start : end+1]
	}
	return s
}

// AnalyzeResults はクエリ結果を分析して要約を生成
func (c *Client) AnalyzeResults(ctx context.Context, userMessage string, results map[string]string, schemaInfo string) (string, error) {
	systemPrompt := `あなたはデータ分析アシスタントです。
ユーザーの質問に対して、クエリ結果を分析し、わかりやすく要約して回答してください。

回答のガイドライン：
- 簡潔でわかりやすい日本語で回答
- 複数のデータソースがある場合は統合して分析
- 重要なポイントを箇条書きで整理
- 数値は適切にフォーマット（カンマ区切りなど）
- ユーザーの質問に直接答える形式で`

	if schemaInfo != "" {
		systemPrompt += "\n\n" + schemaInfo
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("質問: %s\n\n", userMessage))
	sb.WriteString("クエリ結果:\n")
	for name, result := range results {
		sb.WriteString(fmt.Sprintf("\n【%s】\n%s\n", name, result))
	}

	return c.Invoke(ctx, systemPrompt, sb.String())
}
