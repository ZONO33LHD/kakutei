package usecase

import (
	"context"
	"fmt"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/policy"
	"github.com/ZONO33LHD/kakutei/domain/repository"
	"github.com/ZONO33LHD/kakutei/domain/service/bookkeeping"
	"github.com/ZONO33LHD/kakutei/domain/service/taxation"
)

// IncomeTaxOptions は申告時にリクエストで指定する本人属性・当年オプション
// (永続化しない入力)。
type IncomeTaxOptions struct {
	BlueReturnDeduction    model.Money          // 青色申告特別控除 (0/10万/55万/65万)
	WidowStatus            model.WidowStatus    // 寡婦/ひとり親
	SelfDisability         model.DisabilityKind // 本人の障害区分
	IsWorkingStudent       bool
	SelfMedicationExpenses model.Money // セルフメディケーション対象OTC購入額
	SelfMedicationEligible bool
	IdecoContribution      model.Money
	SmallBusinessMutualAid model.Money
	DisabilityMutualAid    model.Money
	EstimatedTaxPayment    model.Money // 予定納税額 (第1期+第2期)
	PensionRevenue         model.Money // 公的年金等の収入金額
	PensionIsOver65        bool
}

// ConsumptionTaxOptions は消費税計算のオプション。
type ConsumptionTaxOptions struct {
	Method                 taxation.ConsumptionTaxMethod
	SimplifiedBusinessType policy.SimplifiedBusinessType // 簡易課税の事業区分
	InterimPayment         model.Money                   // 中間納付税額
}

// ConsumptionTaxOutcome は消費税計算の結果と帳簿集計の内訳。
type ConsumptionTaxOutcome struct {
	Aggregate *bookkeeping.ConsumptionAggregate // 帳簿からの集計 (警告フラグ含む)
	Result    *taxation.ConsumptionTaxResult
}

// SanityCheckOutcome は申告前チェックの結果。
type SanityCheckOutcome struct {
	IncomeTax *taxation.IncomeTaxResult
	Check     *taxation.SanityCheckResult
}

// FurusatoSummary はふるさと納税の集計。
type FurusatoSummary struct {
	FiscalYear        model.FiscalYear
	TotalAmount       model.Money
	DonationCount     int
	MunicipalityCount int
	OneStopCount      int
	// DeductionAmount は所得控除見込額 (合計 − 2,000円。40%上限は所得計算時に適用)
	DeductionAmount model.Money
	// EstimatedLimit は推定控除上限額 (所得税計算に基づく)
	EstimatedLimit model.Money
	// NeedsTaxReturn は確定申告が必要か (事業所得者は常に true、
	// ワンストップ特例は確定申告をすると無効になるため寄附分の申告が必要)
	NeedsTaxReturn bool
	Donations      []model.FurusatoDonation
}

// DepreciationEntry は減価償却計算の1資産分。
type DepreciationEntry struct {
	Asset             model.FixedAsset
	Months            int         // 当年の償却月数
	CurrentYearAmount model.Money // 当年の償却費
}

// DepreciationOutcome は年度の減価償却計算結果。
type DepreciationOutcome struct {
	FiscalYear model.FiscalYear
	Entries    []DepreciationEntry
	Total      model.Money
}

// FilingUsecase は確定申告計算のアプリケーションサービス。
// 帳簿・申告資料から計算材料を収集し、taxation ドメインサービスに委譲する。
type FilingUsecase interface {
	// CalculateIncomeTax は所得税を計算する。
	CalculateIncomeTax(ctx context.Context, year model.FiscalYear, opts IncomeTaxOptions) (*taxation.IncomeTaxResult, error)

	// CalculateConsumptionTax は帳簿の集計から消費税を計算する。
	CalculateConsumptionTax(ctx context.Context, year model.FiscalYear, opts ConsumptionTaxOptions) (*ConsumptionTaxOutcome, error)

	// SanityCheck は所得税計算と申告前サニティチェックを実行する。
	SanityCheck(ctx context.Context, year model.FiscalYear, opts IncomeTaxOptions) (*SanityCheckOutcome, error)

	// SummarizeFurusato はふるさと納税の集計と控除上限の推定を返す。
	SummarizeFurusato(ctx context.Context, year model.FiscalYear, opts IncomeTaxOptions) (*FurusatoSummary, error)

	// CalculateDepreciation は固定資産台帳から当年の減価償却費を計算する。
	// 計算結果は決算整理仕訳 (減価償却費) の起票材料であり、自動起票はしない。
	CalculateDepreciation(ctx context.Context, year model.FiscalYear) (*DepreciationOutcome, error)
}

