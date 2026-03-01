package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// GroupConfig はユーザーグループの定義
type GroupConfig struct {
	Name    string   `yaml:"name"`
	Members []string `yaml:"members"`
}

// Groups はグループ設定全体を保持
type Groups struct {
	Groups []GroupConfig `yaml:"groups"`
	index  map[string]map[string]bool // groupName -> userID -> bool
}

// LoadGroups はグループ設定ファイルを読み込む
func LoadGroups(path string) (*Groups, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read groups file: %w", err)
	}

	var g Groups
	if err := yaml.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("failed to parse groups file: %w", err)
	}

	// O(1) 検索のためインデックスを構築
	g.index = make(map[string]map[string]bool)
	for _, group := range g.Groups {
		members := make(map[string]bool)
		for _, m := range group.Members {
			members[m] = true
		}
		g.index[group.Name] = members
	}

	return &g, nil
}

// IsMember は userID が groupNames のいずれかに所属しているか確認する
func (g *Groups) IsMember(userID string, groupNames []string) bool {
	for _, groupName := range groupNames {
		if members, ok := g.index[groupName]; ok {
			if members[userID] {
				return true
			}
		}
	}
	return false
}
