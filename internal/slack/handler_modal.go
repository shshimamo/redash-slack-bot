package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/slack-go/slack"

	"github.com/shshimamo/redash-slack-bot/internal/config"
)

// processMessage はメッセージを処理して調査選択 UI を返す
func (h *Handler) processMessage(ctx context.Context, channel, user, text, threadTimestamp string) {
	// メンションを除去
	cleanText := strings.TrimSpace(text)
	if strings.HasPrefix(cleanText, "<@") {
		if idx := strings.Index(cleanText, ">"); idx != -1 {
			cleanText = strings.TrimSpace(cleanText[idx+1:])
		}
	}

	log.Printf("Processing message from user %s", user)

	// requestID は channel + threadTimestamp で一意に識別
	requestID := fmt.Sprintf("%s_%s", channel, threadTimestamp)

	// pendingRequests に保存
	h.mu.Lock()
	h.pendingRequests[requestID] = pendingRequest{
		Channel:  channel,
		ThreadTimestamp: threadTimestamp,
	}
	h.mu.Unlock()

	// 30分後にクリーンアップ
	time.AfterFunc(30*time.Minute, func() {
		h.mu.Lock()
		delete(h.pendingRequests, requestID)
		h.mu.Unlock()
	})

	// 実行権限のある調査のみ表示
	var options []*slack.OptionBlockObject
	for _, inv := range h.config.Investigations {
		if !h.groups.IsMember(user, inv.AllowedGroups) {
			continue
		}
		options = append(options, &slack.OptionBlockObject{
			Text:  &slack.TextBlockObject{Type: slack.PlainTextType, Text: inv.Name},
			Value: inv.Name,
		})
	}

	if len(options) == 0 {
		h.sendMessage(channel, threadTimestamp, "実行できる調査がありません。")
		return
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

	_, selectMsgTS, err := h.slackClient.PostMessage(
		channel,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionTS(threadTimestamp),
	)
	if err != nil {
		log.Printf("Error posting select message: %v", err)
		return
	}

	// セレクトボックスのメッセージ TS を保存（選択後に上書きするため）
	h.mu.Lock()
	if req, ok := h.pendingRequests[requestID]; ok {
		req.SelectMessageTimestamp = selectMsgTS
		h.pendingRequests[requestID] = req
	}
	h.mu.Unlock()
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

	// セレクトボックスをテキストに置き換え
	if req.SelectMessageTimestamp != "" {
		_, _, _, err := h.slackClient.UpdateMessage(
			req.Channel,
			req.SelectMessageTimestamp,
			slack.MsgOptionText(fmt.Sprintf("「%s」を実行します。", investigationName), false),
		)
		if err != nil {
			log.Printf("Error updating select message: %v", err)
		}
	}

	if len(investigation.Parameters) == 0 {
		// パラメータなし → 直接実行
		h.executeInvestigation(ctx, req.Channel, req.ThreadTimestamp, investigationName, nil)
	} else {
		// パラメータあり → モーダルを開く
		h.openParameterModal(callback.TriggerID, investigation, req)
	}
}

// openParameterModal はパラメータ入力モーダルを開く
func (h *Handler) openParameterModal(triggerID string, investigation *config.InvestigationConfig, req pendingRequest) {
	metadata := modalPrivateMetadata{
		Channel:           req.Channel,
		ThreadTimestamp:          req.ThreadTimestamp,
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

	// 入力されたパラメータを表示
	if len(parameters) > 0 {
		var sb strings.Builder
		sb.WriteString("パラメータ:\n")
		for _, param := range investigation.Parameters {
			if val, ok := parameters[param.Name]; ok {
				sb.WriteString(fmt.Sprintf("• %s: %v\n", param.Name, val))
			}
		}
		h.sendMessage(metadata.Channel, metadata.ThreadTimestamp, sb.String())
	}

	h.executeInvestigation(ctx, metadata.Channel, metadata.ThreadTimestamp, metadata.InvestigationName, parameters)
}