type filingUsecase struct {
	journals    repository.JournalRepository
	accounts    repository.AccountRepository
	materials   *Materials
	statements  *bookkeeping.StatementService
	incomeTax   *taxation.IncomeTaxService
	consumption *taxation.ConsumptionTaxService
}

// NewFilingUsecase は FilingUsecase を生成する。
func NewFilingUsecase(
	journals repository.JournalRepository,
	accounts repository.AccountRepository,
	materials *Materials,
	statements *bookkeeping.StatementService,
	incomeTax *taxation.IncomeTaxService,
	consumption *taxation.ConsumptionTaxService,
) FilingUsecase {
	return &filingUsecase{
		journals: journals, accounts: accounts, materials: materials,
		statements: statements, incomeTax: incomeTax, consumption: consumption,
	}
}

// gatherIncomeTaxInput は帳簿・申告資料から所得税計算の入力を組み立てる。
//
// 保険料 (生命・地震・旧長期) は insurance_policies を唯一の情報源とする。
// 源泉徴収票の保険料欄は参考情報であり、二重計上を防ぐため合算しない。
func (u *filingUsecase) gatherIncomeTaxInput(
	ctx context.Context, year model.FiscalYear, opts IncomeTaxOptions,
) (*taxation.IncomeTaxInput, error) {
	if err := year.Validate(); err != nil {
		return nil, err
	}

	in := &taxation.IncomeTaxInput{
		FiscalYear:             year,
		BlueReturnDeduction:    opts.BlueReturnDeduction,
		WidowStatus:            opts.WidowStatus,
		SelfDisability:         opts.SelfDisability,
		IsWorkingStudent:       opts.IsWorkingStudent,
		SelfMedicationExpenses: opts.SelfMedicationExpenses,
		SelfMedicationEligible: opts.SelfMedicationEligible,
		IdecoContribution:      opts.IdecoContribution,
		SmallBusinessMutualAid: opts.SmallBusinessMutualAid,
		DisabilityMutualAid:    opts.DisabilityMutualAid,
		EstimatedTaxPayment:    opts.EstimatedTaxPayment,
		PensionRevenue:         opts.PensionRevenue,
		PensionIsOver65:        opts.PensionIsOver65,
	}
	if in.WidowStatus == "" {
		in.WidowStatus = model.WidowNone
	}

	if err := u.gatherSalaryAndSocial(ctx, year, in); err != nil {
		return nil, err
	}
	if err := u.gatherBusiness(ctx, year, in); err != nil {
		return nil, err
	}
	if err := u.gatherInsurance(ctx, year, in); err != nil {
		return nil, err
	}
	if err := u.gatherDeductionMaterials(ctx, year, in); err != nil {
		return nil, err
	}
	if err := u.gatherOtherIncome(ctx, year, in); err != nil {
		return nil, err
	}
	return in, nil
}

// gatherSalaryAndSocial は源泉徴収票と社会保険料内訳を集計する。
func (u *filingUsecase) gatherSalaryAndSocial(
	ctx context.Context, year model.FiscalYear, in *taxation.IncomeTaxInput,
) error {
	slips, err := u.materials.WithholdingSlips.List(ctx, year)
	if err != nil {
		return err
	}
	for i := range slips {
		in.SalaryRevenue += slips[i].PaymentAmount
		in.SalaryWithheldTax += slips[i].WithheldTax
		// 「社会保険料等の金額」欄は国民年金保険料等の申告分を含む合計額のため、
		// NationalPensionPremium (内訳表示) は加算しない (二重計上防止)。
		in.SocialInsurance += slips[i].SocialInsurance
		// 年末調整で適用済みの住宅借入金等特別控除 (明細がある場合は明細を優先)
		in.YearEndAdjustedHousingLoanCredit += slips[i].HousingLoanDeduction
	}
	items, err := u.materials.SocialInsurances.List(ctx, year)
	if err != nil {
		return err
	}
	for i := range items {
		in.SocialInsurance += items[i].Amount
	}
	return nil
}

