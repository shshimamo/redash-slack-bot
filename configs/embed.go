package configs

import "embed"

//go:embed queries.yaml schemas prompts documents
var FS embed.FS
