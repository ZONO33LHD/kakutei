package taxation

import (
	"fmt"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/policy"
)

// IncomeTaxInput は所得税計算の入力。金額は全て円単位・非負 (別記ある場合を除く)。
type IncomeTaxInput struct {
	FiscalYear model.FiscalYear

	// 収入
	SalaryRevenue       model.Money // 給与収入 (支払金額)
	BusinessRevenue     model.Money // 事業収入
	BusinessExpenses    model.Money // 事業経費 (減価償却費等を含む)
	BlueReturnDeduction model.Money // 青色申告特別控除 (0/10万/55万/65万)
	MiscIncome          model.Money // 雑所得・年金以外 (収入−経費、計算済みの所得金額)
	PensionRevenue      model.Money // 公的年金等の収入金額 (控除計算はサービス内で行う)
	PensionIsOver65     bool        // 年度末時点で65歳以上か (税法上の年齢)
	DividendIncome      model.Money // 配当所得 (総合課税)
	OneTimeIncomeGross  model.Money // 一時所得 (支出控除後・特別控除前)

	// 所得控除の材料
	SocialInsurance             model.Money                 // 社会保険料 (全額控除)
	LifeInsurance               model.LifeInsurancePremiums // 生命保険料 (3区分・新旧)
	EarthquakeInsurancePremium  model.Money
	OldLongTermInsurancePremium model.Money
	MedicalExpenses             model.Money
	SelfMedicationExpenses      model.Money // セルフメディケーション対象OTC購入額
	SelfMedicationEligible      bool        // 特定健康診査等の受診要件を満たすか
	FurusatoDonation            model.Money // ふるさと納税の合計額
	Donations                   []model.DonationRecord
	Spouse                      *model.Spouse // nil = 配偶者なし
	Dependents                  []model.Dependent
	IdecoContribution           model.Money // iDeCo 掛金
	SmallBusinessMutualAid      model.Money // 小規模企業共済掛金
	DisabilityMutualAid         model.Money // 心身障害者扶養共済掛金
	WidowStatus                 model.WidowStatus
	SelfDisability              model.DisabilityKind
	IsWorkingStudent            bool

	// 税額控除の材料
	HousingLoans []model.HousingLoanDetail

	// YearEndAdjustedHousingLoanCredit は年末調整で適用済みの住宅借入金等特別控除額
	// (源泉徴収票の転記)。HousingLoans の明細がある場合は明細からの計算を優先し、
	// この値は無視する (二重計上防止)。
	YearEndAdjustedHousingLoanCredit model.Money

	// 繰越・源泉・予定納税
	LossCarryforward    model.Money // 純損失の繰越控除額
	SalaryWithheldTax   model.Money // 給与の源泉徴収税額
	BusinessWithheldTax model.Money // 事業所得の源泉徴収税額
	OtherWithheldTax    model.Money // その他所得の源泉徴収税額
	EstimatedTaxPayment model.Money // 予定納税額 (第1期+第2期)
}

// IncomeTaxResult は所得税計算の結果。
type IncomeTaxResult struct {
	FiscalYear model.FiscalYear

	// 所得
	SalaryIncome                 model.Money // 給与所得 (所得金額調整控除適用後)
	BusinessIncome               model.Money // 事業所得 (青色控除後、赤字は負)
	PensionIncome                model.Money // 雑所得 (公的年金等、控除後)
	AggregateIncome              model.Money // 合計所得金額 (損益通算後・繰越控除前)
	TotalIncome                  model.Money // 総所得金額等 (繰越控除後)
	EffectiveBlueReturnDeduction model.Money

	// 控除と課税所得
	TotalIncomeDeductions model.Money
	TaxableIncome         model.Money // 課税総所得金額 (1,000円未満切捨)

	// 税額
	IncomeTaxBase         model.Money // 算出税額 (速算表)
	DividendCredit        model.Money
	HousingLoanCredit     model.Money
	TotalTaxCredits       model.Money
	IncomeTaxAfterCredits model.Money // 税額控除後 (基準所得税額)
	ReconstructionTax     model.Money // 復興特別所得税 (2.1%、1円未満切捨)
	TotalTax              model.Money // 所得税及び復興特別所得税の額

	// 精算
	LossCarryforwardApplied model.Money
	TotalWithheld           model.Money // 源泉徴収 + 予定納税の合計
	TaxDue                  model.Money // 正: 納付 (100円未満切捨) / 負: 還付 (1円単位)

	Deductions *DeductionsResult // 控除内訳
	Warnings   []string          // 自動調整等の警告
}