// gatherBusiness は損益計算書から事業収入・経費を、取引先別源泉と繰越損失を集計する。
func (u *filingUsecase) gatherBusiness(
	ctx context.Context, year model.FiscalYear, in *taxation.IncomeTaxInput,
) error {
	accounts, err := u.accounts.FindAll(ctx)
	if err != nil {
		return err
	}
	entries, err := u.journals.ListByFiscalYear(ctx, year)
	if err != nil {
		return err
	}
	pl, err := u.statements.BuildProfitAndLoss(year, accounts, entries)
	if err != nil {
		return err
	}
	in.BusinessRevenue = pl.TotalRevenue
	in.BusinessExpenses = pl.TotalExpense

	withholdings, err := u.materials.BusinessWithholdings.List(ctx, year)
	if err != nil {
		return err
	}
	for i := range withholdings {
		in.BusinessWithheldTax += withholdings[i].WithheldTax
	}

	losses, err := u.materials.LossCarryforwards.List(ctx, year)
	if err != nil {
		return err
	}
	seenLossYears := map[model.FiscalYear]struct{}{}
	for i := range losses {
		// 同一の損失発生年度が複数登録されている場合は二重控除の疑いがあるため拒否する
		if _, dup := seenLossYears[losses[i].LossYear]; dup {
			return apperrors.Newf(apperrors.CodeConflict,
				"損失年度 %d の繰越損失が複数登録されています。重複を解消してください", int(losses[i].LossYear))
		}
		seenLossYears[losses[i].LossYear] = struct{}{}
		in.LossCarryforward += losses[i].Remaining()
	}
	return nil
}

// gatherInsurance は保険料 (生命3区分新旧・地震・旧長期) を集計する。
//
// 保険契約 (insurance_policies) を第一の情報源とし、1件も登録がない場合のみ
// 源泉徴収票の保険料欄 (年末調整の転記) をフォールバックとして使う。
// 両方を合算すると二重計上になるため、同時には使わない。
func (u *filingUsecase) gatherInsurance(
	ctx context.Context, year model.FiscalYear, in *taxation.IncomeTaxInput,
) error {
	policies, err := u.materials.InsurancePolicies.List(ctx, year)
	if err != nil {
		return err
	}
	if len(policies) == 0 {
		slips, err := u.materials.WithholdingSlips.List(ctx, year)
		if err != nil {
			return err
		}
		for i := range slips {
			s := &slips[i]
			in.LifeInsurance.GeneralNew += s.LifeInsuranceGeneralNew
			in.LifeInsurance.GeneralOld += s.LifeInsuranceGeneralOld
			in.LifeInsurance.MedicalCare += s.LifeInsuranceMedicalCare
			in.LifeInsurance.AnnuityNew += s.LifeInsuranceAnnuityNew
			in.LifeInsurance.AnnuityOld += s.LifeInsuranceAnnuityOld
			in.EarthquakeInsurancePremium += s.EarthquakeInsurance
			in.OldLongTermInsurancePremium += s.OldLongTermInsurance
		}
		return nil
	}
	for i := range policies {
		p := &policies[i]
		switch p.Kind {
		case model.PolicyLifeGeneralNew:
			in.LifeInsurance.GeneralNew += p.Premium
		case model.PolicyLifeGeneralOld:
			in.LifeInsurance.GeneralOld += p.Premium
		case model.PolicyLifeMedicalCare:
			in.LifeInsurance.MedicalCare += p.Premium
		case model.PolicyLifeAnnuityNew:
			in.LifeInsurance.AnnuityNew += p.Premium
		case model.PolicyLifeAnnuityOld:
			in.LifeInsurance.AnnuityOld += p.Premium
		case model.PolicyEarthquake:
			in.EarthquakeInsurancePremium += p.Premium
		case model.PolicyOldLongTerm:
			in.OldLongTermInsurancePremium += p.Premium
		}
	}
	return nil
}

