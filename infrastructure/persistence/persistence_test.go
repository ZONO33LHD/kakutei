package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/repository"
	"github.com/ZONO33LHD/kakutei/infrastructure/db"
)

// newTestDB はマイグレーション済みのテスト用 DB を返す。
// 標準の科目マスタと年度 2025 (open) を投入する。
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	sqlDB, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := Migrate(ctx, sqlDB); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := NewAccountRepository(sqlDB).SaveAll(ctx, model.DefaultAccounts()); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}
	if err := NewFiscalYearRepository(sqlDB).Create(ctx, 2025); err != nil {
		t.Fatalf("FiscalYear Create: %v", err)
	}
	return sqlDB
}

func testDate(t *testing.T, s string) model.Date {
	t.Helper()
	d, err := model.ParseDate(s)
	if err != nil {
		t.Fatalf("ParseDate: %v", err)
	}
	return d
}

func sampleEntry(t *testing.T, date, counterparty string, amount model.Money) *model.JournalEntry {
	t.Helper()
	return &model.JournalEntry{
		FiscalYear:   2025,
		Date:         testDate(t, date),
		Description:  "テスト仕訳",
		Counterparty: counterparty,
		Lines: []model.JournalLine{
			{Side: model.SideDebit, AccountCode: "1002", Amount: amount},
			{Side: model.SideCredit, AccountCode: "4001", Amount: amount, TaxCategory: model.LineTaxTaxable10},
		},
		Source: model.SourceManual,
	}
}

func assertCode(t *testing.T, err error, want apperrors.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("エラーになるべき (want %s)", want)
	}
	if got := apperrors.CodeOf(err); got != want {
		t.Fatalf("エラーコード = %s, want %s (%v)", got, want, err)
	}
}

func TestAccountRepository(t *testing.T) {
	sqlDB := newTestDB(t)
	ctx := context.Background()
	repo := NewAccountRepository(sqlDB)

	accounts, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(accounts) != 72 {
		t.Errorf("科目数 = %d, want 72", len(accounts))
	}
	// コード順
	for i := 1; i < len(accounts); i++ {
		if accounts[i-1].Code >= accounts[i].Code {
			t.Fatalf("コード順でない: %s → %s", accounts[i-1].Code, accounts[i].Code)
		}
	}

	cash, err := repo.FindByCode(ctx, "1001")
	if err != nil || cash.Name != "現金" || cash.Category != model.CategoryAsset {
		t.Errorf("FindByCode(1001) = %+v, err=%v", cash, err)
	}
	_, err = repo.FindByCode(ctx, "9999")
	assertCode(t, err, apperrors.CodeNotFound)

	// SaveAll は冪等 (upsert)
	if err := repo.SaveAll(ctx, model.DefaultAccounts()); err != nil {
		t.Fatalf("SaveAll (2回目): %v", err)
	}
}

func TestFiscalYearRepository(t *testing.T) {
	sqlDB := newTestDB(t)
	ctx := context.Background()
	repo := NewFiscalYearRepository(sqlDB)

	assertCode(t, repo.Create(ctx, 2025), apperrors.CodeConflict) // 既存

	status, err := repo.Find(ctx, 2025)
	if err != nil || status.State != model.FiscalYearOpen {
		t.Fatalf("Find = %+v, err=%v", status, err)
	}
	_, err = repo.Find(ctx, 2030)
	assertCode(t, err, apperrors.CodeNotFound)

	if err := repo.Create(ctx, 2024); err != nil {
		t.Fatalf("Create 2024: %v", err)
	}
	years, err := repo.List(ctx)
	if err != nil || len(years) != 2 || years[0].Year != 2024 {
		t.Fatalf("List = %+v, err=%v", years, err)
	}

	if err := repo.UpdateState(ctx, 2025, model.FiscalYearClosed); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	status, _ = repo.Find(ctx, 2025)
	if status.State != model.FiscalYearClosed {
		t.Errorf("State = %s, want closed", status.State)
	}
	assertCode(t, repo.UpdateState(ctx, 2030, model.FiscalYearOpen), apperrors.CodeNotFound)
}

