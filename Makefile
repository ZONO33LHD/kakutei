.PHONY: tidy fmt fmt-check vet lint test test-cover build run

# 依存関係の整理。go.mod を手で編集した後は必ず実行する。
tidy:
	go mod tidy

# gofmt で全ファイルを整形する。
fmt:
	gofmt -l -w .

# CI 用: 整形されていないファイルがあれば一覧を出して失敗する。
fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt が必要なファイル:"; echo "$$unformatted"; exit 1; \
	fi

vet:
	go vet ./...

# 静的解析一式。gofmt 済みであることも含めて検証する。
lint: fmt-check vet
	golangci-lint run ./...

test:
	go test ./... -race -cover

# カバレッジプロファイルを出力する (詳細確認用)。
test-cover:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

# static binary を bin/server に出力する (pure Go SQLite のため CGO 不要)。
build:
	@mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/server ./cmd/server

# ローカル実行。KAKUTEI_DB_PATH / KAKUTEI_ADDR で上書き可能。
run:
	go run ./cmd/server
