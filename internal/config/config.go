package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config はアプリケーション全体の設定
type Config struct {
	Investigations []InvestigationConfig `yaml:"investigations"`
	Schema         *TblsSchema           // tbls の schema.json（オプション）
}

// InvestigationConfig は調査の定義（1件でも複数クエリでも同じ形式）
type InvestigationConfig struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Parameters  []ParameterConfig `yaml:"parameters"`
	Queries     []QueryConfig     `yaml:"queries"`
}

// QueryConfig は Redash クエリの定義
type QueryConfig struct {
	ID          int    `yaml:"id"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	SQL         string `yaml:"sql"`
}

// ParameterConfig はクエリパラメータの定義
type ParameterConfig struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
}

// TblsSchema は tbls の schema.json 形式
type TblsSchema struct {
	Name      string     `json:"name"`
	Tables    []Table    `json:"tables"`
	Relations []Relation `json:"relations,omitempty"`
}

// Table はテーブル定義
type Table struct {
	Name    string   `json:"name"`
	Type    string   `json:"type,omitempty"`
	Comment string   `json:"comment,omitempty"`
	Columns []Column `json:"columns"`
}

// Column はカラム定義
type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Default  string `json:"default,omitempty"`
	Comment  string `json:"comment,omitempty"`
	PK       bool   `json:"pk,omitempty"`
	FK       bool   `json:"fk,omitempty"`
}

// Relation はテーブル間のリレーション
type Relation struct {
	Table         string   `json:"table"`
	Columns       []string `json:"columns"`
	ParentTable   string   `json:"parent_table"`
	ParentColumns []string `json:"parent_columns"`
	Cardinality   string   `json:"cardinality,omitempty"`
}

// LoadConfig は queries.yaml を読み込む
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// LoadSchema は tbls の schema.json を読み込んで Config に追加
func (c *Config) LoadSchema(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}

	var schema TblsSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return fmt.Errorf("failed to parse schema file: %w", err)
	}

	c.Schema = &schema
	return nil
}

// GetInvestigationByName は指定された名前の調査を取得
func (c *Config) GetInvestigationByName(name string) *InvestigationConfig {
	for i := range c.Investigations {
		if c.Investigations[i].Name == name {
			return &c.Investigations[i]
		}
	}
	return nil
}

// FormatForLLM は LLM 向けに調査一覧を整形
func (c *Config) FormatForLLM() string {
	var sb strings.Builder

	sb.WriteString("=== 利用可能な調査 ===\n")
	for _, inv := range c.Investigations {
		sb.WriteString(fmt.Sprintf("\n【%s】\n", inv.Name))
		sb.WriteString(fmt.Sprintf("  説明: %s\n", inv.Description))
		if len(inv.Parameters) > 0 {
			sb.WriteString("  パラメータ:\n")
			for _, p := range inv.Parameters {
				sb.WriteString(fmt.Sprintf("    - %s (%s): %s\n", p.Name, p.Type, p.Description))
			}
		}
		sb.WriteString(fmt.Sprintf("  実行されるクエリ: %d件\n", len(inv.Queries)))
		for _, q := range inv.Queries {
			sb.WriteString(fmt.Sprintf("    - %s: %s\n", q.Name, q.Description))
		}
	}

	return sb.String()
}

// FormatQueriesForLLM は LLM 向けに調査一覧を整形（後方互換性のため残す）
func (c *Config) FormatQueriesForLLM() string {
	return c.FormatForLLM()
}

// FormatSchemaForLLM は LLM 向けにスキーマ情報を整形
func (c *Config) FormatSchemaForLLM() string {
	if c.Schema == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("データベーススキーマ (%s):\n\n", c.Schema.Name))

	for _, table := range c.Schema.Tables {
		sb.WriteString(fmt.Sprintf("テーブル: %s", table.Name))
		if table.Comment != "" {
			sb.WriteString(fmt.Sprintf(" - %s", table.Comment))
		}
		sb.WriteString("\n")

		for _, col := range table.Columns {
			nullable := "NOT NULL"
			if col.Nullable {
				nullable = "NULL"
			}
			pk := ""
			if col.PK {
				pk = " [PK]"
			}
			fk := ""
			if col.FK {
				fk = " [FK]"
			}

			sb.WriteString(fmt.Sprintf("  - %s: %s %s%s%s", col.Name, col.Type, nullable, pk, fk))
			if col.Comment != "" {
				sb.WriteString(fmt.Sprintf(" -- %s", col.Comment))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(c.Schema.Relations) > 0 {
		sb.WriteString("リレーション:\n")
		for _, rel := range c.Schema.Relations {
			sb.WriteString(fmt.Sprintf("  - %s.%s -> %s.%s\n",
				rel.Table,
				strings.Join(rel.Columns, ", "),
				rel.ParentTable,
				strings.Join(rel.ParentColumns, ", "),
			))
		}
	}

	return sb.String()
}
