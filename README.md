# kakutei

日本の個人事業主 (青色申告) 向けの確定申告バックエンド。複式簿記の帳簿管理から所得税・消費税の計算、申告前チェックまでを Go + クリーンアーキテクチャで実装しています。

対応課税年度: **令和7年分 (2025年)・令和8年分 (2026年)**。税制定数は `domain/policy` に一元管理され、年度依存の値は `XxxFor(year)` 関数で提供します。他年度への誤適用は入口で拒否されます。

## アーキテクチャ

```text
cmd/server/            エントリポイント (環境変数で設定、graceful shutdown)
registry/              DI コンテナ (全レイヤーの合成)
interface/rest/        REST API (net/http、apperrors → HTTP ステータス変換)
usecase/               アプリケーションサービス
                       (材料収集・オーケストレーション。計算はドメインへ委譲)
domain/
  model/               エンティティ・値オブジェクト (Money, Date, 仕訳, 申告資料)
  policy/              税制定数・速算表 (令和7・8年分)
  service/
    taxation/          税計算ドメインサービス (所得税・消費税・年金・退職・減価償却)
    bookkeeping/       簿記ドメインサービス (財務諸表・元帳・重複検出・消費税集計)
  repository/          永続化の契約 (インターフェース)
  apperrors/           エラーコード付きエラー型
infrastructure/
  db/                  SQLite 接続 (modernc.org/sqlite, pure Go)
  persistence/         リポジトリ実装 (帳簿はリレーショナル、申告資料は JSON ドキュメント)
```

依存方向: `interface → usecase → domain ← infrastructure`。
ドメインサービス (純粋な計算・検証) とアプリケーションサービス (リポジトリからの材料収集・調整) を明確に分離しています。

## 主な機能

### 帳簿管理 (複式簿記)
- 仕訳の登録・訂正・削除 (**電子帳簿保存法対応の訂正削除履歴**を同一トランザクションで記録)
- 重複検出: 内容ハッシュの完全一致はブロック、同日同額の類似は警告 (`force` で登録可)
- 残高試算表・損益計算書・貸借対照表 (期首残高対応)・総勘定元帳
- 標準勘定科目マスタ 72 科目 (個人事業・青色申告用)
- 年度の締め (締め後の変更はすべて拒否)

### 税額計算 (令和7年分・令和8年分)
- **所得税**: 給与所得 (速算表の A 方式・所得金額調整控除)、事業所得 (青色申告特別控除の利益上限自動調整)、公的年金等・雑・配当・一時所得、損益通算の法定順序、純損失の繰越控除
- **所得控除**: 基礎 (R7改正)・社会保険料・生命保険料 (3区分新旧)・地震保険料 (旧長期対応)・医療費 (セルフメディケーションと有利選択)・寄附金・配偶者 (老人控除対象配偶者)・扶養 (同居老親等・特定親族特別控除)・寡婦/ひとり親・障害者・勤労学生
- **税額控除**: 住宅ローン控除 (重複適用の按分・取得対価上限・所得要件)、配当控除 (1,000万円閾値の按分)、寄附金特別控除 (政党等30%/認定NPO40%/公益社団等40% を所得控除と**最大8パターン比較で自動有利選択**)
- **消費税**: 本則 (課税売上割合による一括比例配分対応)・簡易・2割特例。帳簿の税区分から自動集計
- ふるさと納税の集計と控除上限推定、減価償却 (定額/定率)、申告前サニティチェック (10項目)

## セットアップ

要件: Go 1.26.6+ (CGO 不要)

```bash
make build          # bin/server を生成
make run            # ローカル起動 (既定: 127.0.0.1:8080, kakutei.db)
make test           # go test ./... -race -cover
make lint           # gofmt チェック + go vet + golangci-lint
```

環境変数:

| 変数 | 既定値 | 説明 |
|---|---|---|
| `KAKUTEI_DB_PATH` | `kakutei.db` | SQLite ファイルパス |
| `KAKUTEI_ADDR` | `127.0.0.1:8080` | 待受アドレス |
| `KAKUTEI_ALLOW_NONLOOPBACK` | 未設定 | `1` で非ループバック待受を許可 (既定は起動拒否。認証がないため要注意) |
| `KAKUTEI_LOG_LEVEL` | `info` | ログレベル (`debug`/`info`/`warn`/`error`) |
| `KAKUTEI_LOG_FORMAT` | `json` | ログ形式 (`json`/`text`) |
| `KAKUTEI_LOG_TRACE` | `on` | エラーログの詳細トレース (`off` で無効化。トレースには絶対パス・原因メッセージが含まれる) |

