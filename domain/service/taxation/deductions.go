package taxation

import (
	"fmt"

	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/policy"
)

// DeductionType は控除項目の種別識別子。
type DeductionType string

const (
	DeductionBasic               DeductionType = "basic"                     // 基礎控除
	DeductionSocialInsurance     DeductionType = "social_insurance"          // 社会保険料控除
	DeductionLifeInsurance       DeductionType = "life_insurance"            // 生命保険料控除
	DeductionEarthquakeInsurance DeductionType = "earthquake_insurance"      // 地震保険料控除
	DeductionMutualAid           DeductionType = "small_business_mutual_aid" // 小規模企業共済等掛金控除
	DeductionMedical             DeductionType = "medical"                   // 医療費控除
	DeductionSelfMedication      DeductionType = "self_medication"           // セルフメディケーション税制
	DeductionDonation            DeductionType = "donation"                  // 寄附金控除 (所得控除)
	DeductionSpouse              DeductionType = "spouse"                    // 配偶者(特別)控除
	DeductionDependent           DeductionType = "dependent"                 // 扶養控除
	DeductionSpecificRelative    DeductionType = "specific_relative_special" // 特定親族特別控除
	DeductionDisability          DeductionType = "disability"                // 障害者控除 (扶養親族・配偶者)
	DeductionDisabilitySelf      DeductionType = "disability_self"           // 障害者控除 (本人)
	DeductionWidow               DeductionType = "widow"                     // 寡婦/ひとり親控除
	DeductionWorkingStudent      DeductionType = "working_student"           // 勤労学生控除
	CreditHousingLoan            DeductionType = "housing_loan"              // 住宅ローン控除 (税額控除)
	CreditDividend               DeductionType = "dividend"                  // 配当控除 (税額控除)
	CreditPoliticalDonation      DeductionType = "political_donation"        // 政党等寄附金特別控除
	CreditNPODonation            DeductionType = "npo_donation"              // 認定NPO法人等寄附金特別控除
	CreditPublicInterestDonation DeductionType = "public_interest_donation"  // 公益社団法人等寄附金特別控除
)

// DeductionItem は控除1項目の計算結果。
type DeductionItem struct {
	Type    DeductionType
	Name    string
	Amount  model.Money
	Details string // 補足 (対象者名等)。空 = なし
}

// DeductionsResult は所得控除・税額控除の集計結果。
type DeductionsResult struct {
	IncomeDeductions      []DeductionItem // 所得控除
	TaxCredits            []DeductionItem // 税額控除
	TotalIncomeDeductions model.Money
	TotalTaxCredits       model.Money
	Notes                 []string // 選択適用等の注意事項
}

func sumItems(items []DeductionItem) model.Money {
	var total model.Money
	for _, it := range items {
		total += it.Amount
	}
	return total
}

// BasicDeduction は基礎控除を計算する (令和7年分、所得税法第86条)。
func BasicDeduction(totalIncome model.Money) model.Money {
	return model.Money(policy.LookupBracket(policy.BasicDeductionTable, totalIncome.Yen(), 0))
}

// lifeInsuranceNew は新制度の生命保険料控除 (1区分、上限4万)。
// 所得税法第76条: 割り算で生じる1円未満の端数は切り上げ。
func lifeInsuranceNew(premium model.Money) model.Money {
	switch {
	case premium <= 0:
		return 0
	case premium <= policy.LifeInsuranceNewBracket1:
		return premium
	case premium <= policy.LifeInsuranceNewBracket2:
		return ceilDiv(premium, 2) + 10_000
	case premium <= policy.LifeInsuranceNewBracket3:
		return ceilDiv(premium, 4) + 20_000
	default:
		return policy.LifeInsuranceNewMax
	}
}

// lifeInsuranceOld は旧制度の生命保険料控除 (1区分、上限5万)。
func lifeInsuranceOld(premium model.Money) model.Money {
	switch {
	case premium <= 0:
		return 0
	case premium <= policy.LifeInsuranceOldBracket1:
		return premium
	case premium <= policy.LifeInsuranceOldBracket2:
		return ceilDiv(premium, 2) + 12_500
	case premium <= policy.LifeInsuranceOldBracket3:
		return ceilDiv(premium, 4) + 25_000
	default:
		return policy.LifeInsuranceOldMax
	}
}

