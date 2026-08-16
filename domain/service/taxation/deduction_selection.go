package taxation

import (
	"fmt"
	"strings"

	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/policy"
)

// incomeTaxFromTable は課税総所得金額に速算表を適用して算出税額を返す。
func incomeTaxFromTable(taxable model.Money) model.Money {
	if taxable <= 0 {
		return 0
	}
	for _, b := range policy.IncomeTaxTable {
		if taxable.Yen() <= b.Threshold {
			return taxable.MulDiv(b.RatePct, 100) - model.Money(b.Deduction)
		}
	}
	return taxable.MulDiv(policy.IncomeTaxTopRatePct, 100) - policy.IncomeTaxTopDeduction
}

// buildIncomeDeductionItems は寄附金控除を除く全ての所得控除項目を組み立てる。
//
// 所得判定の基準:
//   - 人的控除 (基礎・配偶者・寡婦・勤労学生等) → 合計所得金額 (繰越控除前)
//   - 医療費控除の5%足切り → 総所得金額等 (繰越控除後)
func buildIncomeDeductionItems(in *IncomeTaxInput, aggregateIncome, totalIncome model.Money) []DeductionItem {
	var items []DeductionItem
	add := func(t DeductionType, name string, amount model.Money, details string) {
		if amount > 0 {
			items = append(items, DeductionItem{Type: t, Name: name, Amount: amount, Details: details})
		}
	}

	add(DeductionBasic, "基礎控除", BasicDeduction(aggregateIncome, in.FiscalYear), "")
	add(DeductionSocialInsurance, "社会保険料控除", in.SocialInsurance, "")
	add(DeductionLifeInsurance, "生命保険料控除",
		LifeInsuranceDeduction(in.LifeInsurance, in.FiscalYear, hasYoungDependent(in.Dependents, in.FiscalYear)), "")
	add(DeductionEarthquakeInsurance, "地震保険料控除",
		EarthquakeInsuranceDeduction(in.EarthquakeInsurancePremium, in.OldLongTermInsurancePremium), "")

	mutualAid := in.IdecoContribution + in.SmallBusinessMutualAid + in.DisabilityMutualAid
	add(DeductionMutualAid, "小規模企業共済等掛金控除", mutualAid, mutualAidDetails(in))

	items = append(items, medicalOrSelfMedicationItems(in, totalIncome)...)

	add(DeductionSpouse, "配偶者控除", SpouseDeduction(in.Spouse, aggregateIncome, in.FiscalYear), "")
	if item, ok := spouseDisabilityItem(in.Spouse, in.FiscalYear); ok {
		items = append(items, item)
	}
	items = append(items, DependentDeductions(in.Dependents, in.FiscalYear)...)

	if w := WidowDeduction(in.WidowStatus, aggregateIncome, in.FiscalYear); w > 0 {
		name := "寡婦控除"
		if in.WidowStatus == model.WidowSingleParent {
			name = "ひとり親控除"
		}
		add(DeductionWidow, name, w, "")
	}
	add(DeductionDisabilitySelf, "障害者控除（本人）", SelfDisabilityDeduction(in.SelfDisability), "")
	add(DeductionWorkingStudent, "勤労学生控除",
		WorkingStudentDeduction(in.IsWorkingStudent, aggregateIncome, in.FiscalYear), "")

	return items
}

// hasYoungDependent は23歳未満の扶養親族 (所得要件を満たす者) がいるかを返す
// (生命保険料控除の子育て世帯特例の判定)。
// 所得金額調整控除と同様、扶養控除と異なり同じ親族について夫婦双方が
// 適用できるため OtherTaxpayerDependent では除外しない。
func hasYoungDependent(dependents []model.Dependent, year model.FiscalYear) bool {
	yearEnd := year.End()
	incomeMax := policy.DependentIncomeMaxFor(int(year))
	for i := range dependents {
		dep := &dependents[i]
		if dep.Income.Yen() > incomeMax {
			continue
		}
		if dep.BirthDate.TaxAgeAt(yearEnd) < policy.SalaryAdjustmentChildAge {
			return true
		}
	}
	return false
}

// mutualAidDetails は小規模企業共済等掛金控除の内訳表示を返す。
func mutualAidDetails(in *IncomeTaxInput) string {
	var parts []string
	if in.IdecoContribution > 0 {
		parts = append(parts, fmt.Sprintf("iDeCo: %d", in.IdecoContribution.Yen()))
	}
	if in.SmallBusinessMutualAid > 0 {
		parts = append(parts, fmt.Sprintf("小規模企業共済: %d", in.SmallBusinessMutualAid.Yen()))
	}
	if in.DisabilityMutualAid > 0 {
		parts = append(parts, fmt.Sprintf("心身障害者扶養共済: %d", in.DisabilityMutualAid.Yen()))
	}
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts, ", ")
}

