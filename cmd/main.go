package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/shshimamo/redash-slack-bot/configs"
	"github.com/shshimamo/redash-slack-bot/internal/config"
	"github.com/shshimamo/redash-slack-bot/internal/llm"
	"github.com/shshimamo/redash-slack-bot/internal/redash"
	"github.com/shshimamo/redash-slack-bot/internal/slack"
)

func main() {
	// 環境変数から設定を読み込み
	slackBotToken := mustGetEnv("SLACK_BOT_TOKEN")
	slackAppToken := mustGetEnv("SLACK_APP_TOKEN") // Socket Mode 用

	// グループ設定（allowed_groups に指定した環境変数名からメンバーを解決）
	groups := config.NewGroups()

	// 設定ファイル読み込み（バイナリに埋め込まれた configs/ を使用）
	cfg, err := config.LoadConfig(configs.FS, "queries.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Printf("Loaded %d investigations from config", len(cfg.Investigations))

	// 設定の整合性チェック
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Config validation failed: %v", err)
	}

	// investigation ごとのスキーマファイル読み込み
	if err := cfg.LoadInvestigationSchemas(configs.FS, "schemas"); err != nil {
		log.Fatalf("Failed to load investigation schemas: %v", err)
	}

	// investigation ごとのプロンプトファイル読み込み
	if err := cfg.LoadInvestigationPrompts(configs.FS, "prompts"); err != nil {
		log.Fatalf("Failed to load investigation prompts: %v", err)
	}

	// investigation ごとのドキュメントファイル読み込み
	if err := cfg.LoadInvestigationDocuments(configs.FS, "documents"); err != nil {
		log.Fatalf("Failed to load investigation documents: %v", err)
	}

	// LLM クライアント初期化
	provider := getEnv("LLM_PROVIDER", "anthropic")
	var llmClient llm.Client
	switch provider {
	case "anthropic":
		llmClient = llm.NewAnthropicClient(mustGetEnv("ANTHROPIC_API_KEY"))
	case "bedrock":
		llmClient = llm.NewBedrockClient()
	case "openai":
		llmClient = llm.NewOpenAIClient(mustGetEnv("OPENAI_API_KEY"))
	default:
		log.Fatalf("Unknown LLM_PROVIDER: %s (anthropic / bedrock / openai)", provider)
	}
	log.Printf("LLM provider: %s", provider)

	// Redash クライアント初期化（redash_instances の定義に基づく）
	redashClients := make(map[string]*redash.Client)
	for _, inst := range cfg.RedashInstances {
		url := os.Getenv(inst.URLEnv)
		apiKey := os.Getenv(inst.APIKeyEnv)
		if url == "" || apiKey == "" {
			log.Fatalf("Redash instance %q: env vars %s and %s must be set", inst.Name, inst.URLEnv, inst.APIKeyEnv)
		}
		redashClients[inst.Name] = redash.NewClient(url, apiKey)
		log.Printf("Redash client initialized for instance: %s", inst.Name)
	}
	log.Printf("Redash clients initialized (%d instances)", len(redashClients))

	// クエリ並列実行数（デフォルト: 5）
	queryConcurrency := 5
	if v := os.Getenv("QUERY_CONCURRENCY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			log.Fatalf("QUERY_CONCURRENCY must be a positive integer, got: %s", v)
		}
		queryConcurrency = n
	}
	log.Printf("Query concurrency: %d", queryConcurrency)

	// クエリ結果の最大サイズ（デフォルト: 10000 bytes、0 で無制限）
	defaultQueryResultMaxBytes := 10000
	if v := os.Getenv("QUERY_RESULT_MAX_BYTES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			log.Fatalf("QUERY_RESULT_MAX_BYTES must be a non-negative integer, got: %s", v)
		}
		defaultQueryResultMaxBytes = n
	}
	log.Printf("Query result max bytes: %d", defaultQueryResultMaxBytes)

	// LLM 入力合計の最大サイズ（デフォルト: 50000 bytes、0 で無制限）
	defaultLLMInputMaxBytes := 50000
	if v := os.Getenv("LLM_INPUT_MAX_BYTES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			log.Fatalf("LLM_INPUT_MAX_BYTES must be a non-negative integer, got: %s", v)
		}
		defaultLLMInputMaxBytes = n
	}
	log.Printf("LLM input max bytes: %d", defaultLLMInputMaxBytes)

	// タイムアウト設定（デフォルト: 120s）
	defaultTimeout := 120 * time.Second
	if v := os.Getenv("INVESTIGATION_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			log.Fatalf("INVESTIGATION_TIMEOUT must be a valid positive duration (e.g. 120s, 2m), got: %s", v)
		}
		defaultTimeout = d
	}
	log.Printf("Investigation default timeout: %s", defaultTimeout)

	// LLM モデル設定（デフォルト: claude-haiku-4-5-20251001）
	defaultModel := getEnv("LLM_MODEL", "claude-haiku-4-5-20251001")
	log.Printf("LLM default model: %s", defaultModel)

	// Slack ハンドラ初期化（Socket Mode）
	handler := slack.NewHandler(slackBotToken, slackAppToken, llmClient, redashClients, cfg, groups, queryConcurrency, defaultQueryResultMaxBytes, defaultLLMInputMaxBytes, defaultTimeout, defaultModel)

	// Context with cancel for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		cancel()
	}()

	// Socket Mode で起動
	log.Println("Starting bot in Socket Mode...")
	if err := handler.Run(ctx); err != nil {
		log.Fatalf("Error running handler: %v", err)
	}

	log.Println("Bot stopped")
}

func mustGetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("Required environment variable %s is not set", key)
	}
	return value
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
