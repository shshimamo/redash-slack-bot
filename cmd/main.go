package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/shshimamo/redash-slack-bot/internal/config"
	"github.com/shshimamo/redash-slack-bot/internal/llm"
	"github.com/shshimamo/redash-slack-bot/internal/redash"
	"github.com/shshimamo/redash-slack-bot/internal/slack"
)

func main() {
	// 環境変数から設定を読み込み
	slackBotToken := mustGetEnv("SLACK_BOT_TOKEN")
	slackAppToken := mustGetEnv("SLACK_APP_TOKEN") // Socket Mode 用
	anthropicAPIKey := mustGetEnv("ANTHROPIC_API_KEY")
	configPath := getEnv("CONFIG_PATH", "configs/queries.yaml")
	schemasDir := getEnv("SCHEMAS_DIR", "configs/schemas")
	promptsDir := getEnv("PROMPTS_DIR", "configs/prompts")

	// 設定ファイル読み込み
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Printf("Loaded %d investigations from config", len(cfg.Investigations))

	// 設定の整合性チェック
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Config validation failed: %v", err)
	}

	// investigation ごとのスキーマファイル読み込み
	if err := cfg.LoadInvestigationSchemas(schemasDir); err != nil {
		log.Printf("Warning: Failed to load investigation schemas: %v", err)
	}

	// investigation ごとのプロンプトファイル読み込み
	if err := cfg.LoadInvestigationPrompts(promptsDir); err != nil {
		log.Printf("Warning: Failed to load investigation prompts: %v", err)
	}

	// Anthropic クライアント初期化
	llmClient := llm.NewClient(anthropicAPIKey)
	log.Println("Anthropic client initialized (claude-3-haiku)")

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

	// Slack ハンドラ初期化（Socket Mode）
	handler := slack.NewHandler(slackBotToken, slackAppToken, llmClient, redashClients, cfg)

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