// medicalOrSelfMedicationItems は医療費控除とセルフメディケーション税制の
// 有利な方を選択して返す (併用不可の選択適用)。
func medicalOrSelfMedicationItems(in *IncomeTaxInput, totalIncome model.Money) []DeductionItem {
	medical := MedicalDeduction(in.MedicalExpenses, totalIncome)
	var selfMed model.Money
	if in.SelfMedicationEligible {
		selfMed = SelfMedicationDeduction(in.SelfMedicationExpenses)
	}
	switch {
	case selfMed > medical:
		return []DeductionItem{{Type: DeductionSelfMedication, Name: "セルフメディケーション税制", Amount: selfMed}}
	case medical > 0:
		return []DeductionItem{{Type: DeductionMedical, Name: "医療費控除", Amount: medical}}
	default:
		return nil
	}
}

// donationBreakdown は寄附金を選択適用グループ別に集計したもの。
// 政治活動 (税額控除30%)、認定NPO (40%)、公益社団法人等 (40%) はそれぞれ
// 独立に所得控除との選択ができる。
type donationBreakdown struct {
	political      model.Money
	npo            model.Money
	publicInterest model.Money
	nonSelectable  model.Money // 特定公益増進法人・その他 (所得控除のみ)
}

func breakdownDonations(donations []model.DonationRecord) donationBreakdown {
	var b donationBreakdown
	for i := range donations {
		d := &donations[i]
		switch d.Kind {
		case model.DonationPolitical:
			b.political += d.Amount
		case model.DonationNPO:
			b.npo += d.Amount
		case model.DonationPublicInterest:
			b.publicInterest += d.Amount
		default:
			b.nonSelectable += d.Amount
		}
	}
	return b
}

// patternResult は寄附金の選択適用1パターンの計算結果。
type patternResult struct {
	deductions    *DeductionsResult
	taxableIncome model.Money
	incomeTaxBase model.Money
	netTax        model.Money
}

// selectBestDeductionPattern は寄附金の所得控除/税額控除の選択適用を行い、
// 最終税額 (配当控除まで含む) が最小になるパターンの控除結果を返す。
//
// 政治活動寄附金 (租特法41条の18)・認定NPO (41条の18の2)・公益社団法人等
// (41条の18の3) はそれぞれ独立に選択できるため、最大8パターンを比較する。
func (s *IncomeTaxService) selectBestDeductionPattern(
	in *IncomeTaxInput, aggregateIncome, totalIncome model.Money,
) (*patternResult, error) {
	baseItems := buildIncomeDeductionItems(in, aggregateIncome, totalIncome)
	housingCredit, err := TotalHousingLoanCredit(in.HousingLoans, aggregateIncome)
	if err != nil {
		return nil, err
	}
	// 明細がない場合は年末調整済みの控除額 (源泉徴収票の転記) を使う。
	// 合計所得金額 2,000万円以下の要件は同様に適用する。
	if len(in.HousingLoans) == 0 && in.YearEndAdjustedHousingLoanCredit > 0 &&
		aggregateIncome <= policy.HousingLoanIncomeLimit {
		housingCredit = in.YearEndAdjustedHousingLoanCredit
	}
	donations := breakdownDonations(in.Donations)

	choices := func(amount model.Money) []bool {
		if amount > 0 {
			return []bool{false, true} // false = 所得控除, true = 税額控除
		}
		return []bool{false}
	}

	var best *patternResult
	selectable := false
	for _, pCredit := range choices(donations.political) {
		for _, nCredit := range choices(donations.npo) {
			for _, iCredit := range choices(donations.publicInterest) {
				r := s.evaluatePattern(in, totalIncome, baseItems, housingCredit, donations,
					donationChoice{pCredit, nCredit, iCredit})
				if best == nil || r.netTax < best.netTax {
					best = r
				}
			}
		}
	}
	selectable = donations.political > 0 || donations.npo > 0 || donations.publicInterest > 0

	if selectable {
		best.deductions.Notes = append(best.deductions.Notes,
			"寄附金控除: 所得控除と税額控除は選択適用です（併用不可）。最終税額が最小になる組み合わせを自動選択しました。")
	}
	return best, nil
}

// donationChoice は選択適用の1パターン (各グループで税額控除を選ぶか)。
type donationChoice struct {
	politicalCredit      bool
	npoCredit            bool
	publicInterestCredit bool
}

// donationFrame は特定寄附金に共通の「総所得金額等の40%」枠と
// 「2,000円の自己負担」を、申告書の計算順 (所得控除 → 政党等 → 認定NPO →
// 公益社団等) で配分するための状態。
type donationFrame struct {
	remaining  model.Money // 40%枠の残り
	burdenLeft model.Money // 未消化の自己負担額 (2,000円)
}

// take は枠から amount を上限まで確保し、自己負担控除後の基礎額を返す。
func (f *donationFrame) take(amount model.Money) model.Money {
	capped := amount.Min(f.remaining)
	f.remaining -= capped
	burden := f.burdenLeft.Min(capped)
	f.burdenLeft -= burden
	return capped - burden
}

