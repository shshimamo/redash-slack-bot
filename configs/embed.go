package configs

import "embed"

//go:embed queries.yaml schemas prompts all:documents
var FS embed.FS
