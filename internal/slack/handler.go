package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/shshimamo/redash-slack-bot/internal/config"
	"github.com/shshimamo/redash-slack-bot/internal/llm"
	"github.com/shshimamo/redash-slack-bot/internal/redash"
)

// Handler は Slack イベントを処理するハンドラ
type Handler struct {
	slackClient  *slack.Client
	socketClient *socketmode.Client
	llmClient    *llm.Client
	redashClient *redash.Client
	config       *config.Config
}

// NewHandler は新しいハンドラを作成
func NewHandler(
	botToken string,
	appToken string,
	llmClient *llm.Client,
	redashClient *redash.Client,
	cfg *config.Config,
) *Handler {
	slackClient := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)
	socketClient := socketmode.New(slackClient)

	return &Handler{
		slackClient:  slackClient,
		socketClient: socketClient,
		llmClient:    llmClient,
		redashClient: redashClient,
		config:       cfg,
	}
}

// Run は Socket Mode でイベントを受信し続ける
func (h *Handler) Run(ctx context.Context) error {
	go h.handleEvents(ctx)
	return h.socketClient.RunContext(ctx)
}

// handleEvents はイベントを処理するループ
func (h *Handler) handleEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-h.socketClient.Events:
			switch evt.Type {
			case socketmode.EventTypeEventsAPI:
				eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					continue
				}
				h.socketClient.Ack(*evt.Request)
				h.handleEventsAPI(ctx, eventsAPIEvent)

			case socketmode.EventTypeConnecting:
				log.Println("Connecting to Slack...")

			case socketmode.EventTypeConnected:
				log.Println("Connected to Slack!")

			case socketmode.EventTypeConnectionError:
				log.Println("Connection error, retrying...")
			}
		}
	}
}

// handleEventsAPI は Events API イベントを処理
func (h *Handler) handleEventsAPI(ctx context.Context, event slackevents.EventsAPIEvent) {
	switch event.Type {
	case slackevents.CallbackEvent:
		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			go h.handleAppMention(ctx, ev)
		}
	}
}

// handleAppMention はメンションイベントを処理
func (h *Handler) handleAppMention(ctx context.Context, ev *slackevents.AppMentionEvent) {
	// スレッド内ならそのスレッドに、そうでなければ元メッセージをスレッド親にする
	threadTS := ev.ThreadTimeStamp
	if threadTS == "" {
		threadTS = ev.TimeStamp
	}
	h.processMessage(ctx, ev.Channel, ev.User, ev.Text, threadTS)
}

// processMessage はメッセージを処理して応答を返す
func (h *Handler) processMessage(ctx context.Context, channel, user, text, threadTS string) {
	// メンションを除去
	cleanText := strings.TrimSpace(text)
	if strings.HasPrefix(cleanText, "<@") {
		if idx := strings.Index(cleanText, ">"); idx != -1 {
			cleanText = strings.TrimSpace(cleanText[idx+1:])
		}
	}

	log.Printf("Processing message from user %s: %s", user, cleanText)

	// 調査選択
	queriesInfo := h.config.FormatQueriesForLLM()
	schemaInfo := h.config.FormatSchemaForLLM()

	selection, err := h.llmClient.SelectQuery(ctx, cleanText, queriesInfo, schemaInfo)
	if err != nil {
		log.Printf("Error selecting investigation: %v", err)
		h.sendMessage(channel, threadTS, "申し訳ありません。リクエストの処理中にエラーが発生しました。")
		return
	}

	if !selection.CanHandle {
		h.sendMessage(channel, threadTS, selection.Message)
		return
	}

	// 調査を実行
	h.executeInvestigation(ctx, channel, threadTS, cleanText, selection)
}

// executeInvestigation は調査を実行
func (h *Handler) executeInvestigation(ctx context.Context, channel, threadTS, userMessage string, selection *llm.SelectionResult) {
	investigation := h.config.GetInvestigationByName(selection.InvestigationName)
	if investigation == nil {
		log.Printf("Investigation not found: %s", selection.InvestigationName)
		h.sendMessage(channel, threadTS, fmt.Sprintf("調査「%s」が見つかりませんでした。", selection.InvestigationName))
		return
	}

	log.Printf("Executing investigation: %s with parameters: %v", investigation.Name, selection.Parameters)

	// 複数クエリの場合は進行状況を通知
	if len(investigation.Queries) > 1 {
		h.sendMessage(channel, threadTS, fmt.Sprintf("「%s」を実行中... (%d件のクエリ)", investigation.Name, len(investigation.Queries)))
	}

	// 各クエリを実行
	results := make(map[string]string)
	for _, query := range investigation.Queries {
		log.Printf("Executing query: %s (ID: %d)", query.Name, query.ID)

		result, err := h.redashClient.ExecuteQuery(query.ID, selection.Parameters)
		if err != nil {
			log.Printf("Error executing query %d: %v", query.ID, err)
			results[query.Name] = fmt.Sprintf("エラー: %v", err)
			continue
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			log.Printf("Error marshaling result for query %d: %v", query.ID, err)
			results[query.Name] = "結果のシリアライズに失敗"
			continue
		}

		results[query.Name] = string(resultJSON)
	}

	// 結果を分析
	analysis, err := h.llmClient.AnalyzeResults(ctx, userMessage, results)
	if err != nil {
		log.Printf("Error analyzing results: %v", err)
		// 分析に失敗した場合は生のデータを返す
		var sb strings.Builder
		sb.WriteString("クエリ結果:\n")
		for name, result := range results {
			sb.WriteString(fmt.Sprintf("\n【%s】\n```\n%s\n```\n", name, result))
		}
		h.sendMessage(channel, threadTS, sb.String())
		return
	}

	h.sendMessage(channel, threadTS, analysis)
}

// sendMessage はSlackにメッセージを送信
func (h *Handler) sendMessage(channel, threadTS, text string) {
	opts := []slack.MsgOption{
		slack.MsgOptionText(text, false),
	}
	if threadTS != "" {
		opts = append(opts, slack.MsgOptionTS(threadTS))
	}

	_, _, err := h.slackClient.PostMessage(channel, opts...)
	if err != nil {
		log.Printf("Error sending message: %v", err)
	}
}
