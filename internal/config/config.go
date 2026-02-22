package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config はアプリケーション全体の設定
type Config struct {
	Investigations []InvestigationConfig `yaml:"investigations"`
	schemaCache    map[string]string
	promptCache    map[string]string
}

// InvestigationConfig は調査の定義（1件でも複数クエリでも同じ形式）
type InvestigationConfig struct {
	Name              string                   `yaml:"name"`
	Description       string                   `yaml:"description"`
	Prompt            string                   `yaml:"prompt"`
	Parameters        []ParameterConfig        `yaml:"parameters"`
	ResolveParameters []ResolveParameterConfig `yaml:"resolve_parameters"`
	Queries           []QueryConfig            `yaml:"queries"`
	Schemas           []string                 `yaml:"schemas"`
}

// ResolveParameterConfig はパラメータ解決クエリの定義
type ResolveParameterConfig struct {
	Name    string                `yaml:"name"`
	QueryID int                   `yaml:"query_id"`
	Outputs []ResolveOutputConfig `yaml:"outputs"`
}

// ResolveOutputConfig はクエリ結果からパラメータへのマッピング
type ResolveOutputConfig struct {
	Name  string `yaml:"name"`  // 解決後のパラメータ名
	Field string `yaml:"field"` // クエリ結果の列名
}

// QueryConfig は Redash クエリの定義
type QueryConfig struct {
	ID                 int      `yaml:"id"`
	Name               string   `yaml:"name"`
	Description        string   `yaml:"description"`
	RequiredParameters []string `yaml:"required_parameters"`
}

// ParameterConfig はクエリパラメータの定義
type ParameterConfig struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
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

// LoadInvestigationSchemas は configs/schemas/ 配下のスキーマファイルを読み込む
func (c *Config) LoadInvestigationSchemas(schemasDir string) error {
	c.schemaCache = make(map[string]string)

	// 全 investigation が参照するスキーマファイルを収集
	seen := make(map[string]bool)
	for _, inv := range c.Investigations {
		for _, schemaFile := range inv.Schemas {
			seen[schemaFile] = true
		}
	}

	// 各スキーマファイルを読み込む
	for schemaFile := range seen {
		path := filepath.Join(schemasDir, schemaFile)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read schema file %s: %w", schemaFile, err)
		}
		c.schemaCache[schemaFile] = string(data)
		fmt.Printf("Loaded schema: %s\n", schemaFile)
	}

	return nil
}

// LoadInvestigationPrompts は configs/prompts/ 配下のプロンプトファイルを読み込む
func (c *Config) LoadInvestigationPrompts(promptsDir string) error {
	c.promptCache = make(map[string]string)

	// default.txt は常に読み込む
	files := map[string]bool{"default.txt": true}
	for _, inv := range c.Investigations {
		if inv.Prompt != "" {
			files[inv.Prompt] = true
		}
	}

	for file := range files {
		path := filepath.Join(promptsDir, file)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read prompt file %s: %w", file, err)
		}
		c.promptCache[file] = string(data)
		fmt.Printf("Loaded prompt: %s\n", file)
	}

	return nil
}

// GetInvestigationPrompt は investigation のプロンプトを返す。未指定の場合は default.txt を使用
func (c *Config) GetInvestigationPrompt(inv *InvestigationConfig) string {
	if c.promptCache == nil {
		return ""
	}
	file := inv.Prompt
	if file == "" {
		file = "default.txt"
	}
	return c.promptCache[file]
}

// FormatInvestigationSchemas は investigation のスキーマ情報を LLM 向けにフォーマット
func (c *Config) FormatInvestigationSchemas(inv *InvestigationConfig) string {
	if len(inv.Schemas) == 0 || c.schemaCache == nil {
		return ""
	}

	var sb strings.Builder
	for _, schemaFile := range inv.Schemas {
		content, ok := c.schemaCache[schemaFile]
		if !ok {
			continue
		}
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	return sb.String()
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

