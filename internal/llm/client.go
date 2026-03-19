package llm

import "context"

// Client は LLM クライアントの共通インターフェース
type Client interface {
	AnalyzeResults(ctx context.Context, model string, results map[string]string, systemPrompt, schemaInfo, documentInfo string) (string, error)
}