// IncomeTaxService は所得税計算のドメインサービス。
type IncomeTaxService struct{}

func NewIncomeTaxService() *IncomeTaxService { return &IncomeTaxService{} }

// Calculate は所得税を計算する (令和7年分・令和8年分)。
//
// 手順:
//  1. 給与所得 (所得金額調整控除を含む)・事業所得 (青色控除の利益上限調整)・
//     その他所得を計算し損益通算 (一時所得は損失充当後に1/2)
//  2. 合計所得金額 (繰越控除前) を確定し、純損失の繰越控除で総所得金額等を確定
//  3. 所得控除を計算 (人的控除の所得判定は合計所得金額、寄附金・医療費の
//     所得基準は総所得金額等。寄附金は所得控除/税額控除の有利選択)
//  4. 課税所得 (1,000円未満切捨) → 速算表で算出税額
//  5. 税額控除 (住宅ローン・配当・寄附金特別控除) を適用
//  6. 復興特別所得税 (2.1%) を加算
//  7. 源泉徴収税額を控除して申告納税額を確定 (正なら100円未満切捨) し、
//     予定納税額を差し引く (還付は1円単位)
func (s *IncomeTaxService) Calculate(in IncomeTaxInput) (*IncomeTaxResult, error) {
	if err := s.validate(&in); err != nil {
		return nil, err
	}

	var warnings []string

	// Step 1: 各種所得
	salaryIncome := SalaryIncome(in.SalaryRevenue, in.FiscalYear)
	if adj := SalaryIncomeAdjustment(in.SalaryRevenue, in.SelfDisability, in.Spouse, in.Dependents, in.FiscalYear); adj > 0 {
		salaryIncome = (salaryIncome - adj).ClampNonNegative()
		warnings = append(warnings, fmt.Sprintf("所得金額調整控除 %d円 を給与所得に適用しました", adj.Yen()))
	}
	biz := BusinessIncome(in.BusinessRevenue, in.BusinessExpenses, in.BlueReturnDeduction)
	if biz.Capped {
		warnings = append(warnings, fmt.Sprintf(
			"青色申告特別控除を自動調整しました: %d円 → %d円（事業利益 %d円が上限）",
			in.BlueReturnDeduction.Yen(), biz.EffectiveBlueReturnDeduction.Yen(),
			(in.BusinessRevenue-in.BusinessExpenses).Yen()))
	}

	oneTimeBase := (in.OneTimeIncomeGross - policy.OneTimeIncomeSpecialDeduction).ClampNonNegative()

	// 公的年金等に係る雑所得。控除額は「公的年金等以外の合計所得金額」に依存する。
	var pensionIncome model.Money
	if in.PensionRevenue > 0 {
		otherIncome := (salaryIncome + biz.Income + in.MiscIncome + in.DividendIncome + oneTimeBase/2).
			ClampNonNegative()
		pension, perr := PensionDeduction(PensionDeductionInput{
			PensionRevenue: in.PensionRevenue,
			IsOver65:       in.PensionIsOver65,
			OtherIncome:    otherIncome,
		})
		if perr != nil {
			return nil, perr
		}
		pensionIncome = pension.TaxablePensionIncome
	}

	// 損益通算 (所得税法第69条): 経常所得グループの損失は
	// 一時所得 (特別控除後・1/2適用前) から控除し、その後に1/2を適用する。
	ordinary := salaryIncome + biz.Income + in.MiscIncome + pensionIncome + in.DividendIncome
	if ordinary < 0 {
		offset := (-ordinary).Min(oneTimeBase)
		oneTimeBase -= offset
		ordinary += offset
	}

	// Step 2: 合計所得金額 (繰越控除前) → 総所得金額等 (繰越控除後)
	aggregateIncome := (ordinary + oneTimeBase/2).ClampNonNegative()
	var lossApplied model.Money
	if in.LossCarryforward > 0 && aggregateIncome > 0 {
		lossApplied = in.LossCarryforward.Min(aggregateIncome)
	}
	totalIncome := aggregateIncome - lossApplied

	// Step 3-5: 控除計算と寄附金の有利選択 → 課税所得・算出税額・税額控除
	sel, err := s.selectBestDeductionPattern(&in, aggregateIncome, totalIncome)
	if err != nil {
		return nil, err
	}
	deductions := sel.deductions

	afterCredits := (sel.incomeTaxBase - deductions.TotalTaxCredits).ClampNonNegative()

	// Step 6: 復興特別所得税 (1円未満切捨は整数演算で自動的に満たす)
	reconstruction := afterCredits.MulDiv(policy.ReconstructionTaxRateNum, policy.ReconstructionTaxRateDenom)
	totalTax := afterCredits + reconstruction

	// Step 7: 精算
	// 申告納税額 = 税額 − 源泉徴収税額 (正なら100円未満切捨、国税通則法第119条)。
	// その後に予定納税額を差し引いて第3期分の税額を求める。還付は1円単位 (同第120条)。
	withheld := in.SalaryWithheldTax + in.BusinessWithheldTax + in.OtherWithheldTax
	afterWithheld := totalTax - withheld
	if afterWithheld > 0 {
		afterWithheld = afterWithheld.RoundDownTo(policy.TaxAmountRoundingUnit)
	}
	taxDue := afterWithheld - in.EstimatedTaxPayment

	return &IncomeTaxResult{
		FiscalYear:                   in.FiscalYear,
		SalaryIncome:                 salaryIncome,
		BusinessIncome:               biz.Income,
		PensionIncome:                pensionIncome,
		AggregateIncome:              aggregateIncome,
		TotalIncome:                  totalIncome,
		EffectiveBlueReturnDeduction: biz.EffectiveBlueReturnDeduction,
		TotalIncomeDeductions:        deductions.TotalIncomeDeductions,
		TaxableIncome:                sel.taxableIncome,
		IncomeTaxBase:                sel.incomeTaxBase,
		DividendCredit:               creditAmountOf(deductions.TaxCredits, CreditDividend),
		HousingLoanCredit:            creditAmountOf(deductions.TaxCredits, CreditHousingLoan),
		TotalTaxCredits:              deductions.TotalTaxCredits,
		IncomeTaxAfterCredits:        afterCredits,
		ReconstructionTax:            reconstruction,
		TotalTax:                     totalTax,
		LossCarryforwardApplied:      lossApplied,
		TotalWithheld:                withheld + in.EstimatedTaxPayment,
		TaxDue:                       taxDue,
		Deductions:                   deductions,
		Warnings:                     warnings,
	}, nil
}

