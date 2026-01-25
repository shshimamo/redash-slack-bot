.PHONY: build run dev docker-build docker-run clean

# ローカルビルド
build:
	go build -o bin/bot ./cmd/main.go

# ローカル実行 (.env ファイルから環境変数を読み込み)
run: build
	@if [ -f .env ]; then \
		export $$(cat .env | grep -v '^#' | xargs) && ./bin/bot; \
	else \
		echo "Error: .env file not found. Copy .env.example to .env and fill in values."; \
		exit 1; \
	fi

# 開発モード (go run)
dev:
	@if [ -f .env ]; then \
		export $$(cat .env | grep -v '^#' | xargs) && go run ./cmd/main.go; \
	else \
		echo "Error: .env file not found. Copy .env.example to .env and fill in values."; \
		exit 1; \
	fi

# Docker ビルド
docker-build:
	docker build -t redash-slack-bot:local .

# Docker 実行
docker-run:
	docker compose up --build

# クリーンアップ
clean:
	rm -rf bin/
	go clean