// gatherDeductionMaterials は医療費・寄附金・配偶者・扶養・住宅ローンを集計する。
func (u *filingUsecase) gatherDeductionMaterials(
	ctx context.Context, year model.FiscalYear, in *taxation.IncomeTaxInput,
) error {
	medical, err := u.materials.MedicalExpenses.List(ctx, year)
	if err != nil {
		return err
	}
	for i := range medical {
		in.MedicalExpenses += medical[i].NetAmount()
	}

	furusato, err := u.materials.FurusatoDonations.List(ctx, year)
	if err != nil {
		return err
	}
	for i := range furusato {
		in.FurusatoDonation += furusato[i].Amount
	}

	donations, err := u.materials.Donations.List(ctx, year)
	if err != nil {
		return err
	}
	in.Donations = donations

	spouse, err := u.materials.Spouse.Get(ctx, year)
	if err != nil {
		return err
	}
	in.Spouse = spouse

	dependents, err := u.materials.Dependents.List(ctx, year)
	if err != nil {
		return err
	}
	in.Dependents = dependents

	loans, err := u.materials.HousingLoans.List(ctx, year)
	if err != nil {
		return err
	}
	in.HousingLoans = loans
	return nil
}

// gatherOtherIncome はその他所得 (雑/配当/一時) を集計する。
func (u *filingUsecase) gatherOtherIncome(
	ctx context.Context, year model.FiscalYear, in *taxation.IncomeTaxInput,
) error {
	incomes, err := u.materials.OtherIncomes.List(ctx, year)
	if err != nil {
		return err
	}
	for i := range incomes {
		o := &incomes[i]
		switch o.Kind {
		case model.OtherIncomeMiscellaneous:
			in.MiscIncome += o.NetIncome()
		case model.OtherIncomeDividend:
			in.DividendIncome += o.NetIncome()
		case model.OtherIncomeOneTime:
			in.OneTimeIncomeGross += o.NetIncome()
		}
		in.OtherWithheldTax += o.WithheldTax
	}
	return nil
}

func (u *filingUsecase) CalculateIncomeTax(
	ctx context.Context, year model.FiscalYear, opts IncomeTaxOptions,
) (*taxation.IncomeTaxResult, error) {
	in, err := u.gatherIncomeTaxInput(ctx, year, opts)
	if err != nil {
		return nil, err
	}
	return u.incomeTax.Calculate(*in)
}

func (u *filingUsecase) CalculateConsumptionTax(
	ctx context.Context, year model.FiscalYear, opts ConsumptionTaxOptions,
) (*ConsumptionTaxOutcome, error) {
	if err := year.Validate(); err != nil {
		return nil, err
	}
	accounts, err := u.accounts.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := u.journals.ListByFiscalYear(ctx, year)
	if err != nil {
		return nil, err
	}
	agg, err := u.statements.AggregateConsumption(year, accounts, entries)
	if err != nil {
		return nil, err
	}
	result, err := u.consumption.Calculate(taxation.ConsumptionTaxInput{
		FiscalYear:             year,
		Method:                 opts.Method,
		TaxableSales10:         agg.TaxableSales10,
		TaxableSales8:          agg.TaxableSales8,
		TaxablePurchases10:     agg.TaxablePurchases10,
		TaxablePurchases8:      agg.TaxablePurchases8,
		NonTaxableSales:        agg.NonTaxableSales,
		ExemptSales:            agg.ExemptSales,
		SimplifiedBusinessType: opts.SimplifiedBusinessType,
		InterimPayment:         opts.InterimPayment,
	})
	if err != nil {
		return nil, err
	}
	return &ConsumptionTaxOutcome{Aggregate: agg, Result: result}, nil
}

func (u *filingUsecase) SanityCheck(
	ctx context.Context, year model.FiscalYear, opts IncomeTaxOptions,
) (*SanityCheckOutcome, error) {
	in, err := u.gatherIncomeTaxInput(ctx, year, opts)
	if err != nil {
		return nil, err
	}
	result, err := u.incomeTax.Calculate(*in)
	if err != nil {
		return nil, err
	}
	return &SanityCheckOutcome{
		IncomeTax: result,
		Check:     taxation.SanityCheckIncomeTax(in, result),
	}, nil
}