// ceilDiv は正の金額の切り上げ除算。
func ceilDiv(m model.Money, d int64) model.Money {
	return model.Money((m.Yen() + d - 1) / d)
}

// lifeInsuranceCategory は新旧合算で1区分の控除額を計算する (合算上限4万)。
// max(新のみ, 旧のみ, min(新+旧, 4万)) を採用する。
func lifeInsuranceCategory(newPremium, oldPremium model.Money) model.Money {
	newOnly := lifeInsuranceNew(newPremium)
	oldOnly := lifeInsuranceOld(oldPremium)
	if newPremium > 0 && oldPremium > 0 {
		combined := (newOnly + oldOnly).Min(policy.LifeInsuranceCombinedMax)
		return newOnly.Max(oldOnly).Max(combined)
	}
	if newPremium > 0 {
		return newOnly
	}
	return oldOnly
}

// LifeInsuranceDeduction は生命保険料控除の合計を計算する (3区分、合計上限12万)。
func LifeInsuranceDeduction(p model.LifeInsurancePremiums) model.Money {
	general := lifeInsuranceCategory(p.GeneralNew, p.GeneralOld)
	medical := lifeInsuranceNew(p.MedicalCare) // 介護医療は新制度のみ
	annuity := lifeInsuranceCategory(p.AnnuityNew, p.AnnuityOld)
	return (general + medical + annuity).Min(policy.LifeInsuranceTotalMax)
}

// EarthquakeInsuranceDeduction は地震保険料控除を計算する (所得税法第77条)。
// 旧長期損害保険料にも対応する。合算上限 5 万円。
func EarthquakeInsuranceDeduction(earthquakePremium, oldLongTermPremium model.Money) model.Money {
	var eq model.Money
	if earthquakePremium > 0 {
		eq = earthquakePremium.Min(policy.EarthquakeInsuranceMax)
	}
	var old model.Money
	switch {
	case oldLongTermPremium <= 0:
	case oldLongTermPremium <= policy.OldLongTermBracket1:
		old = oldLongTermPremium
	case oldLongTermPremium <= policy.OldLongTermBracket2:
		old = ceilDiv(oldLongTermPremium, 2) + 2_500
	default:
		old = policy.OldLongTermMax
	}
	return (eq + old).Min(policy.EarthquakeInsuranceMax)
}

// WidowDeduction は寡婦控除/ひとり親控除を計算する (所得500万以下)。
func WidowDeduction(status model.WidowStatus, totalIncome model.Money) model.Money {
	if totalIncome > policy.PersonalDeductionIncomeMax {
		return 0
	}
	switch status {
	case model.WidowSingleParent:
		return policy.SingleParentDeduction
	case model.WidowWidow:
		return policy.WidowDeduction
	default:
		return 0
	}
}

// SelfDisabilityDeduction は本人の障害者控除を計算する。
// 本人は「同居特別障害者」に該当しないため special_cohabiting は特別障害者として扱う。
func SelfDisabilityDeduction(kind model.DisabilityKind) model.Money {
	switch kind {
	case model.DisabilitySpecial, model.DisabilitySpecialCohabiting:
		return policy.DisabilitySpecial
	case model.DisabilityGeneral:
		return policy.DisabilityGeneral
	default:
		return 0
	}
}

// disabilityDeductionFor は扶養親族・配偶者の障害者控除を計算する。
func disabilityDeductionFor(kind model.DisabilityKind) (model.Money, string) {
	switch kind {
	case model.DisabilitySpecialCohabiting:
		return policy.DisabilitySpecialCohabiting, "同居特別障害者"
	case model.DisabilitySpecial:
		return policy.DisabilitySpecial, "特別障害者"
	case model.DisabilityGeneral:
		return policy.DisabilityGeneral, "一般障害者"
	default:
		return 0, ""
	}
}

// WorkingStudentDeduction は勤労学生控除を計算する (合計所得85万以下)。
func WorkingStudentDeduction(isWorkingStudent bool, totalIncome model.Money) model.Money {
	if isWorkingStudent && totalIncome <= policy.WorkingStudentIncomeMax {
		return policy.WorkingStudentDeduction
	}
	return 0
}

