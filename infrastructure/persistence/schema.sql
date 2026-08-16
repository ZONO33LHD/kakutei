-- kakutei スキーマ: 確定申告のための複式簿記データベース
--
-- 帳簿 (仕訳・科目・期首残高・監査ログ) はリレーショナルに保持し、
-- 申告資料 (源泉徴収票・控除素材等) は filing_records に JSON ドキュメントとして保持する。
-- ドメイン検証は domain/model の Validate が担い、DB 制約は最終防衛線。

-- 年度管理
CREATE TABLE IF NOT EXISTS fiscal_years (
    year INTEGER PRIMARY KEY,
    state TEXT NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'closed')),
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 勘定科目マスタ
CREATE TABLE IF NOT EXISTS accounts (
    code TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    category TEXT NOT NULL CHECK (category IN ('asset', 'liability', 'equity', 'revenue', 'expense')),
    sub_category TEXT NOT NULL DEFAULT '',
    tax_category TEXT NOT NULL DEFAULT '',
    is_active INTEGER NOT NULL DEFAULT 1,
    sort_order INTEGER NOT NULL DEFAULT 0
);

-- 仕訳ヘッダ
CREATE TABLE IF NOT EXISTS journals (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    fiscal_year INTEGER NOT NULL REFERENCES fiscal_years(year),
    date TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    counterparty TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT '',
    source_file TEXT NOT NULL DEFAULT '',
    is_adjustment INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 仕訳明細 (借方・貸方)
CREATE TABLE IF NOT EXISTS journal_lines (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    journal_id INTEGER NOT NULL REFERENCES journals(id) ON DELETE CASCADE,
    side TEXT NOT NULL CHECK (side IN ('debit', 'credit')),
    account_code TEXT NOT NULL REFERENCES accounts(code),
    amount INTEGER NOT NULL CHECK (amount > 0),
    tax_category TEXT NOT NULL DEFAULT '',
    tax_amount INTEGER NOT NULL DEFAULT 0
);

-- 仕訳の訂正・削除履歴 (電子帳簿保存法施行規則5条5項1号イ準拠)
CREATE TABLE IF NOT EXISTS journal_audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    journal_id INTEGER NOT NULL,
    fiscal_year INTEGER NOT NULL,
    operation TEXT NOT NULL CHECK (operation IN ('update', 'delete')),
    before_snapshot TEXT NOT NULL,
    after_snapshot TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 期首残高
CREATE TABLE IF NOT EXISTS opening_balances (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    fiscal_year INTEGER NOT NULL REFERENCES fiscal_years(year),
    account_code TEXT NOT NULL REFERENCES accounts(code),
    amount INTEGER NOT NULL DEFAULT 0,
    UNIQUE (fiscal_year, account_code)
);

-- 申告資料 (源泉徴収票・扶養親族・寄附金・医療費・保険・繰越損失・固定資産等)
-- kind ごとのドメインエンティティを JSON で保持する。
-- JSON の形式 (Go 構造体のフィールド) を変更する場合は、migrations に
-- data を変換するマイグレーションを必ず追加する (旧形式の黙殺ゼロ値化を防ぐ)。
CREATE TABLE IF NOT EXISTS filing_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    fiscal_year INTEGER NOT NULL REFERENCES fiscal_years(year),
    kind TEXT NOT NULL CHECK (kind <> ''),
    data TEXT NOT NULL CHECK (json_valid(data)),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- インデックス
CREATE UNIQUE INDEX IF NOT EXISTS idx_journals_content_hash ON journals(fiscal_year, content_hash);
CREATE INDEX IF NOT EXISTS idx_journals_fiscal_year_date ON journals(fiscal_year, date);
CREATE INDEX IF NOT EXISTS idx_journal_lines_journal_id ON journal_lines(journal_id);
CREATE INDEX IF NOT EXISTS idx_journal_lines_account_code ON journal_lines(account_code);
CREATE INDEX IF NOT EXISTS idx_journal_audit_logs_journal_id ON journal_audit_logs(journal_id);
CREATE INDEX IF NOT EXISTS idx_journal_audit_logs_fiscal_year ON journal_audit_logs(fiscal_year);
CREATE INDEX IF NOT EXISTS idx_opening_balances_fiscal_year ON opening_balances(fiscal_year);
CREATE INDEX IF NOT EXISTS idx_filing_records_year_kind ON filing_records(fiscal_year, kind);

-- 配偶者情報等「年度に1件」の資料は kind 単位で一意
CREATE UNIQUE INDEX IF NOT EXISTS idx_filing_records_singleton
    ON filing_records(fiscal_year, kind) WHERE kind IN ('spouse');
