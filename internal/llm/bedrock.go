package llm

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/bedrock"
)

// NewBedrockClient は AWS Bedrock 経由の Anthropic クライアントを作成。
// AWS 認証は環境変数 / ~/.aws/credentials / IAM ロールで自動解決される。
// リージョンは AWS_DEFAULT_REGION または AWS_REGION で指定すること。
func NewBedrockClient() Client {
	c := anthropic.NewClient(bedrock.WithLoadDefaultConfig(context.Background()))
	return &anthropicClient{client: &c}
}
