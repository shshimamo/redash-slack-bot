package redash

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client は Redash API クライアント
type Client struct {
	BaseURL string
	APIKey  string
	client  *http.Client
}

// NewClient は新しい Redash クライアントを作成
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// QueryExecuteResponse はクエリ実行結果
type QueryExecuteResponse struct {
	Job         *QueryJob    `json:"job,omitempty"`
	QueryResult *QueryResult `json:"query_result,omitempty"`
}

type QueryJob struct {
	ID          string       `json:"id"`
	Status      int          `json:"status"` // 1: pending, 2: started, 3: success, 4: failure
	Error       string       `json:"error,omitempty"`
	QueryResult *QueryResult `json:"query_result,omitempty"`
}

type QueryResult struct {
	ID   int             `json:"id"`
	Data json.RawMessage `json:"data"`
}

// QueryResultData はクエリ結果のデータ部分
type QueryResultData struct {
	Columns []Column          `json:"columns"`
	Rows    []json.RawMessage `json:"rows"`
}

type Column struct {
	Name         string `json:"name"`
	FriendlyName string `json:"friendly_name"`
	Type         string `json:"type"`
}

// ExecuteQuery は保存済みクエリを実行
func (c *Client) ExecuteQuery(queryID int, parameters map[string]interface{}) (*QueryResultData, error) {
	url := fmt.Sprintf("%s/api/queries/%d/results", c.BaseURL, queryID)

	var body io.Reader
	if len(parameters) > 0 {
		paramsJSON := map[string]interface{}{
			"parameters": parameters,
		}
		data, err := json.Marshal(paramsJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal parameters: %w", err)
		}
		body = bytes.NewBuffer(data)
	}

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Key %s", c.APIKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result QueryExecuteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var rawData json.RawMessage

	if result.QueryResult != nil {
		rawData = result.QueryResult.Data
	} else if result.Job != nil {
		rawData, err = c.waitForJob(result.Job.ID)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("unexpected response format")
	}

	var data QueryResultData
	if err := json.Unmarshal(rawData, &data); err != nil {
		return nil, fmt.Errorf("failed to parse query result data: %w", err)
	}

	return &data, nil
}

// waitForJob はジョブの完了を待機
func (c *Client) waitForJob(jobID string) (json.RawMessage, error) {
	url := fmt.Sprintf("%s/api/jobs/%s", c.BaseURL, jobID)

	for i := 0; i < 60; i++ {
		time.Sleep(1 * time.Second)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create job status request: %w", err)
		}

		req.Header.Set("Authorization", fmt.Sprintf("Key %s", c.APIKey))

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to get job status: %w", err)
		}

		var jobResp struct {
			Job QueryJob `json:"job"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&jobResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode job status: %w", err)
		}
		resp.Body.Close()

		switch jobResp.Job.Status {
		case 3: // Success
			if jobResp.Job.QueryResult != nil {
				return jobResp.Job.QueryResult.Data, nil
			}
			return nil, fmt.Errorf("query succeeded but no result data")
		case 4: // Failure
			return nil, fmt.Errorf("query failed: %s", jobResp.Job.Error)
		case 1, 2: // Pending or Started
			continue
		}
	}

	return nil, fmt.Errorf("query timeout: job did not complete in 60 seconds")
}