// validate は入力全体の検証を行う。
func (s *IncomeTaxService) validate(in *IncomeTaxInput) error {
	if err := validateFiscalYear(in.FiscalYear); err != nil {
		return err
	}
	if err := ValidateBlueReturnDeduction(in.BlueReturnDeduction); err != nil {
		return err
	}
	nonNegatives := []struct {
		name   string
		amount model.Money
	}{
		{"給与収入", in.SalaryRevenue}, {"事業収入", in.BusinessRevenue}, {"事業経費", in.BusinessExpenses},
		{"雑所得", in.MiscIncome}, {"年金収入", in.PensionRevenue},
		{"配当所得", in.DividendIncome}, {"一時所得", in.OneTimeIncomeGross},
		{"社会保険料", in.SocialInsurance}, {"地震保険料", in.EarthquakeInsurancePremium},
		{"旧長期損害保険料", in.OldLongTermInsurancePremium}, {"医療費", in.MedicalExpenses},
		{"セルフメディケーション対象額", in.SelfMedicationExpenses}, {"ふるさと納税", in.FurusatoDonation},
		{"年末調整済み住宅ローン控除", in.YearEndAdjustedHousingLoanCredit},
		{"iDeCo掛金", in.IdecoContribution}, {"小規模企業共済掛金", in.SmallBusinessMutualAid},
		{"心身障害者扶養共済掛金", in.DisabilityMutualAid}, {"繰越損失", in.LossCarryforward},
		{"給与源泉徴収税額", in.SalaryWithheldTax}, {"事業源泉徴収税額", in.BusinessWithheldTax},
		{"その他源泉徴収税額", in.OtherWithheldTax}, {"予定納税額", in.EstimatedTaxPayment},
	}
	for _, v := range nonNegatives {
		if v.amount < 0 || !v.amount.ValidateAmountRange() {
			return apperrors.Newf(apperrors.CodeBadRequest, "%sの金額が不正です: %d", v.name, v.amount.Yen())
		}
	}
	if err := in.LifeInsurance.Validate(); err != nil {
		return err
	}
	if err := in.WidowStatus.Validate(); err != nil && in.WidowStatus != "" {
		return err
	}
	if in.SelfDisability != "" {
		if err := in.SelfDisability.Validate(); err != nil {
			return err
		}
	}
	yearEnd := in.FiscalYear.End()
	// checkYear は年度付きエンティティが入力年度と一致するかを検証する
	// (0 = 未設定は許容し、日付ベースの検証に委ねる)。
	checkYear := func(entityYear model.FiscalYear, label string) error {
		if entityYear != 0 && entityYear != in.FiscalYear {
			return apperrors.Newf(apperrors.CodeBadRequest,
				"%sの年度 %d が計算対象年度 %d と一致しません", label, int(entityYear), int(in.FiscalYear))
		}
		return nil
	}
	if in.Spouse != nil {
		if err := in.Spouse.Validate(); err != nil {
			return err
		}
		if err := checkYear(in.Spouse.FiscalYear, "配偶者情報"); err != nil {
			return err
		}
		if in.Spouse.BirthDate.After(yearEnd) {
			return apperrors.New(apperrors.CodeBadRequest, "配偶者の生年月日が課税年度末より後です")
		}
	}
	for i := range in.Dependents {
		if err := in.Dependents[i].Validate(); err != nil {
			return err
		}
		if err := checkYear(in.Dependents[i].FiscalYear, "扶養親族"); err != nil {
			return err
		}
		if in.Dependents[i].BirthDate.After(yearEnd) {
			return apperrors.Newf(apperrors.CodeBadRequest,
				"扶養親族 %s の生年月日が課税年度末より後です", in.Dependents[i].Name)
		}
	}
	for i := range in.Donations {
		if err := in.Donations[i].Validate(); err != nil {
			return err
		}
		if err := checkYear(in.Donations[i].FiscalYear, "寄附金"); err != nil {
			return err
		}
		if !in.FiscalYear.Contains(in.Donations[i].Date) {
			return apperrors.Newf(apperrors.CodeBadRequest,
				"寄附金 (%s) の寄附日 %s が課税年度外です", in.Donations[i].RecipientName, in.Donations[i].Date)
		}
	}
	for i := range in.HousingLoans {
		if err := in.HousingLoans[i].Validate(); err != nil {
			return err
		}
		if err := checkYear(in.HousingLoans[i].FiscalYear, "住宅ローン控除明細"); err != nil {
			return err
		}
		if in.HousingLoans[i].MoveInDate.After(yearEnd) {
			return apperrors.New(apperrors.CodeBadRequest, "住宅ローン控除の入居日が課税年度末より後です")
		}
	}
	return nil
}

func creditAmountOf(credits []DeductionItem, t DeductionType) model.Money {
	var total model.Money
	for _, c := range credits {
		if c.Type == t {
			total += c.Amount
		}
	}
	return total
}