func TestJournalRepositoryCRUD(t *testing.T) {
	sqlDB := newTestDB(t)
	ctx := context.Background()
	repo := NewJournalRepository(sqlDB)
	auditRepo := NewJournalAuditRepository(sqlDB)

	entry := sampleEntry(t, "2025-04-01", "取引先A", 110_000)
	id, err := repo.Create(ctx, entry)
	if err != nil || id == 0 {
		t.Fatalf("Create: id=%d err=%v", id, err)
	}

	// 取得して全フィールドが一致すること
	got, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Date.String() != "2025-04-01" || got.Counterparty != "取引先A" ||
		got.Source != model.SourceManual || len(got.Lines) != 2 {
		t.Errorf("復元結果が不一致: %+v", got)
	}
	if got.Lines[1].TaxCategory != model.LineTaxTaxable10 || got.Lines[1].Amount != 110_000 {
		t.Errorf("明細の復元が不一致: %+v", got.Lines[1])
	}

	// 同一内容は一意制約で CodeConflict
	_, err = repo.Create(ctx, sampleEntry(t, "2025-04-01", "取引先A", 110_000))
	assertCode(t, err, apperrors.CodeConflict)

	// 取引先が違えば登録できる (ハッシュに取引先を含む)
	if _, err := repo.Create(ctx, sampleEntry(t, "2025-04-01", "取引先B", 110_000)); err != nil {
		t.Fatalf("別取引先の登録: %v", err)
	}

	// 更新 → 監査ログが同一トランザクションで記録される
	got.Description = "更新後"
	got.Lines[0].Amount = 220_000
	got.Lines[1].Amount = 220_000
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, _ := repo.FindByID(ctx, id)
	if updated.Description != "更新後" || updated.TotalDebit() != 220_000 {
		t.Errorf("更新が反映されていない: %+v", updated)
	}
	logs, err := auditRepo.ListByJournalID(ctx, id)
	if err != nil || len(logs) != 1 {
		t.Fatalf("監査ログ = %d件, err=%v", len(logs), err)
	}
	if logs[0].Operation != model.AuditUpdate || !json.Valid([]byte(logs[0].BeforeSnapshot)) {
		t.Errorf("監査ログが不正: %+v", logs[0])
	}
	// BeforeSnapshot は更新前の状態
	var before model.JournalEntry
	if err := json.Unmarshal([]byte(logs[0].BeforeSnapshot), &before); err != nil {
		t.Fatalf("BeforeSnapshot の復元: %v", err)
	}
	if before.Description != "テスト仕訳" {
		t.Errorf("BeforeSnapshot が更新前でない: %q", before.Description)
	}

	// 年度の変更は拒否
	badYear := *updated
	badYear.FiscalYear = 2024
	assertCode(t, repo.Update(ctx, &badYear), apperrors.CodeBadRequest)

	// 削除 → 監査ログ追加
	if err := repo.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = repo.FindByID(ctx, id)
	assertCode(t, err, apperrors.CodeNotFound)
	logs, _ = auditRepo.ListByJournalID(ctx, id)
	if len(logs) != 2 || logs[1].Operation != model.AuditDelete {
		t.Errorf("削除の監査ログがない: %+v", logs)
	}
	// 明細もカスケード削除されている
	var lineCount int
	_ = sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM journal_lines WHERE journal_id = ?", id).Scan(&lineCount)
	if lineCount != 0 {
		t.Errorf("明細が残っている: %d件", lineCount)
	}

	assertCode(t, repo.Delete(ctx, 99999), apperrors.CodeNotFound)
}

