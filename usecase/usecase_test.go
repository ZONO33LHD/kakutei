package usecase

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/repository"
	"github.com/ZONO33LHD/kakutei/domain/service/bookkeeping"
	"github.com/ZONO33LHD/kakutei/domain/service/taxation"
	"github.com/ZONO33LHD/kakutei/infrastructure/db"
	"github.com/ZONO33LHD/kakutei/infrastructure/persistence"
)

// testEnv は usecase 統合テスト用の全サービス束。実 SQLite を使う。
type testEnv struct {
	fiscalYears FiscalYearUsecase
	journals    JournalUsecase
	reports     ReportUsecase
	materials   *Materials
	filing      FilingUsecase
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx := context.Background()
	sqlDB, err := db.Open(ctx, filepath.Join(t.TempDir(), "usecase.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := persistence.Migrate(ctx, sqlDB); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	journalRepo := persistence.NewJournalRepository(sqlDB)
	accountRepo := persistence.NewAccountRepository(sqlDB)
	auditRepo := persistence.NewJournalAuditRepository(sqlDB)
	openingRepo := persistence.NewOpeningBalanceRepository(sqlDB)
	yearRepo := persistence.NewFiscalYearRepository(sqlDB)

	materials, err := NewMaterials(MaterialRepositories{
		WithholdingSlips:     persistence.NewWithholdingSlipRepository(sqlDB),
		Dependents:           persistence.NewDependentRepository(sqlDB),
		FurusatoDonations:    persistence.NewFurusatoDonationRepository(sqlDB),
		Donations:            persistence.NewDonationRepository(sqlDB),
		MedicalExpenses:      persistence.NewMedicalExpenseRepository(sqlDB),
		SocialInsurances:     persistence.NewSocialInsuranceRepository(sqlDB),
		InsurancePolicies:    persistence.NewInsurancePolicyRepository(sqlDB),
		BusinessWithholdings: persistence.NewBusinessWithholdingRepository(sqlDB),
		LossCarryforwards:    persistence.NewLossCarryforwardRepository(sqlDB),
		HousingLoans:         persistence.NewHousingLoanRepository(sqlDB),
		FixedAssets:          persistence.NewFixedAssetRepository(sqlDB),
		OtherIncomes:         persistence.NewOtherIncomeRepository(sqlDB),
		Spouse:               persistence.NewSpouseRepository(sqlDB),
	})
	if err != nil {
		t.Fatalf("NewMaterials: %v", err)
	}

	statements := bookkeeping.NewStatementService()
	env := &testEnv{
		fiscalYears: NewFiscalYearUsecase(yearRepo, accountRepo),
		journals:    NewJournalUsecase(journalRepo, auditRepo, bookkeeping.NewDuplicateService()),
		reports:     NewReportUsecase(journalRepo, accountRepo, openingRepo, statements),
		materials:   materials,
		filing: NewFilingUsecase(journalRepo, accountRepo, materials, statements,
			taxation.NewIncomeTaxService(), taxation.NewConsumptionTaxService()),
	}
	if err := env.fiscalYears.Setup(ctx, 2025); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	return env
}

func ucDate(t *testing.T, s string) model.Date {
	t.Helper()
	d, err := model.ParseDate(s)
	if err != nil {
		t.Fatalf("ParseDate: %v", err)
	}
	return d
}

func salesEntry(t *testing.T, date string, amount model.Money) *model.JournalEntry {
	t.Helper()
	return &model.JournalEntry{
		FiscalYear: 2025, Date: ucDate(t, date), Description: "売上",
		Lines: []model.JournalLine{
			{Side: model.SideDebit, AccountCode: "1002", Amount: amount},
			{Side: model.SideCredit, AccountCode: "4001", Amount: amount, TaxCategory: model.LineTaxTaxable10},
		},
		Source: model.SourceManual,
	}
}

func TestFiscalYearUsecaseSetup(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Setup は科目マスタも投入済み
	accounts, err := env.reports.ListAccounts(ctx)
	if err != nil || len(accounts) != 72 {
		t.Fatalf("科目 = %d件, err=%v", len(accounts), err)
	}

	years, err := env.fiscalYears.List(ctx)
	if err != nil || len(years) != 1 || years[0].Year != 2025 {
		t.Fatalf("List: %+v err=%v", years, err)
	}

	// 締め → 仕訳追加が拒否される → 再開で追加できる
	if err := env.fiscalYears.Close(ctx, 2025); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err = env.journals.Add(ctx, salesEntry(t, "2025-04-01", 110_000), false)
	if apperrors.CodeOf(err) != apperrors.CodeConflict {
		t.Errorf("締め年度への追加は CONFLICT: %v", err)
	}
	if err := env.fiscalYears.Reopen(ctx, 2025); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if _, err := env.journals.Add(ctx, salesEntry(t, "2025-04-01", 110_000), false); err != nil {
		t.Errorf("再開後の追加: %v", err)
	}
}

func TestJournalUsecaseDuplicateFlow(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	first, err := env.journals.Add(ctx, salesEntry(t, "2025-04-01", 110_000), false)
	if err != nil || first.ID == 0 {
		t.Fatalf("Add: %+v err=%v", first, err)
	}

	// 完全一致は force でもブロック
	_, err = env.journals.Add(ctx, salesEntry(t, "2025-04-01", 110_000), true)
	if apperrors.CodeOf(err) != apperrors.CodeConflict {
		t.Errorf("完全一致は CONFLICT: %v", err)
	}

	// 類似 (同日同額・別科目) は force なしで CONFLICT、force ありで警告付き登録
	similar := &model.JournalEntry{
		FiscalYear: 2025, Date: ucDate(t, "2025-04-01"), Description: "雑収入",
		Lines: []model.JournalLine{
			{Side: model.SideDebit, AccountCode: "1001", Amount: 110_000},
			{Side: model.SideCredit, AccountCode: "4110", Amount: 110_000},
		},
	}
	_, err = env.journals.Add(ctx, similar, false)
	if apperrors.CodeOf(err) != apperrors.CodeConflict {
		t.Errorf("類似は force なしで CONFLICT: %v", err)
	}
	result, err := env.journals.Add(ctx, similar, true)
	if err != nil || len(result.Warnings) != 1 || result.Warnings[0].Kind != bookkeeping.MatchSimilar {
		t.Errorf("force 登録は警告付き: %+v err=%v", result, err)
	}

	// 申告前チェックで類似ペアが検出される
	check, err := env.journals.CheckDuplicates(ctx, 2025, 0)
	if err != nil || check.SuspectedCount != 1 {
		t.Errorf("CheckDuplicates: %+v err=%v", check, err)
	}
}

func TestJournalUsecaseUpdateDeleteAudit(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	added, err := env.journals.Add(ctx, salesEntry(t, "2025-04-01", 110_000), false)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	entry, err := env.journals.Get(ctx, added.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	entry.Description = "売上 (訂正)"
	if err := env.journals.Update(ctx, entry); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := env.journals.Delete(ctx, added.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	logs, err := env.journals.ListAuditLogs(ctx, added.ID)
	if err != nil || len(logs) != 2 {
		t.Fatalf("監査ログ = %d件, err=%v", len(logs), err)
	}

	// ID なしの更新は BadRequest
	noID := salesEntry(t, "2025-05-01", 10_000)
	if apperrors.CodeOf(env.journals.Update(ctx, noID)) != apperrors.CodeBadRequest {
		t.Error("ID なしの更新は BAD_REQUEST")
	}
}

func TestReportUsecase(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	if _, err := env.journals.Add(ctx, salesEntry(t, "2025-02-01", 1_100_000), false); err != nil {
		t.Fatalf("Add: %v", err)
	}
	expense := &model.JournalEntry{
		FiscalYear: 2025, Date: ucDate(t, "2025-03-01"), Description: "通信費",
		Lines: []model.JournalLine{
			{Side: model.SideDebit, AccountCode: "5140", Amount: 110_000, TaxCategory: model.LineTaxTaxable10},
			{Side: model.SideCredit, AccountCode: "1002", Amount: 110_000},
		},
	}
	if _, err := env.journals.Add(ctx, expense, false); err != nil {
		t.Fatalf("Add expense: %v", err)
	}
	// 期首残高は貸借が釣り合う組で登録する (預金 500,000 / 元入金 500,000)
	for _, ob := range []model.OpeningBalance{
		{FiscalYear: 2025, AccountCode: "1002", Amount: 500_000},
		{FiscalYear: 2025, AccountCode: "3001", Amount: 500_000},
	} {
		if err := env.reports.SetOpeningBalance(ctx, &ob); err != nil {
			t.Fatalf("SetOpeningBalance: %v", err)
		}
	}

	tb, err := env.reports.TrialBalance(ctx, 2025)
	if err != nil || !tb.Balanced() {
		t.Fatalf("TrialBalance: balanced=%t err=%v", tb.Balanced(), err)
	}

	pl, err := env.reports.ProfitAndLoss(ctx, 2025)
	if err != nil || pl.NetIncome != 990_000 {
		t.Fatalf("PL: NetIncome=%d err=%v", pl.NetIncome.Yen(), err)
	}

	bs, err := env.reports.BalanceSheet(ctx, 2025)
	if err != nil || !bs.Balanced() {
		t.Fatalf("BS: balanced=%t err=%v", bs.Balanced(), err)
	}
	// 預金: 期首 500,000 + 1,100,000 − 110,000 = 1,490,000
	ledger, err := env.reports.GeneralLedger(ctx, 2025, "1002")
	if err != nil || ledger.ClosingBalance != 1_490_000 {
		t.Fatalf("GeneralLedger: closing=%d err=%v", ledger.ClosingBalance.Yen(), err)
	}

	// 存在しない科目の期首残高は NotFound
	err = env.reports.SetOpeningBalance(ctx, &model.OpeningBalance{
		FiscalYear: 2025, AccountCode: "9999", Amount: 1,
	})
	if apperrors.CodeOf(err) != apperrors.CodeNotFound {
		t.Errorf("未知科目の期首残高: %v", err)
	}
}

func TestMaterialUsecaseValidation(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// ドメイン検証エラーは保存前に拒否される
	_, err := env.materials.MedicalExpenses.Add(ctx, &model.MedicalExpense{
		FiscalYear: 2025, Date: ucDate(t, "2024-03-01"), // 年度外の日付
		PatientName: "本人", MedicalInstitution: "病院", Amount: 10_000,
	})
	if apperrors.CodeOf(err) != apperrors.CodeBadRequest {
		t.Errorf("年度外の医療費は BAD_REQUEST: %v", err)
	}

	id, err := env.materials.MedicalExpenses.Add(ctx, &model.MedicalExpense{
		FiscalYear: 2025, Date: ucDate(t, "2025-03-01"),
		PatientName: "本人", MedicalInstitution: "病院", Amount: 10_000,
	})
	if err != nil || id == 0 {
		t.Fatalf("Add: %v", err)
	}
	list, err := env.materials.MedicalExpenses.List(ctx, 2025)
	if err != nil || len(list) != 1 {
		t.Fatalf("List: %d件 err=%v", len(list), err)
	}
}

func TestMaterialUsecaseCRUDAndSpouse(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// 汎用 CRUD (Get / Update / Delete)
	id, err := env.materials.MedicalExpenses.Add(ctx, &model.MedicalExpense{
		FiscalYear: 2025, Date: ucDate(t, "2025-03-01"),
		PatientName: "本人", MedicalInstitution: "病院", Amount: 10_000,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, err := env.materials.MedicalExpenses.Get(ctx, id)
	if err != nil || got.Amount != 10_000 {
		t.Fatalf("Get: %+v err=%v", got, err)
	}
	got.Amount = 20_000
	if err := env.materials.MedicalExpenses.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// 不正な内容の更新は拒否
	bad := *got
	bad.PatientName = ""
	if apperrors.CodeOf(env.materials.MedicalExpenses.Update(ctx, &bad)) != apperrors.CodeBadRequest {
		t.Error("不正な更新は BAD_REQUEST")
	}
	if err := env.materials.MedicalExpenses.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// 配偶者 (年度1件)
	if err := env.materials.Spouse.Set(ctx, &model.Spouse{
		FiscalYear: 2025, Name: "花子", BirthDate: ucDate(t, "1990-05-01"),
	}); err != nil {
		t.Fatalf("Spouse.Set: %v", err)
	}
	spouse, err := env.materials.Spouse.Get(ctx, 2025)
	if err != nil || spouse == nil || spouse.Name != "花子" {
		t.Fatalf("Spouse.Get: %+v err=%v", spouse, err)
	}
	if err := env.materials.Spouse.Delete(ctx, 2025); err != nil {
		t.Fatalf("Spouse.Delete: %v", err)
	}
	if apperrors.CodeOf(env.materials.Spouse.Set(ctx, &model.Spouse{Name: "x"})) != apperrors.CodeBadRequest {
		t.Error("不正な配偶者は BAD_REQUEST")
	}
}

func TestNewMaterialsMissingRepo(t *testing.T) {
	if _, err := NewMaterials(MaterialRepositories{}); apperrors.CodeOf(err) != apperrors.CodeInternal {
		t.Errorf("未注入は INTERNAL: %v", err)
	}

	// 各フィールドの欠落を検出できること
	sqlDB, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "v.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()
	full := func() MaterialRepositories {
		return MaterialRepositories{
			WithholdingSlips:     persistence.NewWithholdingSlipRepository(sqlDB),
			Dependents:           persistence.NewDependentRepository(sqlDB),
			FurusatoDonations:    persistence.NewFurusatoDonationRepository(sqlDB),
			Donations:            persistence.NewDonationRepository(sqlDB),
			MedicalExpenses:      persistence.NewMedicalExpenseRepository(sqlDB),
			SocialInsurances:     persistence.NewSocialInsuranceRepository(sqlDB),
			InsurancePolicies:    persistence.NewInsurancePolicyRepository(sqlDB),
			BusinessWithholdings: persistence.NewBusinessWithholdingRepository(sqlDB),
			LossCarryforwards:    persistence.NewLossCarryforwardRepository(sqlDB),
			HousingLoans:         persistence.NewHousingLoanRepository(sqlDB),
			FixedAssets:          persistence.NewFixedAssetRepository(sqlDB),
			OtherIncomes:         persistence.NewOtherIncomeRepository(sqlDB),
			Spouse:               persistence.NewSpouseRepository(sqlDB),
		}
	}
	if _, err := NewMaterials(full()); err != nil {
		t.Fatalf("全注入は成功すべき: %v", err)
	}
	mutations := []func(*MaterialRepositories){
		func(r *MaterialRepositories) { r.Dependents = nil },
		func(r *MaterialRepositories) { r.FurusatoDonations = nil },
		func(r *MaterialRepositories) { r.Donations = nil },
		func(r *MaterialRepositories) { r.MedicalExpenses = nil },
		func(r *MaterialRepositories) { r.SocialInsurances = nil },
		func(r *MaterialRepositories) { r.InsurancePolicies = nil },
		func(r *MaterialRepositories) { r.BusinessWithholdings = nil },
		func(r *MaterialRepositories) { r.LossCarryforwards = nil },
		func(r *MaterialRepositories) { r.HousingLoans = nil },
		func(r *MaterialRepositories) { r.FixedAssets = nil },
		func(r *MaterialRepositories) { r.OtherIncomes = nil },
		func(r *MaterialRepositories) { r.Spouse = nil },
	}
	for i, mutate := range mutations {
		repos := full()
		mutate(&repos)
		if _, err := NewMaterials(repos); err == nil {
			t.Errorf("mutation %d: 欠落を検出すべき", i)
		}
	}
}

func TestJournalUsecaseSearchAndOpeningBalances(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	if _, err := env.journals.Add(ctx, salesEntry(t, "2025-02-01", 110_000), false); err != nil {
		t.Fatalf("Add: %v", err)
	}
	entries, total, err := env.journals.Search(ctx, repository.JournalSearchQuery{FiscalYear: 2025})
	if err != nil || total != 1 || len(entries) != 1 {
		t.Errorf("Search: total=%d err=%v", total, err)
	}
	if _, _, err := env.journals.Search(ctx, repository.JournalSearchQuery{FiscalYear: 25}); err == nil {
		t.Error("不正年度の検索はエラー")
	}

	if err := env.reports.SetOpeningBalance(ctx, &model.OpeningBalance{
		FiscalYear: 2025, AccountCode: "1001", Amount: 100_000,
	}); err != nil {
		t.Fatalf("SetOpeningBalance: %v", err)
	}
	balances, err := env.reports.ListOpeningBalances(ctx, 2025)
	if err != nil || len(balances) != 1 {
		t.Fatalf("ListOpeningBalances: %d件 err=%v", len(balances), err)
	}
	if err := env.reports.DeleteOpeningBalance(ctx, balances[0].ID); err != nil {
		t.Fatalf("DeleteOpeningBalance: %v", err)
	}
}

// 源泉徴収票のみ登録した場合、保険料欄と年末調整済み住宅ローン控除が反映される。
func TestFilingUsecaseSlipFallbacks(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	_, err := env.materials.WithholdingSlips.Add(ctx, &model.WithholdingSlip{
		FiscalYear: 2025, PayerName: "勤務先", PaymentAmount: 5_000_000, WithheldTax: 100_000,
		SocialInsurance:         700_000,
		NationalPensionPremium:  200_000, // 「社会保険料等の金額」の内訳 → 加算しない
		LifeInsuranceGeneralNew: 100_000,
		EarthquakeInsurance:     60_000,
		HousingLoanDeduction:    100_000, // 年末調整済み
	})
	if err != nil {
		t.Fatalf("源泉徴収票: %v", err)
	}

	result, err := env.filing.CalculateIncomeTax(ctx, 2025, IncomeTaxOptions{})
	if err != nil {
		t.Fatalf("CalculateIncomeTax: %v", err)
	}

	var social, life, quake model.Money
	for _, item := range result.Deductions.IncomeDeductions {
		switch item.Type {
		case taxation.DeductionSocialInsurance:
			social = item.Amount
		case taxation.DeductionLifeInsurance:
			life = item.Amount
		case taxation.DeductionEarthquakeInsurance:
			quake = item.Amount
		}
	}
	// 国民年金の内訳は二重計上しない → 社保は 700,000 のまま
	if social != 700_000 {
		t.Errorf("社会保険料控除 = %d, want 700000 (内訳の二重計上なし)", social.Yen())
	}
	// 保険契約が未登録なら源泉徴収票の保険料欄をフォールバック
	if life != 40_000 || quake != 50_000 {
		t.Errorf("生保 = %d (want 40000), 地震 = %d (want 50000)", life.Yen(), quake.Yen())
	}
	// 年末調整済みの住宅ローン控除が転記される
	if result.HousingLoanCredit != 100_000 {
		t.Errorf("住宅ローン控除 = %d, want 100000", result.HousingLoanCredit.Yen())
	}
}

// 同一の損失発生年度が複数登録されている場合はエラー (二重控除防止)。
func TestFilingUsecaseDuplicateLossYear(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	for range 2 {
		if _, err := env.materials.LossCarryforwards.Add(ctx, &model.LossCarryforward{
			FiscalYear: 2025, LossYear: 2024, Amount: 100_000,
		}); err != nil {
			t.Fatalf("繰越損失: %v", err)
		}
	}
	_, err := env.filing.CalculateIncomeTax(ctx, 2025, IncomeTaxOptions{})
	if apperrors.CodeOf(err) != apperrors.CodeConflict {
		t.Errorf("重複損失年度は CONFLICT: %v", err)
	}
}

// バッチ内の類似 (同日同額) は force なしでブロック、force ありで警告。
func TestJournalUsecaseAddBatchSimilarInBatch(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	similar := []model.JournalEntry{
		*salesEntry(t, "2025-05-01", 110_000),
		{
			FiscalYear: 2025, Date: ucDate(t, "2025-05-01"), Description: "雑収入",
			Lines: []model.JournalLine{
				{Side: model.SideDebit, AccountCode: "1001", Amount: 110_000},
				{Side: model.SideCredit, AccountCode: "4110", Amount: 110_000},
			},
		},
	}
	_, _, err := env.journals.AddBatch(ctx, similar, false)
	if apperrors.CodeOf(err) != apperrors.CodeConflict {
		t.Errorf("バッチ内類似は force なしで CONFLICT: %v", err)
	}
	ids, warnings, err := env.journals.AddBatch(ctx, similar, true)
	if err != nil || len(ids) != 2 || len(warnings) != 1 {
		t.Errorf("force あり: ids=%v warnings=%d err=%v", ids, len(warnings), err)
	}
}

// 混用資産: 事業割合の適用前に全体の償却額を帳簿価額で打ち止める。
func TestFilingUsecaseDepreciationBookValueCap(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// 帳簿価額 10,000 (取得 1,000,000 − 累計 990,000)、事業割合 50%
	// 全体の年間償却 250,000 → 簿価 10,000 で打ち止め → ×50% = 5,000
	if _, err := env.materials.FixedAssets.Add(ctx, &model.FixedAsset{
		FiscalYear: 2025, Name: "旧PC", AcquisitionDate: ucDate(t, "2022-01-01"),
		AcquisitionCost: 1_000_000, UsefulLife: 4, Method: model.DepreciationStraightLine,
		BusinessUseRatioPct: 50, AccumulatedDepreciation: 990_000,
	}); err != nil {
		t.Fatalf("固定資産: %v", err)
	}
	dep, err := env.filing.CalculateDepreciation(ctx, 2025)
	if err != nil {
		t.Fatalf("CalculateDepreciation: %v", err)
	}
	if dep.Total != 5_000 {
		t.Errorf("Total = %d, want 5000", dep.Total.Yen())
	}
}

// 年度未設定の申告資料は保存できない。
func TestMaterialUsecaseRequiresYear(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	_, err := env.materials.Dependents.Add(ctx, &model.Dependent{
		Name: "子", BirthDate: ucDate(t, "2010-01-01"),
	})
	if apperrors.CodeOf(err) != apperrors.CodeBadRequest {
		t.Errorf("年度なしは BAD_REQUEST: %v", err)
	}
	// 年度外の寄附日も保存時に拒否
	_, err = env.materials.FurusatoDonations.Add(ctx, &model.FurusatoDonation{
		FiscalYear: 2025, Municipality: "○○市", Amount: 10_000, Date: ucDate(t, "2024-12-31"),
	})
	if apperrors.CodeOf(err) != apperrors.CodeBadRequest {
		t.Errorf("年度外の寄附日は BAD_REQUEST: %v", err)
	}
}

// 保険契約の全区分と地震・旧長期の集計経路。
func TestFilingUsecaseInsuranceAllKinds(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	kinds := []model.InsurancePolicyKind{
		model.PolicyLifeGeneralNew, model.PolicyLifeGeneralOld, model.PolicyLifeMedicalCare,
		model.PolicyLifeAnnuityNew, model.PolicyLifeAnnuityOld,
		model.PolicyEarthquake, model.PolicyOldLongTerm,
	}
	for _, k := range kinds {
		if _, err := env.materials.InsurancePolicies.Add(ctx, &model.InsurancePolicy{
			FiscalYear: 2025, Kind: k, CompanyName: "○○保険", Premium: 100_000,
		}); err != nil {
			t.Fatalf("保険 %s: %v", k, err)
		}
	}
	result, err := env.filing.CalculateIncomeTax(ctx, 2025, IncomeTaxOptions{})
	if err != nil {
		t.Fatalf("CalculateIncomeTax: %v", err)
	}
	// 生保: 3区分とも上限 → 12万、地震: 上限 5万
	var life, quake model.Money
	for _, item := range result.Deductions.IncomeDeductions {
		switch item.Type {
		case taxation.DeductionLifeInsurance:
			life = item.Amount
		case taxation.DeductionEarthquakeInsurance:
			quake = item.Amount
		}
	}
	if life != 120_000 || quake != 50_000 {
		t.Errorf("生保 = %d (want 120000), 地震 = %d (want 50000)", life.Yen(), quake.Yen())
	}
}

func TestJournalUsecaseAddBatch(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	entries := []model.JournalEntry{
		*salesEntry(t, "2025-02-01", 110_000),
		*salesEntry(t, "2025-02-02", 220_000),
	}
	ids, warnings, err := env.journals.AddBatch(ctx, entries, false)
	if err != nil || len(ids) != 2 || len(warnings) != 0 {
		t.Fatalf("AddBatch: ids=%v warnings=%v err=%v", ids, warnings, err)
	}

	// バッチ内の完全一致は常にブロック
	dup := []model.JournalEntry{
		*salesEntry(t, "2025-03-01", 330_000),
		*salesEntry(t, "2025-03-01", 330_000),
	}
	_, _, err = env.journals.AddBatch(ctx, dup, true)
	if apperrors.CodeOf(err) != apperrors.CodeConflict {
		t.Errorf("バッチ内重複は CONFLICT: %v", err)
	}

	// 不正な仕訳を含むバッチは BAD_REQUEST
	invalid := []model.JournalEntry{*salesEntry(t, "2025-04-01", 440_000)}
	invalid[0].Lines[0].Amount = 999 // 貸借不一致
	_, _, err = env.journals.AddBatch(ctx, invalid, false)
	if apperrors.CodeOf(err) != apperrors.CodeBadRequest {
		t.Errorf("不正バッチは BAD_REQUEST: %v", err)
	}
}

// 帳簿 + 申告資料からの所得税計算エンドツーエンド (手計算で検証済みの数値)。
func TestFilingUsecaseIncomeTaxEndToEnd(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// 帳簿: 売上 5,500,000 (税込10%)、通信費 550,000 (税込)
	if _, err := env.journals.Add(ctx, salesEntry(t, "2025-02-01", 5_500_000), false); err != nil {
		t.Fatalf("売上: %v", err)
	}
	expense := &model.JournalEntry{
		FiscalYear: 2025, Date: ucDate(t, "2025-03-01"), Description: "通信費",
		Lines: []model.JournalLine{
			{Side: model.SideDebit, AccountCode: "5140", Amount: 550_000, TaxCategory: model.LineTaxTaxable10},
			{Side: model.SideCredit, AccountCode: "1002", Amount: 550_000},
		},
	}
	if _, err := env.journals.Add(ctx, expense, false); err != nil {
		t.Fatalf("経費: %v", err)
	}

	// 申告資料
	mustAdd := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("材料登録: %v", err)
		}
	}
	_, err := env.materials.WithholdingSlips.Add(ctx, &model.WithholdingSlip{
		FiscalYear: 2025, PayerName: "副業先", PaymentAmount: 3_000_000,
		WithheldTax: 60_000, SocialInsurance: 450_000,
	})
	mustAdd(err)
	_, err = env.materials.SocialInsurances.Add(ctx, &model.SocialInsuranceItem{
		FiscalYear: 2025, Kind: model.SocialInsuranceNationalPension, Amount: 50_000,
	})
	mustAdd(err)
	_, err = env.materials.InsurancePolicies.Add(ctx, &model.InsurancePolicy{
		FiscalYear: 2025, Kind: model.PolicyLifeGeneralNew, CompanyName: "○○生命", Premium: 100_000,
	})
	mustAdd(err)
	_, err = env.materials.MedicalExpenses.Add(ctx, &model.MedicalExpense{
		FiscalYear: 2025, Date: ucDate(t, "2025-05-01"),
		PatientName: "本人", MedicalInstitution: "病院", Amount: 300_000,
	})
	mustAdd(err)
	_, err = env.materials.FurusatoDonations.Add(ctx, &model.FurusatoDonation{
		FiscalYear: 2025, Municipality: "○○市", Amount: 50_000, Date: ucDate(t, "2025-06-01"),
	})
	mustAdd(err)

	result, err := env.filing.CalculateIncomeTax(ctx, 2025, IncomeTaxOptions{
		BlueReturnDeduction: 650_000,
	})
	if err != nil {
		t.Fatalf("CalculateIncomeTax: %v", err)
	}

	// 事業所得 = (5,500,000 − 550,000) − 65万 = 4,300,000
	if result.BusinessIncome != 4_300_000 {
		t.Errorf("BusinessIncome = %d, want 4300000", result.BusinessIncome.Yen())
	}
	// 給与所得 = 300万 → 202万
	if result.SalaryIncome != 2_020_000 {
		t.Errorf("SalaryIncome = %d, want 2020000", result.SalaryIncome.Yen())
	}
	// 合計所得 6,320,000 → 控除: 基礎63万 + 社保50万 + 生保4万 + 医療費20万 + 寄附金4.8万 = 1,418,000
	if result.TotalIncomeDeductions != 1_418_000 {
		t.Errorf("TotalIncomeDeductions = %d, want 1418000", result.TotalIncomeDeductions.Yen())
	}
	// 課税所得 4,902,000 → 税額 552,900 → 復興税 11,610 → 564,510 − 源泉6万 → 504,500
	if result.TaxableIncome != 4_902_000 {
		t.Errorf("TaxableIncome = %d, want 4902000", result.TaxableIncome.Yen())
	}
	if result.TaxDue != 504_500 {
		t.Errorf("TaxDue = %d, want 504500", result.TaxDue.Yen())
	}

	// サニティチェックは通過
	sanity, err := env.filing.SanityCheck(ctx, 2025, IncomeTaxOptions{BlueReturnDeduction: 650_000})
	if err != nil || !sanity.Check.Passed {
		t.Errorf("SanityCheck: passed=%t err=%v items=%+v", sanity.Check.Passed, err, sanity.Check.Items)
	}
}

// SanityCheck が警告 (給与ありで源泉0) を usecase 経由で表面化させる。
func TestFilingUsecaseSanityCheckWarning(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	if _, err := env.materials.WithholdingSlips.Add(ctx, &model.WithholdingSlip{
		FiscalYear: 2025, PayerName: "勤務先", PaymentAmount: 3_000_000, WithheldTax: 0,
	}); err != nil {
		t.Fatalf("源泉徴収票: %v", err)
	}
	sanity, err := env.filing.SanityCheck(ctx, 2025, IncomeTaxOptions{})
	if err != nil {
		t.Fatalf("SanityCheck: %v", err)
	}
	found := false
	for _, item := range sanity.Check.Items {
		if item.Code == "NO_WITHHOLDING_ON_SALARY" {
			found = true
		}
	}
	if !found {
		t.Errorf("警告が表面化すべき: %+v", sanity.Check.Items)
	}
}

func TestFilingUsecaseConsumptionTax(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	if _, err := env.journals.Add(ctx, salesEntry(t, "2025-02-01", 5_500_000), false); err != nil {
		t.Fatalf("売上: %v", err)
	}

	outcome, err := env.filing.CalculateConsumptionTax(ctx, 2025, ConsumptionTaxOptions{
		Method: taxation.MethodSpecial20Pct,
	})
	if err != nil {
		t.Fatalf("CalculateConsumptionTax: %v", err)
	}
	if outcome.Aggregate.TaxableSales10 != 5_500_000 {
		t.Errorf("集計売上 = %d", outcome.Aggregate.TaxableSales10.Yen())
	}
	// 2割特例: 課税標準 5,000,000 → 国税 390,000 → 納付 78,000 + 地方 22,000 = 100,000
	if outcome.Result.TotalDue != 100_000 {
		t.Errorf("TotalDue = %d, want 100000", outcome.Result.TotalDue.Yen())
	}
}

func TestFilingUsecaseFurusatoAndDepreciation(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	if _, err := env.journals.Add(ctx, salesEntry(t, "2025-02-01", 5_500_000), false); err != nil {
		t.Fatalf("売上: %v", err)
	}
	for _, d := range []struct {
		city    string
		amount  model.Money
		oneStop bool
	}{{"A市", 30_000, true}, {"B市", 20_000, false}, {"A市", 10_000, false}} {
		if _, err := env.materials.FurusatoDonations.Add(ctx, &model.FurusatoDonation{
			FiscalYear: 2025, Municipality: d.city, Amount: d.amount,
			Date: ucDate(t, "2025-06-01"), OneStopApplied: d.oneStop,
		}); err != nil {
			t.Fatalf("寄附: %v", err)
		}
	}

	summary, err := env.filing.SummarizeFurusato(ctx, 2025, IncomeTaxOptions{BlueReturnDeduction: 650_000})
	if err != nil {
		t.Fatalf("SummarizeFurusato: %v", err)
	}
	if summary.TotalAmount != 60_000 || summary.DonationCount != 3 ||
		summary.MunicipalityCount != 2 || summary.OneStopCount != 1 {
		t.Errorf("集計が不一致: %+v", summary)
	}
	if summary.DeductionAmount != 58_000 {
		t.Errorf("DeductionAmount = %d, want 58000", summary.DeductionAmount.Yen())
	}
	if summary.EstimatedLimit <= 0 {
		t.Errorf("EstimatedLimit = %d, want > 0", summary.EstimatedLimit.Yen())
	}
	if !summary.NeedsTaxReturn {
		t.Error("事業所得者は確定申告が必要")
	}

	// 減価償却: 年央取得の定額法 (7月取得 → 6ヶ月分)、前年取得の定率法 (12ヶ月)
	if _, err := env.materials.FixedAssets.Add(ctx, &model.FixedAsset{
		FiscalYear: 2025, Name: "PC", AcquisitionDate: ucDate(t, "2025-07-01"),
		AcquisitionCost: 600_000, UsefulLife: 4, Method: model.DepreciationStraightLine,
		BusinessUseRatioPct: 100,
	}); err != nil {
		t.Fatalf("固定資産: %v", err)
	}
	if _, err := env.materials.FixedAssets.Add(ctx, &model.FixedAsset{
		FiscalYear: 2025, Name: "車両", AcquisitionDate: ucDate(t, "2024-01-15"),
		AcquisitionCost: 2_000_000, UsefulLife: 6, Method: model.DepreciationDecliningBalance,
		DecliningRatePerMille: 333, BusinessUseRatioPct: 50, AccumulatedDepreciation: 666_000,
	}); err != nil {
		t.Fatalf("固定資産2: %v", err)
	}
	dep, err := env.filing.CalculateDepreciation(ctx, 2025)
	if err != nil {
		t.Fatalf("CalculateDepreciation: %v", err)
	}
	if len(dep.Entries) != 2 {
		t.Fatalf("減価償却件数: %+v", dep)
	}
	// PC: 600,000/4 × 6/12 = 75,000
	// 車両: 帳簿価額 1,334,000 × 0.333 × 50% = 222,111
	if dep.Entries[0].Months != 6 || dep.Entries[0].CurrentYearAmount != 75_000 {
		t.Errorf("定額法: %+v", dep.Entries[0])
	}
	if dep.Entries[1].Months != 12 || dep.Entries[1].CurrentYearAmount != 222_111 {
		t.Errorf("定率法: %+v", dep.Entries[1])
	}
	if dep.Total != 297_111 {
		t.Errorf("Total = %d, want 297111", dep.Total.Yen())
	}
}

// 配偶者・扶養・寄附金・その他所得・事業源泉・繰越損失・住宅ローンが
// 所得税計算に反映されること (gather の全経路)。
func TestFilingUsecaseGatherAllMaterials(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	if _, err := env.journals.Add(ctx, salesEntry(t, "2025-02-01", 5_500_000), false); err != nil {
		t.Fatalf("売上: %v", err)
	}
	mustAdd := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("材料登録: %v", err)
		}
	}
	mustAdd(env.materials.Spouse.Set(ctx, &model.Spouse{
		FiscalYear: 2025, Name: "花子", BirthDate: ucDate(t, "1990-05-01"), Income: 0,
	}))
	_, err := env.materials.Dependents.Add(ctx, &model.Dependent{
		FiscalYear: 2025, Name: "子", Relationship: "子",
		BirthDate: ucDate(t, "2005-06-01"), Cohabiting: true,
	})
	mustAdd(err)
	_, err = env.materials.Donations.Add(ctx, &model.DonationRecord{
		FiscalYear: 2025, Kind: model.DonationPolitical, RecipientName: "政党X",
		Amount: 100_000, Date: ucDate(t, "2025-06-01"),
	})
	mustAdd(err)
	_, err = env.materials.OtherIncomes.Add(ctx, &model.OtherIncome{
		FiscalYear: 2025, Kind: model.OtherIncomeMiscellaneous,
		Description: "原稿料", Revenue: 200_000, Expenses: 50_000, WithheldTax: 15_315,
	})
	mustAdd(err)
	_, err = env.materials.BusinessWithholdings.Add(ctx, &model.BusinessWithholding{
		FiscalYear: 2025, ClientName: "取引先A", GrossAmount: 1_100_000, WithheldTax: 102_100,
	})
	mustAdd(err)
	_, err = env.materials.LossCarryforwards.Add(ctx, &model.LossCarryforward{
		FiscalYear: 2025, LossYear: 2024, Amount: 300_000, UsedAmount: 100_000,
	})
	mustAdd(err)
	_, err = env.materials.HousingLoans.Add(ctx, &model.HousingLoanDetail{
		FiscalYear: 2025, Kind: model.HousingUsed, Category: "general",
		MoveInDate: ucDate(t, "2025-04-01"), YearEndBalance: 10_000_000,
	})
	mustAdd(err)

	result, err := env.filing.CalculateIncomeTax(ctx, 2025, IncomeTaxOptions{
		BlueReturnDeduction: 650_000,
	})
	if err != nil {
		t.Fatalf("CalculateIncomeTax: %v", err)
	}

	// 反映の確認 (個別の値は taxation のテストで検証済みのため、経路の疎通を見る)
	if result.LossCarryforwardApplied != 200_000 {
		t.Errorf("繰越損失 = %d, want 200000 (残額)", result.LossCarryforwardApplied.Yen())
	}
	if result.HousingLoanCredit != 70_000 {
		t.Errorf("住宅ローン控除 = %d, want 70000", result.HousingLoanCredit.Yen())
	}
	if result.TotalWithheld != 102_100+15_315 {
		t.Errorf("源泉合計 = %d, want 117415", result.TotalWithheld.Yen())
	}
	hasSpouse, hasDependent := false, false
	for _, item := range result.Deductions.IncomeDeductions {
		switch item.Type {
		case taxation.DeductionSpouse:
			hasSpouse = true
		case taxation.DeductionDependent:
			hasDependent = true
		}
	}
	if !hasSpouse || !hasDependent {
		t.Errorf("配偶者控除=%t 扶養控除=%t (どちらも true のはず)", hasSpouse, hasDependent)
	}
	// 政治活動寄附金は税額控除が選択される (限界税率20%)
	hasPolitical := false
	for _, c := range result.Deductions.TaxCredits {
		if c.Type == taxation.CreditPoliticalDonation {
			hasPolitical = true
		}
	}
	if !hasPolitical {
		t.Error("政党等寄附金特別控除が反映されるべき")
	}
}
