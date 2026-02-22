package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
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
	Channel     string
	ThreadTS    string
	UserMessage string
}

// modalPrivateMetadata はモーダルの private_metadata に格納するデータ
type modalPrivateMetadata struct {
	Channel           string `json:"channel"`
	ThreadTS          string `json:"thread_ts"`
	UserMessage       string `json:"user_message"`
	InvestigationName string `json:"investigation_name"`
}

// Handler は Slack イベントを処理するハンドラ
type Handler struct {
	slackClient     *slack.Client
	socketClient    *socketmode.Client
	llmClient       *llm.Client
	redashClient    *redash.Client
	config          *config.Config
	pendingRequests map[string]pendingRequest
	mu              sync.Mutex
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
		slackClient:     slackClient,
		socketClient:    socketClient,
		llmClient:       llmClient,
		redashClient:    redashClient,
		config:          cfg,
		pendingRequests: make(map[string]pendingRequest),
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

// processMessage はメッセージを処理して調査選択UIを返す
func (h *Handler) processMessage(ctx context.Context, channel, user, text, threadTS string) {
	// メンションを除去
	cleanText := strings.TrimSpace(text)
	if strings.HasPrefix(cleanText, "<@") {
		if idx := strings.Index(cleanText, ">"); idx != -1 {
			cleanText = strings.TrimSpace(cleanText[idx+1:])
		}
	}

	log.Printf("Processing message from user %s: %s", user, cleanText)

	// requestID は channel + threadTS で一意に識別
	requestID := fmt.Sprintf("%s_%s", channel, threadTS)

	// pendingRequests に保存
	h.mu.Lock()
	h.pendingRequests[requestID] = pendingRequest{
		Channel:     channel,
		ThreadTS:    threadTS,
		UserMessage: cleanText,
	}
	h.mu.Unlock()

	// 30分後にクリーンアップ
	time.AfterFunc(30*time.Minute, func() {
		h.mu.Lock()
		delete(h.pendingRequests, requestID)
		h.mu.Unlock()
	})

	// 調査選択のセレクトボックスを投稿
	options := make([]*slack.OptionBlockObject, 0, len(h.config.Investigations))
	for _, inv := range h.config.Investigations {
		options = append(options, &slack.OptionBlockObject{
			Text:  &slack.TextBlockObject{Type: slack.PlainTextType, Text: inv.Name},
			Value: inv.Name,
		})
	}

	selectElement := &slack.SelectBlockElement{
		Type:        slack.OptTypeStatic,
		Placeholder: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "調査を選択してください"},
		ActionID:    "select_investigation",
		Options:     options,
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			&slack.TextBlockObject{Type: slack.MarkdownType, Text: "どの調査を実行しますか？"},
			nil, nil,
		),
		slack.NewActionBlock(requestID, selectElement),
	}

	_, _, err := h.slackClient.PostMessage(
		channel,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionTS(threadTS),
	)
	if err != nil {
		log.Printf("Error posting select message: %v", err)
	}
}

// handleBlockActions はブロックアクションを処理
func (h *Handler) handleBlockActions(ctx context.Context, callback slack.InteractionCallback) {
	if len(callback.ActionCallback.BlockActions) == 0 {
		return
	}

	action := callback.ActionCallback.BlockActions[0]
	if action.ActionID != "select_investigation" {
		return
	}

	requestID := action.BlockID
	investigationName := action.SelectedOption.Value

	h.mu.Lock()
	req, ok := h.pendingRequests[requestID]
	h.mu.Unlock()

	if !ok {
		log.Printf("Pending request not found for requestID: %s", requestID)
		return
	}

	investigation := h.config.GetInvestigationByName(investigationName)
	if investigation == nil {
		log.Printf("Investigation not found: %s", investigationName)
		return
	}

	if len(investigation.Parameters) == 0 {
		// パラメータなし → 直接実行
		h.executeInvestigation(ctx, req.Channel, req.ThreadTS, req.UserMessage, investigationName, nil)
	} else {
		// パラメータあり → モーダルを開く
		h.openParameterModal(callback.TriggerID, investigation, req)
	}
}

// openParameterModal はパラメータ入力モーダルを開く
func (h *Handler) openParameterModal(triggerID string, investigation *config.InvestigationConfig, req pendingRequest) {
	metadata := modalPrivateMetadata{
		Channel:           req.Channel,
		ThreadTS:          req.ThreadTS,
		UserMessage:       req.UserMessage,
		InvestigationName: investigation.Name,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		log.Printf("Error marshaling modal metadata: %v", err)
		return
	}

	var inputBlocks []slack.Block
	for _, param := range investigation.Parameters {
		label := param.Description
		if label == "" {
			label = param.Name
		}

		var element slack.BlockElement
		if param.Type == "date" {
			element = &slack.DatePickerBlockElement{
				Type:     slack.METDatepicker,
				ActionID: param.Name,
			}
		} else {
			element = &slack.PlainTextInputBlockElement{
				Type:     slack.METPlainTextInput,
				ActionID: param.Name,
			}
		}

		inputBlock := &slack.InputBlock{
			Type:    slack.MBTInput,
			BlockID: param.Name,
			Label:   &slack.TextBlockObject{Type: slack.PlainTextType, Text: label},
			Element: element,
		}
		inputBlocks = append(inputBlocks, inputBlock)
	}

	modal := slack.ModalViewRequest{
		Type:            slack.VTModal,
		Title:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: investigation.Name},
		Submit:          &slack.TextBlockObject{Type: slack.PlainTextType, Text: "実行"},
		Close:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "キャンセル"},
		CallbackID:      "parameter_input",
		PrivateMetadata: string(metadataJSON),
		Blocks:          slack.Blocks{BlockSet: inputBlocks},
	}

	_, err = h.slackClient.OpenView(triggerID, modal)
	if err != nil {
		log.Printf("Error opening modal: %v", err)
	}
}

