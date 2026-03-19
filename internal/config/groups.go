package config

import (
	"os"
	"strings"
)

// Groups はグループのアクセス制御を管理する
type Groups struct{}

// NewGroups は Groups を作成する
func NewGroups() *Groups {
	return &Groups{}
}

// IsMember は userID が envVarNames のいずれかの環境変数に含まれているか確認する。
// allowed_groups には環境変数名を指定し、値はカンマ区切りの Slack ユーザー ID。
//
// 例:
//
//	queries.yaml:  allowed_groups: [PAYMENT_TEAM_USERS]
//	環境変数:      PAYMENT_TEAM_USERS=UXXXXXXXXX,UYYYYYYYYY
func (g *Groups) IsMember(userID string, envVarNames []string) bool {
	for _, envVarName := range envVarNames {
		for _, m := range strings.Split(os.Getenv(envVarName), ",") {
			if strings.TrimSpace(m) == userID {
				return true
			}
		}
	}
	return false
}
