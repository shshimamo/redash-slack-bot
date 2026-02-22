package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
)

// executeInvestigation は調査を実行
func (h *Handler) executeInvestigation(ctx context.Context, channel, threadTS, investigationName string, parameters map[string]interface{}) {
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

	// クエリ結果サイズ上限: investigation 指定 → デフォルトの順で適用
	maxBytes := h.queryResultMaxBytes
	if investigation.QueryResultMaxBytes != nil {
		maxBytes = *investigation.QueryResultMaxBytes
	}

	redashClient, err := h.redashClientFor(investigation)
	if err != nil {
		log.Printf("Error: %v", err)
		h.sendMessage(channel, threadTS, fmt.Sprintf("設定エラー: %v", err))
		return
	}

	// resolve_parameters フェーズ: 追加パラメータをクエリで解決
	for _, resolve := range investigation.ResolveParameters {
		log.Printf("Resolving parameters: %s (query ID: %d)", resolve.Name, resolve.QueryID)

		result, err := redashClient.ExecuteQuery(resolve.QueryID, params)
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

	// 実行対象クエリを絞り込む（未解決パラメータがあるクエリはスキップ）
	var runnableQueries []struct {
		name        string
		id          int
		queryParams map[string]interface{}
	}
	for _, query := range investigation.Queries {
		if missing := missingParams(query.RequiredParameters, params); len(missing) > 0 {
			log.Printf("Skipping query %q (ID: %d): unresolved parameters: %v", query.Name, query.ID, missing)
			continue
		}
		runnableQueries = append(runnableQueries, struct {
			name        string
			id          int
			queryParams map[string]interface{}
		}{
			name:        query.Name,
			id:          query.ID,
			queryParams: filterParams(query.RequiredParameters, params),
		})
	}

	// 複数クエリの場合は進行状況を通知
	if len(runnableQueries) > 1 {
		h.sendMessage(channel, threadTS, fmt.Sprintf("「%s」を実行中... (%d件のクエリ)", investigation.Name, len(runnableQueries)))
	}

	// クエリを並列実行（セマフォで同時実行数を制限）
	results := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, h.queryConcurrency)

	for _, q := range runnableQueries {
		wg.Add(1)
		sem <- struct{}{}
		go func(name string, id int, queryParams map[string]interface{}) {
			defer wg.Done()
			defer func() { <-sem }()

			log.Printf("Executing query: %s (ID: %d)", name, id)

			result, err := redashClient.ExecuteQuery(id, queryParams)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				log.Printf("Error executing query %d: %v", id, err)
				results[name] = fmt.Sprintf("エラー: %v", err)
				return
			}

			resultJSON, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				log.Printf("Error marshaling result for query %d: %v", id, err)
				results[name] = "結果のシリアライズに失敗"
				return
			}

			// クエリ結果が上限を超える場合はエラー
			if maxBytes > 0 && len(resultJSON) > maxBytes {
				log.Printf("Query result for %q exceeds limit: %d > %d bytes", name, len(resultJSON), maxBytes)
				results[name] = fmt.Sprintf("エラー: クエリ結果が上限 (%d bytes) を超えています (実際: %d bytes)", maxBytes, len(resultJSON))
				return
			}
			results[name] = string(resultJSON)
		}(q.name, q.id, q.queryParams)
	}
	wg.Wait()

	if len(results) == 0 {
		h.sendMessage(channel, threadTS, "実行できるクエリがありませんでした。パラメータを確認してください。")
		return
	}

	// 結果を分析
	systemPrompt := h.config.GetInvestigationPrompt(investigation)
	schemaInfo := h.config.FormatInvestigationSchemas(investigation)

	// LLM 入力合計サイズチェック: investigation 指定 → デフォルトの順で適用
	maxLLMInput := h.llmInputMaxBytes
	if investigation.LLMInputMaxBytes != nil {
		maxLLMInput = *investigation.LLMInputMaxBytes
	}
	if maxLLMInput > 0 {
		total := len(systemPrompt) + len(schemaInfo)
		for _, r := range results {
			total += len(r)
		}
		if total > maxLLMInput {
			log.Printf("LLM input for %q exceeds limit: %d > %d bytes", investigation.Name, total, maxLLMInput)
			h.sendMessage(channel, threadTS, fmt.Sprintf("エラー: LLM への入力合計が上限 (%d bytes) を超えています (実際: %d bytes)", maxLLMInput, total))
			return
		}
	}

	analysis, err := h.llmClient.AnalyzeResults(ctx, results, systemPrompt, schemaInfo)
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
