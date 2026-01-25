FROM golang:1.25-alpine AS builder

WORKDIR /app

# 依存関係をコピーしてダウンロード
COPY go.mod go.sum ./
RUN go mod download

# ソースコードをコピーしてビルド
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bot ./cmd/main.go

# 実行用の軽量イメージ
FROM alpine:3.23

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# バイナリと設定ファイルをコピー
COPY --from=builder /bot /app/bot
COPY configs/ /app/configs/

# 非rootユーザーで実行
RUN adduser -D -g '' appuser
USER appuser

CMD ["/app/bot"]