func (u *filingUsecase) SummarizeFurusato(
	ctx context.Context, year model.FiscalYear, opts IncomeTaxOptions,
) (*FurusatoSummary, error) {
	if err := year.Validate(); err != nil {
		return nil, err
	}
	donations, err := u.materials.FurusatoDonations.List(ctx, year)
	if err != nil {
		return nil, err
	}

	summary := &FurusatoSummary{
		FiscalYear:     year,
		DonationCount:  len(donations),
		NeedsTaxReturn: true, // 事業所得者は常に確定申告が必要 (ワンストップ特例は無効になる)
		Donations:      donations,
	}
	municipalities := map[string]struct{}{}
	for i := range donations {
		summary.TotalAmount += donations[i].Amount
		municipalities[donations[i].Municipality] = struct{}{}
		if donations[i].OneStopApplied {
			summary.OneStopCount++
		}
	}
	summary.MunicipalityCount = len(municipalities)
	summary.DeductionAmount = (summary.TotalAmount - policy.FurusatoSelfBurden).ClampNonNegative()

	// 控除上限の推定 (所得税計算に基づく)
	result, err := u.CalculateIncomeTax(ctx, year, opts)
	if err != nil {
		return nil, err
	}
	summary.EstimatedLimit = taxation.EstimateFurusatoLimit(
		result.TotalIncome, result.TotalIncomeDeductions, 0)
	return summary, nil
}

func (u *filingUsecase) CalculateDepreciation(
	ctx context.Context, year model.FiscalYear,
) (*DepreciationOutcome, error) {
	if err := year.Validate(); err != nil {
		return nil, err
	}
	assets, err := u.materials.FixedAssets.List(ctx, year)
	if err != nil {
		return nil, err
	}
	outcome := &DepreciationOutcome{FiscalYear: year}
	for i := range assets {
		asset := &assets[i]
		if err := asset.Validate(); err != nil {
			return nil, apperrors.Wrap(err, apperrors.CodeOf(err),
				fmt.Sprintf("固定資産 %q が不正です", asset.Name))
		}
		months := depreciationMonths(asset.AcquisitionDate, year)
		if months == 0 {
			continue
		}
		amount, err := u.assetDepreciation(asset, months)
		if err != nil {
			return nil, err
		}
		outcome.Entries = append(outcome.Entries, DepreciationEntry{
			Asset: *asset, Months: months, CurrentYearAmount: amount,
		})
		outcome.Total += amount
	}
	return outcome, nil
}

// assetDepreciation は1資産の当年償却費 (事業専用割合適用後の必要経費額) を計算する。
//
// 事業割合の適用前に「全体の償却額」を帳簿価額で打ち止めてから割合を掛ける
// (割合適用後の金額を全体簿価と比較すると混用資産で過大計上になるため)。
func (u *filingUsecase) assetDepreciation(asset *model.FixedAsset, months int) (model.Money, error) {
	var full model.Money
	var err error
	switch asset.Method {
	case model.DepreciationDecliningBalance:
		full, err = taxation.DecliningBalanceDepreciation(
			asset.BookValue(), asset.DecliningRatePerMille, 100, months)
	case model.DepreciationStraightLine:
		full, err = taxation.StraightLineDepreciation(
			asset.AcquisitionCost, asset.UsefulLife, 100, months)
	default:
		return 0, apperrors.Newf(apperrors.CodeBadRequest, "未知の償却方法です: %q", asset.Method)
	}
	if err != nil {
		return 0, err
	}
	full = full.Min(asset.BookValue()) // 全体の償却は未償却残高を超えない
	return full.MulDiv(int64(asset.BusinessUseRatioPct), 100), nil
}

// depreciationMonths は当年の償却月数を返す。
// 年度内に取得した資産は取得月から12月まで、それ以前の取得は12ヶ月。
// 年度より後の取得日は 0 (対象外)。
func depreciationMonths(acquired model.Date, year model.FiscalYear) int {
	switch {
	case acquired.Year() < int(year):
		return policy.MonthsPerYear
	case acquired.Year() == int(year):
		return policy.MonthsPerYear - int(acquired.Month()) + 1
	default:
		return 0
	}
}
