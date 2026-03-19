# Redash Slack Bot

自然言語で Redash クエリを実行・解析する Slack Bot。

## 機能

- Slack でメンションして質問
- LLM が適切な「調査」を選択してパラメータを抽出
- 調査に紐づくクエリを Redash で実行し、結果を統合分析
- LLM が結果を解析して回答
- 複数の LLM プロバイダーに対応（Anthropic / AWS Bedrock / OpenAI）
- グループ単位のアクセス制御

## セットアップ

### 1. Slack App を作成

1. https://api.slack.com/apps にアクセス
2. **Create New App** → **From an app manifest** を選択
3. Workspace を選択
4. `slack-app-manifest.yaml` の内容を貼り付け
5. **Create** をクリック

### 2. App-Level Token を生成

1. Basic Information > App-Level Tokens
2. **Generate Token and Scopes**
3. Token Name: 任意（例: `socket-token`）
4. Scopes: `connections:write` を追加
5. **Generate**
6. 表示される `xapp-...` をメモ → `SLACK_APP_TOKEN`

### 3. 環境変数を設定

```bash
cp .env.example .env
```

`.env` を編集:

| 変数名 | 必須 | 説明 |
|--------|------|------|
| `SLACK_BOT_TOKEN` | ✅ | Slack Bot Token（`xoxb-...`） |
| `SLACK_APP_TOKEN` | ✅ | Slack App-Level Token（`xapp-...`） |
| `LLM_PROVIDER` | | LLM プロバイダー（`anthropic` / `bedrock` / `openai`、デフォルト: `anthropic`） |
| `ANTHROPIC_API_KEY` | LLM_PROVIDER=anthropic 時 | Anthropic API キー |
| `OPENAI_API_KEY` | LLM_PROVIDER=openai 時 | OpenAI API キー |
| `AWS_DEFAULT_REGION` | LLM_PROVIDER=bedrock 時 | AWS リージョン（例: `us-east-1`） |
| `LLM_MODEL` | | 使用モデル（デフォルト: `claude-haiku-4-5-20251001`） |
| `REDASH_URL` | ✅ | Redash の URL |
| `REDASH_API_KEY` | ✅ | Redash API キー |
| `CONFIG_PATH` | | 調査設定ファイルパス（デフォルト: `configs/queries.yaml`） |
| `任意の環境変数名` | | グループメンバー。カンマ区切りの Slack ユーザー ID。`allowed_groups` に指定した名前と一致させる |
| `QUERY_CONCURRENCY` | | クエリ並列実行数（デフォルト: `5`） |
| `QUERY_RESULT_MAX_BYTES` | | クエリ結果の最大サイズ（デフォルト: `10000`、`0` で無制限） |
| `LLM_INPUT_MAX_BYTES` | | LLM 入力の最大サイズ（デフォルト: `50000`、`0` で無制限） |
| `INVESTIGATION_TIMEOUT` | | 調査タイムアウト（デフォルト: `120s`） |

#### LLM プロバイダー別の設定

**Anthropic（デフォルト）**
```env
LLM_PROVIDER=anthropic
ANTHROPIC_API_KEY=sk-ant-your-api-key
LLM_MODEL=claude-haiku-4-5-20251001
```

**AWS Bedrock**
```env
LLM_PROVIDER=bedrock
AWS_DEFAULT_REGION=us-east-1
LLM_MODEL=anthropic.claude-3-5-sonnet-20241022-v2:0
# AWS 認証は環境変数 / ~/.aws/credentials / IAM ロールで自動解決
```

**OpenAI**
```env
LLM_PROVIDER=openai
OPENAI_API_KEY=sk-your-openai-api-key
LLM_MODEL=gpt-4o-mini
```

### 4. グループ設定

`allowed_groups` には環境変数名を指定します。環境変数の値にカンマ区切りで Slack ユーザー ID を設定してください。

```env
PAYMENT_TEAM_USERS=UXXXXXXXXX,UYYYYYYYYY
ANALYTICS_TEAM_USERS=UXXXXXXXXX
```

> Slack ユーザー ID はプロフィール → ︙ メニューから確認できます。

### 5. 調査設定

`configs/queries.yaml` に Redash インスタンスと調査を定義:

```yaml
redash_instances:
  - name: "default"
    url_env: "REDASH_URL"
    api_key_env: "REDASH_API_KEY"

investigations:
  - name: "決済状況調査"
    description: "request_id に紐づく決済状況を総合的に調査"
    redash_instance: "default"
    timeout: "120s"
    allowed_groups:
      - PAYMENT_TEAM_USERS  # 環境変数名を指定（未指定は全員拒否）
    schemas:
      - test_db.json        # configs/schemas/ 配下のスキーマファイル
    parameters:
      - name: request_id
        type: string
        description: "リクエストID"
    queries:
      - id: 1
        name: "決済情報"
        description: "基本的な決済情報を取得"
        required_parameters:
          - request_id
      - id: 2
        name: "決済ログ"
        description: "決済処理のログを取得"
        required_parameters:
          - request_id
```

### 6. スキーマ設定（オプション）

`configs/schemas/` にDB スキーマを配置すると、LLM がより適切に分析できます。

[tbls](https://github.com/k1LoW/tbls) で生成した JSON をそのまま使用可能:

```bash
tbls out --format json postgres://user:pass@localhost:5432/mydb > configs/schemas/mydb.json
```

### 7. 追加ドキュメント設定（オプション）

`configs/documents/` に任意のファイルを配置し、調査設定で `documents` に指定すると LLM に追加情報として渡せます。シーケンス図・仕様書・正常系データなど用途は自由です。

```
configs/documents/
├── payment_sequence.md   # シーケンス図
├── payment_spec.txt      # 仕様書
└── normal_data.json      # 正常系データ
```

```yaml
investigations:
  - name: "決済状況調査"
    documents:
      - payment_sequence.md
      - payment_spec.txt
      - normal_data.json
```

### 8. 実行

```bash
# ローカル実行
make dev

# または Docker
make docker-run
```

## コマンド一覧

```bash
make dev          # ローカル実行（go run）
make run          # ローカル実行（ビルド後）
make docker-build # Docker イメージビルド
make docker-run   # Docker Compose で実行
```

## 使い方

Slack で Bot をチャンネルに招待し、メンション:

```
@redash-bot request_id abc123 の決済状況を調べて
```

→ 「決済状況調査」が選択され、複数クエリが実行されます。

```
@redash-bot 2024-01-01 から 2024-01-31 の決済を集計して
```

→ 「決済ステータス集計」クエリが実行されます。