// SelfMedicationDeduction はセルフメディケーション税制の控除額を計算する。
// 控除額 = OTC 医薬品購入額 − 12,000 (上限 88,000)。医療費控除との選択適用。
func SelfMedicationDeduction(expenses model.Money) model.Money {
	if expenses <= policy.SelfMedicationThreshold {
		return 0
	}
	return (expenses - policy.SelfMedicationThreshold).Min(policy.SelfMedicationMax)
}

// MedicalDeduction は医療費控除を計算する (所得税法第73条)。
// 足切り = min(10万, 総所得×5%)、上限 200 万円。
func MedicalDeduction(expenses, totalIncome model.Money) model.Money {
	threshold := model.Money(policy.MedicalExpenseThreshold).
		Min(totalIncome.MulDiv(policy.MedicalExpenseIncomeRatioPct, 100))
	if expenses <= threshold {
		return 0
	}
	return (expenses - threshold).Min(policy.MedicalExpenseMax)
}

// DonationIncomeDeduction は寄附金の所得控除額を計算する (所得税法第78条)。
// donationTotal はふるさと納税と他の寄附金の合算額。
// 総所得の40%上限と 2,000 円の足切りを 1 回だけ適用する。
func DonationIncomeDeduction(donationTotal, totalIncome model.Money) model.Money {
	if donationTotal <= policy.DonationSelfBurden {
		return 0
	}
	limit := totalIncome.MulDiv(policy.DonationIncomeDeductionRatioPct, 100)
	return (donationTotal.Min(limit) - policy.DonationSelfBurden).ClampNonNegative()
}

// SpouseDeduction は配偶者控除/配偶者特別控除を計算する (令和7年分)。
//
// 老人控除対象配偶者 (70歳以上・所得58万以下) は増額された控除を適用する。
// 年齢は税法上の満年齢 (誕生日前日加齢) で年度末時点を判定する。
func SpouseDeduction(spouse *model.Spouse, taxpayerIncome model.Money, year model.FiscalYear) model.Money {
	if spouse == nil || spouse.OtherTaxpayerDependent {
		return 0
	}
	if taxpayerIncome > policy.SpouseTaxpayerIncomeMax {
		return 0
	}
	// 老人控除対象配偶者: 配偶者控除の対象 (所得58万以下) かつ 70歳以上
	if spouse.Income <= policy.DependentIncomeMax &&
		!spouse.BirthDate.IsZero() &&
		spouse.BirthDate.TaxAgeAt(year.End()) >= policy.SpouseElderlyAgeMin {
		switch {
		case taxpayerIncome <= policy.SpouseTaxpayerBracket1:
			return policy.SpouseElderlyDeduction
		case taxpayerIncome <= policy.SpouseTaxpayerBracket2:
			return policy.SpouseElderlyDeduction9M
		default:
			return policy.SpouseElderlyDeduction10M
		}
	}
	var table []policy.BracketEntry
	switch {
	case taxpayerIncome <= policy.SpouseTaxpayerBracket1:
		table = policy.SpouseDeductionTable
	case taxpayerIncome <= policy.SpouseTaxpayerBracket2:
		table = policy.SpouseDeductionTable9M
	default:
		table = policy.SpouseDeductionTable10M
	}
	return model.Money(policy.LookupBracket(table, spouse.Income.Yen(), 0))
}