func TestJournalRepositoryClosedYear(t *testing.T) {
	sqlDB := newTestDB(t)
	ctx := context.Background()
	repo := NewJournalRepository(sqlDB)

	entry := sampleEntry(t, "2025-04-01", "", 10_000)
	id, err := repo.Create(ctx, entry)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := NewFiscalYearRepository(sqlDB).UpdateState(ctx, 2025, model.FiscalYearClosed); err != nil {
		t.Fatalf("close: %v", err)
	}

	_, err = repo.Create(ctx, sampleEntry(t, "2025-05-01", "", 20_000))
	assertCode(t, err, apperrors.CodeConflict)

	stored, _ := repo.FindByID(ctx, id)
	assertCode(t, repo.Update(ctx, stored), apperrors.CodeConflict)
	assertCode(t, repo.Delete(ctx, id), apperrors.CodeConflict)

	// 未作成年度への登録は BadRequest
	future := sampleEntry(t, "2026-01-01", "", 10_000)
	future.FiscalYear = 2026
	_, err = repo.Create(ctx, future)
	assertCode(t, err, apperrors.CodeBadRequest)
}

func TestJournalRepositorySearch(t *testing.T) {
	sqlDB := newTestDB(t)
	ctx := context.Background()
	repo := NewJournalRepository(sqlDB)

	seed := []struct {
		date, counterparty string
		amount             model.Money
	}{
		{"2025-01-10", "A社", 10_000},
		{"2025-03-15", "B社", 50_000},
		{"2025-06-20", "A社", 100_000},
	}
	for _, s := range seed {
		if _, err := repo.Create(ctx, sampleEntry(t, s.date, s.counterparty, s.amount)); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	// 日付範囲
	entries, total, err := repo.Search(ctx, repository.JournalSearchQuery{
		FiscalYear: 2025, DateFrom: testDate(t, "2025-02-01"), DateTo: testDate(t, "2025-12-31"),
	})
	if err != nil || total != 2 || len(entries) != 2 {
		t.Errorf("日付範囲: total=%d len=%d err=%v", total, len(entries), err)
	}

	// 取引先の部分一致
	_, total, _ = repo.Search(ctx, repository.JournalSearchQuery{
		FiscalYear: 2025, CounterpartyContains: "A社",
	})
	if total != 2 {
		t.Errorf("取引先検索: total=%d, want 2", total)
	}

	// 金額範囲 (借方合計)
	_, total, _ = repo.Search(ctx, repository.JournalSearchQuery{
		FiscalYear: 2025, AmountMin: 50_000, AmountMax: 60_000,
	})
	if total != 1 {
		t.Errorf("金額範囲: total=%d, want 1", total)
	}

	// 科目コード
	_, total, _ = repo.Search(ctx, repository.JournalSearchQuery{
		FiscalYear: 2025, AccountCode: "4001",
	})
	if total != 3 {
		t.Errorf("科目検索: total=%d, want 3", total)
	}

	// LIKE メタ文字がエスケープされる
	_, total, _ = repo.Search(ctx, repository.JournalSearchQuery{
		FiscalYear: 2025, DescriptionContains: "100%",
	})
	if total != 0 {
		t.Errorf("%%のエスケープ: total=%d, want 0", total)
	}

	// ページネーション
	entries, total, _ = repo.Search(ctx, repository.JournalSearchQuery{
		FiscalYear: 2025, Limit: 2, Offset: 2,
	})
	if total != 3 || len(entries) != 1 {
		t.Errorf("ページネーション: total=%d len=%d", total, len(entries))
	}

	// 重複候補: 同一日付
	candidates, err := repo.FindDuplicateCandidates(ctx, 2025, "nohash", testDate(t, "2025-01-10"))
	if err != nil || len(candidates) != 1 {
		t.Errorf("重複候補: len=%d err=%v", len(candidates), err)
	}
}

func TestJournalRepositoryCreateBatch(t *testing.T) {
	sqlDB := newTestDB(t)
	ctx := context.Background()
	repo := NewJournalRepository(sqlDB)

	entries := []model.JournalEntry{
		*sampleEntry(t, "2025-02-01", "A", 10_000),
		*sampleEntry(t, "2025-02-02", "B", 20_000),
	}
	ids, err := repo.CreateBatch(ctx, entries)
	if err != nil || len(ids) != 2 {
		t.Fatalf("CreateBatch: ids=%v err=%v", ids, err)
	}

	// バッチ内に重複があれば全件ロールバック
	dup := []model.JournalEntry{
		*sampleEntry(t, "2025-03-01", "C", 30_000),
		*sampleEntry(t, "2025-03-01", "C", 30_000),
	}
	if _, err := repo.CreateBatch(ctx, dup); err == nil {
		t.Fatal("バッチ内重複はエラーになるべき")
	}
	all, _ := repo.ListByFiscalYear(ctx, 2025)
	if len(all) != 2 {
		t.Errorf("ロールバックされていない: %d件", len(all))
	}
}

func TestOpeningBalanceRepository(t *testing.T) {
	sqlDB := newTestDB(t)
	ctx := context.Background()
	repo := NewOpeningBalanceRepository(sqlDB)

	if err := repo.Upsert(ctx, &model.OpeningBalance{FiscalYear: 2025, AccountCode: "1001", Amount: 100_000}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	// 同一キーは更新
	if err := repo.Upsert(ctx, &model.OpeningBalance{FiscalYear: 2025, AccountCode: "1001", Amount: 200_000}); err != nil {
		t.Fatalf("Upsert (更新): %v", err)
	}
	balances, err := repo.ListByFiscalYear(ctx, 2025)
	if err != nil || len(balances) != 1 || balances[0].Amount != 200_000 {
		t.Fatalf("List: %+v err=%v", balances, err)
	}

	found, err := repo.FindByID(ctx, balances[0].ID)
	if err != nil || found.AccountCode != "1001" {
		t.Errorf("FindByID: %+v err=%v", found, err)
	}

	if err := repo.Delete(ctx, balances[0].ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	assertCode(t, repo.Delete(ctx, balances[0].ID), apperrors.CodeNotFound)
}

func TestFilingRepository(t *testing.T) {
	sqlDB := newTestDB(t)
	ctx := context.Background()
	repo := NewDependentRepository(sqlDB)

	dep := &model.Dependent{
		FiscalYear: 2025, Name: "子", Relationship: "子",
		BirthDate: testDate(t, "2010-01-01"), Cohabiting: true,
	}
	id, err := repo.Create(ctx, dep)
	if err != nil || id == 0 || dep.ID != id {
		t.Fatalf("Create: id=%d dep.ID=%d err=%v", id, dep.ID, err)
	}

	got, err := repo.FindByID(ctx, id)
	if err != nil || got.Name != "子" || got.BirthDate.String() != "2010-01-01" || !got.Cohabiting {
		t.Fatalf("FindByID: %+v err=%v", got, err)
	}

	// 更新
	got.Income = 500_000
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, _ := repo.FindByID(ctx, id)
	if updated.Income != 500_000 {
		t.Errorf("更新未反映: %+v", updated)
	}

	// 年度変更は拒否
	badYear := *updated
	badYear.FiscalYear = 2024
	assertCode(t, repo.Update(ctx, &badYear), apperrors.CodeBadRequest)

	// 一覧
	list, err := repo.ListByFiscalYear(ctx, 2025)
	if err != nil || len(list) != 1 {
		t.Fatalf("List: len=%d err=%v", len(list), err)
	}

	// 別 kind とは分離されている
	lossRepo := NewLossCarryforwardRepository(sqlDB)
	if _, err := lossRepo.Create(ctx, &model.LossCarryforward{
		FiscalYear: 2025, LossYear: 2024, Amount: 300_000,
	}); err != nil {
		t.Fatalf("LossCarryforward Create: %v", err)
	}
	list, _ = repo.ListByFiscalYear(ctx, 2025)
	if len(list) != 1 {
		t.Errorf("kind の分離が効いていない: %d件", len(list))
	}

	// 削除
	if err := repo.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = repo.FindByID(ctx, id)
	assertCode(t, err, apperrors.CodeNotFound)
}

func TestFilingRepositoryClosedYear(t *testing.T) {
	sqlDB := newTestDB(t)
	ctx := context.Background()
	repo := NewMedicalExpenseRepository(sqlDB)

	expense := &model.MedicalExpense{
		FiscalYear: 2025, Date: testDate(t, "2025-03-01"),
		PatientName: "本人", MedicalInstitution: "病院", Amount: 10_000,
	}
	id, err := repo.Create(ctx, expense)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := NewFiscalYearRepository(sqlDB).UpdateState(ctx, 2025, model.FiscalYearClosed); err != nil {
		t.Fatalf("close: %v", err)
	}
	_, err = repo.Create(ctx, expense)
	assertCode(t, err, apperrors.CodeConflict)
	assertCode(t, repo.Update(ctx, expense), apperrors.CodeConflict)
	assertCode(t, repo.Delete(ctx, id), apperrors.CodeConflict)
}

func TestSpouseRepository(t *testing.T) {
	sqlDB := newTestDB(t)
	ctx := context.Background()
	repo := NewSpouseRepository(sqlDB)

	// 未登録は (nil, nil)
	spouse, err := repo.FindByFiscalYear(ctx, 2025)
	if err != nil || spouse != nil {
		t.Fatalf("未登録: %+v err=%v", spouse, err)
	}

	if err := repo.Upsert(ctx, &model.Spouse{
		FiscalYear: 2025, Name: "花子", BirthDate: testDate(t, "1990-05-01"), Income: 500_000,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	spouse, err = repo.FindByFiscalYear(ctx, 2025)
	if err != nil || spouse == nil || spouse.Name != "花子" {
		t.Fatalf("Find: %+v err=%v", spouse, err)
	}

	// 上書き (年度に1件)
	if err := repo.Upsert(ctx, &model.Spouse{
		FiscalYear: 2025, Name: "花子", BirthDate: testDate(t, "1990-05-01"), Income: 900_000,
	}); err != nil {
		t.Fatalf("Upsert (更新): %v", err)
	}
	spouse, _ = repo.FindByFiscalYear(ctx, 2025)
	if spouse.Income != 900_000 {
		t.Errorf("上書きが反映されていない: %+v", spouse)
	}
	var count int
	_ = sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM filing_records WHERE kind = 'spouse'").Scan(&count)
	if count != 1 {
		t.Errorf("配偶者情報が複数件: %d", count)
	}

	if err := repo.DeleteByFiscalYear(ctx, 2025); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	spouse, _ = repo.FindByFiscalYear(ctx, 2025)
	if spouse != nil {
		t.Error("削除されていない")
	}
}

// 全 filing リポジトリの登録→取得ラウンドトリップ。
func TestAllFilingRepositories(t *testing.T) {
	sqlDB := newTestDB(t)
	ctx := context.Background()
	d := testDate(t, "2025-06-01")

	// roundTrip は Create → ListByFiscalYear で1件返ることを検証する共通ヘルパ。
	roundTrip := func(name string, create func() (int64, error), count func() (int, error)) {
		t.Run(name, func(t *testing.T) {
			id, err := create()
			if err != nil || id == 0 {
				t.Fatalf("Create: id=%d err=%v", id, err)
			}
			n, err := count()
			if err != nil || n != 1 {
				t.Fatalf("List: n=%d err=%v", n, err)
			}
		})
	}

	ws := NewWithholdingSlipRepository(sqlDB)
	roundTrip("源泉徴収票",
		func() (int64, error) {
			return ws.Create(ctx, &model.WithholdingSlip{FiscalYear: 2025, PayerName: "X社", PaymentAmount: 5_000_000, WithheldTax: 100_000})
		},
		func() (int, error) { l, err := ws.ListByFiscalYear(ctx, 2025); return len(l), err })

	fd := NewFurusatoDonationRepository(sqlDB)
	roundTrip("ふるさと納税",
		func() (int64, error) {
			return fd.Create(ctx, &model.FurusatoDonation{FiscalYear: 2025, Municipality: "○○市", Amount: 10_000, Date: d})
		},
		func() (int, error) { l, err := fd.ListByFiscalYear(ctx, 2025); return len(l), err })

	dn := NewDonationRepository(sqlDB)
	roundTrip("寄附金",
		func() (int64, error) {
			return dn.Create(ctx, &model.DonationRecord{FiscalYear: 2025, Kind: model.DonationNPO, RecipientName: "NPO", Amount: 5_000, Date: d})
		},
		func() (int, error) { l, err := dn.ListByFiscalYear(ctx, 2025); return len(l), err })

	si := NewSocialInsuranceRepository(sqlDB)
	roundTrip("社会保険料",
		func() (int64, error) {
			return si.Create(ctx, &model.SocialInsuranceItem{FiscalYear: 2025, Kind: model.SocialInsuranceNationalPension, Amount: 200_000})
		},
		func() (int, error) { l, err := si.ListByFiscalYear(ctx, 2025); return len(l), err })

	ip := NewInsurancePolicyRepository(sqlDB)
	roundTrip("保険契約",
		func() (int64, error) {
			return ip.Create(ctx, &model.InsurancePolicy{FiscalYear: 2025, Kind: model.PolicyLifeGeneralNew, CompanyName: "○○生命", Premium: 60_000})
		},
		func() (int, error) { l, err := ip.ListByFiscalYear(ctx, 2025); return len(l), err })

	bw := NewBusinessWithholdingRepository(sqlDB)
	roundTrip("事業源泉徴収",
		func() (int64, error) {
			return bw.Create(ctx, &model.BusinessWithholding{FiscalYear: 2025, ClientName: "取引先", GrossAmount: 1_000_000, WithheldTax: 102_100})
		},
		func() (int, error) { l, err := bw.ListByFiscalYear(ctx, 2025); return len(l), err })

	hl := NewHousingLoanRepository(sqlDB)
	roundTrip("住宅ローン控除明細",
		func() (int64, error) {
			return hl.Create(ctx, &model.HousingLoanDetail{FiscalYear: 2025, Kind: model.HousingUsed, Category: "general", MoveInDate: d, YearEndBalance: 10_000_000})
		},
		func() (int, error) { l, err := hl.ListByFiscalYear(ctx, 2025); return len(l), err })

	fa := NewFixedAssetRepository(sqlDB)
	roundTrip("固定資産",
		func() (int64, error) {
			return fa.Create(ctx, &model.FixedAsset{FiscalYear: 2025, Name: "PC", AcquisitionDate: d, AcquisitionCost: 300_000, UsefulLife: 4, Method: model.DepreciationStraightLine, BusinessUseRatioPct: 100})
		},
		func() (int, error) { l, err := fa.ListByFiscalYear(ctx, 2025); return len(l), err })

	oi := NewOtherIncomeRepository(sqlDB)
	roundTrip("その他所得",
		func() (int64, error) {
			return oi.Create(ctx, &model.OtherIncome{FiscalYear: 2025, Kind: model.OtherIncomeMiscellaneous, Description: "原稿料", Revenue: 100_000})
		},
		func() (int, error) { l, err := oi.ListByFiscalYear(ctx, 2025); return len(l), err })
}

// Money・Date が JSON 経由で正しく往復することの確認 (filing_records の前提)。
func TestFilingRecordJSONRoundTrip(t *testing.T) {
	original := model.HousingLoanDetail{
		FiscalYear: 2025, Kind: model.HousingUsed, Category: "general",
		MoveInDate: testDate(t, "2025-04-01"), YearEndBalance: 15_151_931,
		AcquisitionCost: 42_800_000, DualApplicationGroup: "loan-01", CostForProration: 42_800_000,
	}
	data, err := json.Marshal(&original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var restored model.HousingLoanDetail
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if restored != original {
		t.Errorf("往復で不一致:\n got %+v\nwant %+v", restored, original)
	}
}

// エラー分類ヘルパの検証。
func TestIsUniqueViolation(t *testing.T) {
	if isUniqueViolation(nil) {
		t.Error("nil は一意制約違反ではない")
	}
	if isUniqueViolation(errors.New("some other error")) {
		t.Error("無関係のエラーを誤判定")
	}
}