// evaluatePattern は選択適用の1パターンを評価する。
func (s *IncomeTaxService) evaluatePattern(
	in *IncomeTaxInput, totalIncome model.Money, baseItems []DeductionItem,
	housingCredit model.Money, donations donationBreakdown, choice donationChoice,
) *patternResult {
	frame := &donationFrame{
		remaining:  totalIncome.MulDiv(policy.DonationIncomeDeductionRatioPct, 100),
		burdenLeft: policy.DonationSelfBurden,
	}

	// 所得控除に含める寄附金の合算 (税額控除を選んだグループは除外)
	donationForIncome := in.FurusatoDonation + donations.nonSelectable
	if !choice.politicalCredit {
		donationForIncome += donations.political
	}
	if !choice.npoCredit {
		donationForIncome += donations.npo
	}
	if !choice.publicInterestCredit {
		donationForIncome += donations.publicInterest
	}

	incomeItems := make([]DeductionItem, len(baseItems), len(baseItems)+1)
	copy(incomeItems, baseItems)
	if d := frame.take(donationForIncome); d > 0 {
		incomeItems = append(incomeItems, DeductionItem{
			Type: DeductionDonation, Name: "寄附金控除", Amount: d,
			Details: fmt.Sprintf("対象寄附金合計: %d", donationForIncome.Yen()),
		})
	}
	totalIncomeDeductions := sumItems(incomeItems)

	taxable := (totalIncome - totalIncomeDeductions).ClampNonNegative().
		RoundDownTo(policy.TaxableIncomeRoundingUnit)
	taxBase := incomeTaxFromTable(taxable)

	credits := s.buildTaxCredits(in, taxable, taxBase, housingCredit, donations, choice, frame)

	deductions := &DeductionsResult{
		IncomeDeductions:      incomeItems,
		TaxCredits:            credits,
		TotalIncomeDeductions: totalIncomeDeductions,
		TotalTaxCredits:       sumItems(credits),
	}
	return &patternResult{
		deductions:    deductions,
		taxableIncome: taxable,
		incomeTaxBase: taxBase,
		netTax:        (taxBase - deductions.TotalTaxCredits).ClampNonNegative(),
	}
}

// buildTaxCredits は税額控除 (住宅ローン・配当・寄附金特別控除) を組み立てる。
//
// 寄附金特別控除の上限: 政党等は所得税額の25%、認定NPOと公益社団等は
// 合わせて所得税額の25% (それぞれ別枠)。最終額は100円未満切捨て。
func (s *IncomeTaxService) buildTaxCredits(
	in *IncomeTaxInput, taxable, taxBase, housingCredit model.Money,
	donations donationBreakdown, choice donationChoice, frame *donationFrame,
) []DeductionItem {
	var credits []DeductionItem
	if housingCredit > 0 {
		credits = append(credits, DeductionItem{Type: CreditHousingLoan, Name: "住宅ローン控除", Amount: housingCredit})
	}

	cap25 := taxBase.MulDiv(policy.PoliticalDonationCreditCapPct, 100)
	if choice.politicalCredit {
		base := frame.take(donations.political)
		credit := base.MulDiv(policy.PoliticalDonationCreditRatePct, 100).
			Min(cap25).RoundDownTo(policy.TaxAmountRoundingUnit)
		if credit > 0 {
			credits = append(credits, DeductionItem{
				Type: CreditPoliticalDonation, Name: "政党等寄附金特別控除", Amount: credit,
			})
		}
	}

	// 認定NPO・公益社団等は共通の25%枠
	npoSharedCap := taxBase.MulDiv(policy.NPODonationCreditCapPct, 100)
	if choice.npoCredit {
		base := frame.take(donations.npo)
		credit := base.MulDiv(policy.NPODonationCreditRatePct, 100).
			Min(npoSharedCap).RoundDownTo(policy.TaxAmountRoundingUnit)
		if credit > 0 {
			credits = append(credits, DeductionItem{
				Type: CreditNPODonation, Name: "認定NPO法人等寄附金特別控除", Amount: credit,
			})
			npoSharedCap -= credit
		}
	}
	if choice.publicInterestCredit {
		base := frame.take(donations.publicInterest)
		credit := base.MulDiv(policy.NPODonationCreditRatePct, 100).
			Min(npoSharedCap).RoundDownTo(policy.TaxAmountRoundingUnit)
		if credit > 0 {
			credits = append(credits, DeductionItem{
				Type: CreditPublicInterestDonation, Name: "公益社団法人等寄附金特別控除", Amount: credit,
			})
		}
	}

	// 配当控除 (パターンごとの課税所得に依存するためここで計算し、選択比較に含める)
	if in.DividendIncome > 0 {
		if credit := DividendCredit(in.DividendIncome, taxable); credit > 0 {
			credits = append(credits, DeductionItem{Type: CreditDividend, Name: "配当控除", Amount: credit})
		}
	}
	return credits
}
