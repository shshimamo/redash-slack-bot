# redash-slack-bot 評価レポート 
- **評価日時**: 2026-03-01
- **評価時コミット**: `5657744923ad873014393512de1a27e7771ce31f`

---

## 総評

**「プロトタイプとして優秀、本番運用にはいくつかの課題あり」**

設計の骨格はしっかりしており、小規模チームなら今すぐ使える水準。ただし、実際の開発現場で継続運用するには補強が必要な箇所がある。

---

## 強み

**アーキテクチャ**
- 関心の分離が明確（config / redash / llm / slack）
- `resolve_parameters` による自動パラメータ解決は実用的で差別化になる
- Named Redash instances + 環境変数参照は本番環境の運用を考えた設計
- per-investigation の timeout / size limit オーバーライドは柔軟性が高い
- Socket Mode 採用でプライベートネットワークでも動く

**運用面**
- `cmd/validate` による CI チェックがある
- `make schema` で tbls からスキーマ自動生成できる
- Docker multi-stage build で軽量イメージを作れる
- graceful shutdown 実装済み

---

## 課題

### 高優先度（実際に障害や運用問題になる）

**1. LLM モデルが hardcode されている**

`claude-3-haiku-20240307` が `internal/llm/client.go` に直書きされており、古いモデル名。per-investigation でモデルを切り替えたいケースにも対応できない（複雑な分析は Sonnet/Opus、軽い確認は Haiku など）。

**2. リトライが一切ない**

Redash API・Anthropic API ともにリトライなし。一時的なネットワーク障害やレートリミットで即エラーになる。

**3. `pendingRequests` がメモリのみ**

再起動すると未完了の investigation 選択がすべて消える。bot 再起動後に「セレクトボックスを選択しても無反応」という状況が発生する。

**4. `waitForJob` のポーリングが異常 status に対応していない**

```go
switch jobResp.Job.Status {
case 3: // Success
case 4: // Failure
}
// それ以外（1=pending, 2=started）は何もせずループ継続
```

Redash が異常な status を返すと、context timeout まで延々ポーリングし続ける。

**5. テストが皆無**

`go test ./...` が実行できるテストがない。リファクタリングや機能追加のたびにリグレッションリスクが高い。

---

### 中優先度（運用を続けると問題になる）

**6. ユーザー認証・権限制御がない**

Slack にアクセスできる全員が全 investigation を実行できる。機密データへのクエリや高コストなクエリを誰でも実行できる状態。

**7. 監査ログ（audit trail）がない**

「誰がいつどの investigation を実行したか」が残らない。インシデント対応の調査やコンプライアンス上問題になり得る。

**8. レートリミットがない**

同一ユーザーや同一チャンネルから短時間に大量リクエストが来てもスロットリングしない。

**9. `fmt.Printf` と `log` が混在**

```go
// internal/config/config.go では fmt.Printf
fmt.Printf("Loaded schema: %s\n", schemaFile)

// handler_execute.go では log.Printf
log.Printf(...)
```

ログ集約ツール（Datadog, CloudWatch など）でログを収集する際に扱いが困難になる。

**10. Slack メッセージの 3000 文字 truncate が情報ロス**

長い分析結果がブツ切りになる。スレッドに複数投稿する・ファイルとして添付するなどの代替手段がない。

---

### 低優先度（あると便利）

**11. LLM がストリーミング非対応**

長い分析（数十秒）は Slack 上でフィードバックがなく、ユーザーが待たされる感覚が強い。

**12. Redash クエリ結果のキャッシュなし**

同じパラメータで複数回実行しても毎回 Redash にリクエストが飛ぶ。Redash 側の `max_age` パラメータを活用すれば既存キャッシュを使える。

**13. LLM モデルの per-investigation 設定がない**

`model: claude-sonnet-4-6` のように investigation ごとに指定できると、コストとクオリティのバランスを取りやすくなる。

---

## 利用可否まとめ

| シナリオ | 評価 |
|---|---|
| 少人数の開発チーム内ツール（社内限定） | **今すぐ使える** |
| 全社的なインシデント対応ツール | **要補強**（権限制御・監査ログ） |
| 外部公開・SaaS 化 | **要大幅追加**（認証・レートリミット・テスト）|

最も費用対効果が高い改善を一つ挙げるなら **テストの追加**。現状の機能を壊さずに以降の改善を進める基盤になる。