// handleViewSubmission はモーダルの送信を処理
func (h *Handler) handleViewSubmission(ctx context.Context, callback slack.InteractionCallback) {
	if callback.View.CallbackID != "parameter_input" {
		return
	}

	var metadata modalPrivateMetadata
	if err := json.Unmarshal([]byte(callback.View.PrivateMetadata), &metadata); err != nil {
		log.Printf("Error parsing modal metadata: %v", err)
		return
	}

	investigation := h.config.GetInvestigationByName(metadata.InvestigationName)
	if investigation == nil {
		log.Printf("Investigation not found: %s", metadata.InvestigationName)
		return
	}

	// パラメータ値を取得
	parameters := make(map[string]interface{})
	for _, param := range investigation.Parameters {
		blockValues, ok := callback.View.State.Values[param.Name]
		if !ok {
			continue
		}
		actionValue, ok := blockValues[param.Name]
		if !ok {
			continue
		}
		if param.Type == "date" {
			parameters[param.Name] = actionValue.SelectedDate
		} else {
			parameters[param.Name] = actionValue.Value
		}
	}

	h.executeInvestigation(ctx, metadata.Channel, metadata.ThreadTS, metadata.UserMessage, metadata.InvestigationName, parameters)
}


// executeInvestigation は調査を実行
func (h *Handler) executeInvestigation(ctx context.Context, channel, threadTS, userMessage, investigationName string, parameters map[string]interface{}) {
	investigation := h.config.GetInvestigationByName(investigationName)
	if investigation == nil {
		log.Printf("Investigation not found: %s", investigationName)
		h.sendMessage(channel, threadTS, fmt.Sprintf("調査「%s」が見つかりませんでした。", investigationName))
		return
	}

	// パラメータのコピーを作成（元のマップを変更しないため）
	params := make(map[string]interface{})
	for k, v := range parameters {
		params[k] = v
	}

	log.Printf("Executing investigation: %s with parameters: %v", investigation.Name, params)

	// resolve_parameters フェーズ: 追加パラメータをクエリで解決
	for _, resolve := range investigation.ResolveParameters {
		log.Printf("Resolving parameters: %s (query ID: %d)", resolve.Name, resolve.QueryID)

		result, err := h.redashClient.ExecuteQuery(resolve.QueryID, params)
		if err != nil {
			log.Printf("Warning: resolve query %d failed: %v", resolve.QueryID, err)
			continue
		}

		if len(result.Rows) == 0 {
			log.Printf("Warning: resolve query %d returned 0 rows, skipping outputs", resolve.QueryID)
			continue
		}

		// 1行目からフィールドを抽出してパラメータにマージ
		var row map[string]interface{}
		if err := json.Unmarshal(result.Rows[0], &row); err != nil {
			log.Printf("Warning: failed to parse resolve query %d result: %v", resolve.QueryID, err)
			continue
		}

		for _, output := range resolve.Outputs {
			if val, ok := row[output.Field]; ok {
				params[output.Name] = val
				log.Printf("Resolved parameter %s = %v", output.Name, val)
			} else {
				log.Printf("Warning: field %q not found in resolve query %d result", output.Field, resolve.QueryID)
			}
		}
	}

	// 複数クエリの場合は進行状況を通知
	if len(investigation.Queries) > 1 {
		h.sendMessage(channel, threadTS, fmt.Sprintf("「%s」を実行中... (%d件のクエリ)", investigation.Name, len(investigation.Queries)))
	}

	// 各クエリを実行（未解決パラメータがあるクエリはスキップ）
	results := make(map[string]string)
	for _, query := range investigation.Queries {
		if missing := missingParams(query.RequiredParameters, params); len(missing) > 0 {
			log.Printf("Skipping query %q (ID: %d): unresolved parameters: %v", query.Name, query.ID, missing)
			continue
		}

		log.Printf("Executing query: %s (ID: %d)", query.Name, query.ID)

		// required_parameters で指定されたパラメータのみ渡す
		queryParams := filterParams(query.RequiredParameters, params)
		result, err := h.redashClient.ExecuteQuery(query.ID, queryParams)
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

	if len(results) == 0 {
		h.sendMessage(channel, threadTS, "実行できるクエリがありませんでした。パラメータを確認してください。")
		return
	}

	// 結果を分析
	analysis, err := h.llmClient.AnalyzeResults(ctx, userMessage, results)
	if err != nil {
		log.Printf("Error analyzing results: %v", err)
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

// filterParams は required で指定されたキーのみ抽出して返す
func filterParams(required []string, params map[string]interface{}) map[string]interface{} {
	if len(required) == 0 {
		return params
	}
	filtered := make(map[string]interface{}, len(required))
	for _, name := range required {
		if val, ok := params[name]; ok {
			filtered[name] = val
		}
	}
	return filtered
}

// missingParams は required のうち params に存在しないものを返す
func missingParams(required []string, params map[string]interface{}) []string {
	var missing []string
	for _, name := range required {
		if _, ok := params[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
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