// DependentDeductions は扶養親族に係る控除を計算する
// (扶養控除 + 特定親族特別控除 + 障害者控除)。
//
// 年齢は税法上の満年齢 (誕生日前日加齢) で年度末時点を判定する。
func DependentDeductions(dependents []model.Dependent, year model.FiscalYear) []DeductionItem {
	yearEnd := year.End()
	var items []DeductionItem
	for i := range dependents {
		dep := &dependents[i]
		// 他の納税者の扶養親族 → 二重控除防止のため除外
		if dep.OtherTaxpayerDependent {
			continue
		}
		age := dep.BirthDate.TaxAgeAt(yearEnd)
		isSpecificAge := age >= policy.DependentAgeSpecificMin && age < policy.DependentAgeSpecificMax

		// 所得要件: 19〜22歳は123万まで許容 (特定親族特別控除)、それ以外は58万
		if isSpecificAge {
			if dep.Income > policy.SpecificRelativeSpecialIncomeMax {
				continue
			}
		} else if dep.Income > policy.DependentIncomeMax {
			continue
		}

		if item, ok := dependentAgeDeduction(dep, age, isSpecificAge); ok {
			items = append(items, item)
		}

		// 障害者控除 (年齢制限なし・16歳未満でも適用可)。
		// 対象は「扶養親族」なので所得要件 58万以下を満たす場合のみ
		// (特定親族特別控除の対象者 (所得58万超) は扶養親族ではない)。
		if dep.Income <= policy.DependentIncomeMax {
			if amount, label := disabilityDeductionFor(dep.Disability); amount > 0 {
				items = append(items, DeductionItem{
					Type:    DeductionDisability,
					Name:    "障害者控除",
					Amount:  amount,
					Details: fmt.Sprintf("%s（%s）", dep.Name, label),
				})
			}
		}
	}
	return items
}

// dependentAgeDeduction は年齢・所得に応じた扶養控除/特定親族特別控除を返す。
func dependentAgeDeduction(dep *model.Dependent, age int, isSpecificAge bool) (DeductionItem, bool) {
	switch {
	case age >= policy.DependentAgeElderly:
		// 同居老親等 (58万) は本人または配偶者の直系尊属かつ同居の場合のみ。
		// それ以外の老人扶養親族は 48万。
		if dep.Cohabiting && dep.DirectAscendant {
			return DeductionItem{
				Type: DeductionDependent, Name: "扶養控除",
				Amount:  policy.DependentElderlyCohabiting,
				Details: fmt.Sprintf("%s（同居老親等）", dep.Name),
			}, true
		}
		return DeductionItem{
			Type: DeductionDependent, Name: "扶養控除",
			Amount:  policy.DependentElderly,
			Details: fmt.Sprintf("%s（老人扶養）", dep.Name),
		}, true
	case isSpecificAge:
		if dep.Income <= policy.DependentIncomeMax {
			return DeductionItem{
				Type: DeductionDependent, Name: "扶養控除",
				Amount:  policy.DependentSpecific,
				Details: fmt.Sprintf("%s（特定扶養）", dep.Name),
			}, true
		}
		// 所得58万超〜123万: 特定親族特別控除 (段階的逓減)
		amount := policy.LookupBracket(policy.SpecificRelativeSpecialDeductionTable, dep.Income.Yen(), 0)
		if amount > 0 {
			return DeductionItem{
				Type: DeductionSpecificRelative, Name: "特定親族特別控除",
				Amount:  model.Money(amount),
				Details: fmt.Sprintf("%s（所得%d円）", dep.Name, dep.Income.Yen()),
			}, true
		}
		return DeductionItem{}, false
	case age >= policy.DependentAgeMin:
		return DeductionItem{
			Type: DeductionDependent, Name: "扶養控除",
			Amount:  policy.DependentGeneral,
			Details: fmt.Sprintf("%s（一般扶養）", dep.Name),
		}, true
	default:
		// 16歳未満: 扶養控除なし (児童手当対象)
		return DeductionItem{}, false
	}
}

// spouseDisabilityItem は同一生計配偶者 (所得58万以下) の障害者控除を返す。
// 配偶者控除とは独立に適用できる (所得税法第79条)。
func spouseDisabilityItem(spouse *model.Spouse) (DeductionItem, bool) {
	if spouse == nil || spouse.OtherTaxpayerDependent {
		return DeductionItem{}, false
	}
	if spouse.Income > policy.DependentIncomeMax {
		return DeductionItem{}, false
	}
	amount, label := disabilityDeductionFor(spouse.Disability)
	if amount == 0 {
		return DeductionItem{}, false
	}
	return DeductionItem{
		Type:    DeductionDisability,
		Name:    "障害者控除",
		Amount:  amount,
		Details: fmt.Sprintf("%s（配偶者・%s）", spouse.Name, label),
	}, true
}
