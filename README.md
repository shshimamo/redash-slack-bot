# Redash Slack Bot

自然言語で Redash クエリを実行・解析する Slack Bot。

## 機能

- Slack でメンションして質問
- LLM (Claude Haiku) が適切な「調査」を選択
- 調査に紐づくクエリを実行し、結果を統合分析
- LLM が結果を解析して回答

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

### 4. 調査設定

`configs/queries.yaml` に調査を定義:

```yaml
investigations:
  # 複数クエリをまとめた調査
  - name: "決済状況調査"
    description: "request_id に紐づく決済状況を総合的に調査"
    parameters:
      - name: request_id
        type: string
        description: "リクエストID"
    queries:
      - id: 1
        name: "決済情報"
        description: "基本的な決済情報を取得"
      - id: 2
        name: "決済ログ"
        description: "決済処理のログを取得"

  # 1つのクエリだけの調査も可能
  - name: "決済ステータス集計"
    description: "期間内の決済ステータスごとの件数を集計"
    parameters:
      - name: start_date
        type: date
        description: "開始日"
    queries:
      - id: 100
        name: "ステータス集計"
```

### 5. スキーマ設定（オプション）

`configs/schema.json` に DB スキーマを設定すると、LLM がより適切にクエリを選択できます。

[tbls](https://github.com/k1LoW/tbls) で生成した `schema.json` をそのまま使用できます:

```bash
# tbls でスキーマを出力
tbls out --format json postgres://user:pass@localhost:5432/mydb > configs/schema.json
```

### 6. 実行

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