## API 概要

JSON フィールド名は Go の公開フィールド名 (PascalCase)。金額は円単位の整数、日付は `YYYY-MM-DD`。
変更系リクエストには `Content-Type: application/json` が必須、一覧系は `?year=` が必須です。

```bash
# 年度の作成 (勘定科目マスタも初期投入)
curl -X POST localhost:8080/api/fiscal-years \
  -H 'Content-Type: application/json' -d '{"Year": 2025}'

# 仕訳の登録
curl -X POST localhost:8080/api/journals \
  -H 'Content-Type: application/json' -d '{
  "Entry": {
    "FiscalYear": 2025, "Date": "2025-04-01", "Description": "サービス売上",
    "Lines": [
      {"Side": "debit",  "AccountCode": "1002", "Amount": 110000},
      {"Side": "credit", "AccountCode": "4001", "Amount": 110000, "TaxCategory": "taxable_10"}
    ],
    "Source": "manual"
  }
}'

# 財務諸表
curl 'localhost:8080/api/reports/profit-and-loss?year=2025'

# 所得税の計算 (帳簿と申告資料から自動集計)
curl -X POST 'localhost:8080/api/filing/income-tax?year=2025' \
  -H 'Content-Type: application/json' -d '{"BlueReturnDeduction": 650000}'

# 消費税の計算 (2割特例)
curl -X POST 'localhost:8080/api/filing/consumption-tax?year=2025' \
  -H 'Content-Type: application/json' -d '{"Method": "special_20pct"}'
```

主要エンドポイント:

| パス | 説明 |
|---|---|
| `POST/GET /api/fiscal-years`, `POST .../{year}/close`, `POST .../{year}/reopen` | 年度管理 |
| `POST/GET /api/journals`, `POST .../batch`, `GET/PUT/DELETE .../{id}`, `GET .../{id}/audit-logs`, `GET .../duplicates` | 仕訳・監査ログ・重複スキャン |
| `GET /api/accounts` | 勘定科目マスタ |
| `GET /api/reports/{trial-balance,profit-and-loss,balance-sheet,general-ledger}` | 財務諸表 |
| `POST/GET /api/opening-balances`, `DELETE .../{id}` | 期首残高 |
| `/api/materials/{withholding-slips,dependents,furusato-donations,donations,medical-expenses,social-insurances,insurance-policies,business-withholdings,loss-carryforwards,housing-loans,fixed-assets,other-incomes}` | 申告資料の CRUD |
| `PUT/GET/DELETE /api/materials/spouse` | 配偶者情報 (年度1件) |
| `POST /api/filing/{income-tax,consumption-tax,sanity-check,furusato-summary}`, `GET /api/filing/depreciation` | 申告計算 |

## 設計上の判断・既知の制限

- **単一ユーザーのローカル利用が前提**: 認証なし・既定でループバック待受。SQLite を採用。ブラウザ経由の攻撃 (CSRF/DNS rebinding) 対策として、変更系リクエストは `Content-Type: application/json` を必須とし、Host ヘッダをローカルホスト系に限定
- **非ループバック待受は既定で起動拒否**: 認証がないため、`KAKUTEI_ADDR` をループバック以外にする場合は `KAKUTEI_ALLOW_NONLOOPBACK=1` の明示的なオプトインが必要。その場合も **HostGuard は Host ヘッダ検証にすぎず認証の代替にはならない**ので、リバースプロキシ等で必ず認証を追加すること
- 保険料控除は保険契約 (`insurance-policies`) を正とし、未登録時のみ源泉徴収票の欄をフォールバック (二重計上防止)
- 減価償却は計算結果を返すのみで自動起票しない (決算整理仕訳の起票材料)
- 未対応: 消費税の個別対応方式 (一括比例配分のみ)、簡易課税の複数事業区分/75%特例、給与+年金の所得金額調整控除、証券投資信託の配当控除軽減率、株式/FX/仮想通貨の分離課税、CSV取込・OCR・PDF出力・e-Tax連携

## 免責

本ソフトウェアの計算結果は参考情報です。申告内容の正確性は利用者自身で確認し、必要に応じて税理士等の専門家に相談してください。
