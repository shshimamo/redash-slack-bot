package slack

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/shshimamo/redash-slack-bot/internal/config"
	"github.com/shshimamo/redash-slack-bot/internal/llm"
	"github.com/shshimamo/redash-slack-bot/internal/redash"
)

// pendingRequest はユーザーの選択待ち状態を保持
type pendingRequest struct {
	Channel  string
	ThreadTS string
}

// modalPrivateMetadata はモーダルの private_metadata に格納するデータ
type modalPrivateMetadata struct {
	Channel           string `json:"channel"`
	ThreadTS          string `json:"thread_ts"`
	InvestigationName string `json:"investigation_name"`
}

// slackMessageMaxChars は Slack メッセージの文字数上限
const slackMessageMaxChars = 3000

// Handler は Slack イベントを処理するハンドラ
type Handler struct {
	slackClient         *slack.Client
	socketClient        *socketmode.Client
	llmClient           *llm.Client
	redashClients       map[string]*redash.Client
	config              *config.Config
	pendingRequests     map[string]pendingRequest
	mu                  sync.Mutex
	queryConcurrency    int
	defaultQueryResultMaxBytes int
	defaultLLMInputMaxBytes    int
	defaultTimeout      time.Duration
	defaultModel        string
}

// NewHandler は新しいハンドラを作成
func NewHandler(
	botToken string,
	appToken string,
	llmClient *llm.Client,
	redashClients map[string]*redash.Client,
	cfg *config.Config,
	queryConcurrency int,
	defaultQueryResultMaxBytes int,
	defaultLLMInputMaxBytes int,
	defaultTimeout time.Duration,
	defaultModel string,
) *Handler {
	slackClient := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)
	socketClient := socketmode.New(slackClient)

	return &Handler{
		slackClient:         slackClient,
		socketClient:        socketClient,
		llmClient:           llmClient,
		redashClients:       redashClients,
		config:              cfg,
		pendingRequests:     make(map[string]pendingRequest),
		queryConcurrency:    queryConcurrency,
		defaultQueryResultMaxBytes: defaultQueryResultMaxBytes,
		defaultLLMInputMaxBytes:    defaultLLMInputMaxBytes,
		defaultTimeout:      defaultTimeout,
		defaultModel:        defaultModel,
	}
}

// redashClientFor は investigation の redash_instance に対応するクライアントを返す
// redash_instance が未指定または未定義の場合はエラーを返す
func (h *Handler) redashClientFor(investigation *config.InvestigationConfig) (*redash.Client, error) {
	key := investigation.RedashInstance
	if key == "" {
		return nil, fmt.Errorf("investigation %q: redash_instance is not specified", investigation.Name)
	}
	client, ok := h.redashClients[key]
	if !ok {
		return nil, fmt.Errorf("investigation %q: redash_instance %q is not defined", investigation.Name, key)
	}
	return client, nil
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

			case socketmode.EventTypeInteractive:
				callback, ok := evt.Data.(slack.InteractionCallback)
				if !ok {
					continue
				}
				h.socketClient.Ack(*evt.Request)
				switch callback.Type {
				case slack.InteractionTypeBlockActions:
					go h.handleBlockActions(ctx, callback)
				case slack.InteractionTypeViewSubmission:
					go h.handleViewSubmission(ctx, callback)
				}

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
	threadTS := ev.ThreadTimeStamp
	if threadTS == "" {
		threadTS = ev.TimeStamp
	}
	h.processMessage(ctx, ev.Channel, ev.User, ev.Text, threadTS)
}

// sendMessage は Slack にメッセージを送信
func (h *Handler) sendMessage(channel, threadTS, text string) {
	// Slack のテキスト上限を超える場合はトランケート
	runes := []rune(text)
	if len(runes) > slackMessageMaxChars {
		text = string(runes[:slackMessageMaxChars-14]) + "\n... (truncated)"
	}

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
