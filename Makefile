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

# 全パッケージのビルド検証。サーバーバイナリの出力ターゲットは
# cmd/server 追加時 (interface 層の PR) に差し替える。
build:
	CGO_ENABLED=0 go build ./...
