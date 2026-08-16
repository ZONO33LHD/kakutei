package taxation

import (
	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/policy"
)

// ConsumptionTaxMethod は消費税の計算方式。
type ConsumptionTaxMethod string

const (
	MethodStandard     ConsumptionTaxMethod = "standard"      // 本則課税
	MethodSimplified   ConsumptionTaxMethod = "simplified"    // 簡易課税
	MethodSpecial20Pct ConsumptionTaxMethod = "special_20pct" // 2割特例 (インボイス経過措置)
)

// Validate は定義済みの方式かを検証する。
func (m ConsumptionTaxMethod) Validate() error {
	switch m {
	case MethodStandard, MethodSimplified, MethodSpecial20Pct:
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正な消費税計算方式です: %q", string(m))
}

// ConsumptionTaxInput は消費税計算の入力。売上・仕入は税込金額で指定する。
type ConsumptionTaxInput struct {
	FiscalYear model.FiscalYear
	Method     ConsumptionTaxMethod

	TaxableSales10     model.Money // 課税売上高 (税込、標準税率10%)
	TaxableSales8      model.Money // 課税売上高 (税込、軽減税率8%)
	TaxablePurchases10 model.Money // 課税仕入高 (税込、標準税率10%) — 本則のみ使用
	TaxablePurchases8  model.Money // 課税仕入高 (税込、軽減税率8%) — 本則のみ使用

	// NonTaxableSales は非課税売上高等 (課税売上割合の分母に加算される額)。
	// 本則課税で課税売上割合の判定・一括比例配分に使う。
	NonTaxableSales model.Money

	// SimplifiedBusinessType は簡易課税の事業区分 (1〜6)。簡易課税では必須。
	// ※ 複数事業区分の併用 (事業区分別計算・75%特例) は未対応。
	//   全売上に単一のみなし仕入率を適用する。
	SimplifiedBusinessType policy.SimplifiedBusinessType

	InterimPayment model.Money // 中間納付税額
}

// ConsumptionTaxResult は消費税計算の結果。
//
// 計算フロー (消費税法第28条・第45条、国税通則法第118条・第119条):
//  1. 課税標準額 = 税込金額 × 100/110 (or 100/108)、1,000円未満切捨
//  2. 消費税額 (国税) = 課税標準額 × 7.8% (or 6.24%)
//  3. 控除対象仕入税額 (方式による)
//  4. 差引税額 = 消費税額 − 仕入税額、100円未満切捨 (負なら控除不足還付税額)
//  5. 地方消費税 = 差引税額 × 22/78、100円未満切捨
type ConsumptionTaxResult struct {
	FiscalYear model.FiscalYear
	Method     ConsumptionTaxMethod

	TaxableSalesTotal model.Money // 課税売上高合計 (税込、表示用)
	TaxableBase10     model.Money // 課税標準額 (10%分、税抜、1,000円切捨)
	TaxableBase8      model.Money // 課税標準額 (8%分、税抜、1,000円切捨)

	NationalTaxOnSales model.Money // 消費税額 (国税: 7.8%分+6.24%分)
	TaxOnPurchases     model.Money // 控除対象仕入税額 (国税部分)

	NetTax          model.Money // 差引税額 (100円切捨、正の場合のみ)
	RefundShortfall model.Money // 控除不足還付税額 (仕入税額 > 売上税額の場合)
	InterimPayment  model.Money // 中間納付税額
	TaxDue          model.Money // 国税の納付税額 = 差引税額 − 控除不足還付税額 − 中間納付税額
	LocalTaxDue     model.Money // 地方消費税額 (還付時は負)
	TotalDue        model.Money // 合計納付税額 (国税 + 地方、負 = 還付)

	// ProportionalCreditApplied は一括比例配分方式で仕入税額を按分したか。
	// 課税売上高5億円超または課税売上割合95%未満の場合に true (本則課税のみ)。
	// ※ 個別対応方式は未対応。
	ProportionalCreditApplied bool
}

// ConsumptionTaxService は消費税計算のドメインサービス。
type ConsumptionTaxService struct{}

// NewConsumptionTaxService は ConsumptionTaxService を生成する。
func NewConsumptionTaxService() *ConsumptionTaxService { return &ConsumptionTaxService{} }

// Calculate は消費税を計算する (令和7年分)。
func (s *ConsumptionTaxService) Calculate(in ConsumptionTaxInput) (*ConsumptionTaxResult, error) {
	if err := s.validate(&in); err != nil {
		return nil, err
	}

	// Step 1: 課税標準額 (税込→税抜、1,000円未満切捨)
	base10 := in.TaxableSales10.MulDiv(100, policy.ConsumptionTaxIncludedDenom10).
		RoundDownTo(policy.ConsumptionTaxBaseRounding)
	base8 := in.TaxableSales8.MulDiv(100, policy.ConsumptionTaxIncludedDenom8).
		RoundDownTo(policy.ConsumptionTaxBaseRounding)

	// Step 2: 消費税額 (国税)
	nationalTax := base10.MulDiv(policy.ConsumptionStandardNationalRateNum, policy.ConsumptionStandardNationalRateDenom) +
		base8.MulDiv(policy.ConsumptionReducedNationalRateNum, policy.ConsumptionReducedNationalRateDenom)

	// Step 3: 控除対象仕入税額
	purchasesTax, proportional := s.purchasesTax(&in, nationalTax)
	taxDueRaw := nationalTax - purchasesTax

	// Step 4: 差引税額 or 控除不足還付税額
	var netTax, refundShortfall model.Money
	if taxDueRaw >= 0 {
		netTax = taxDueRaw.RoundDownTo(policy.TaxAmountRoundingUnit)
	} else {
		refundShortfall = -taxDueRaw
	}
	// 国税の納付税額 (負 = 還付)。控除不足還付税額と中間納付還付を含む。
	taxDue := netTax - refundShortfall - in.InterimPayment

	// Step 5: 地方消費税 (22/78、100円未満切捨。還付時は負)
	var localTax model.Money
	switch {
	case netTax > 0:
		localTax = netTax.MulDiv(policy.LocalTaxRatio, policy.NationalTaxRatio).
			RoundDownTo(policy.TaxAmountRoundingUnit)
	case refundShortfall > 0:
		localTax = -refundShortfall.MulDiv(policy.LocalTaxRatio, policy.NationalTaxRatio).
			RoundDownTo(policy.TaxAmountRoundingUnit)
	}

	return &ConsumptionTaxResult{
		FiscalYear:                in.FiscalYear,
		Method:                    in.Method,
		TaxableSalesTotal:         in.TaxableSales10 + in.TaxableSales8,
		TaxableBase10:             base10,
		TaxableBase8:              base8,
		NationalTaxOnSales:        nationalTax,
		TaxOnPurchases:            purchasesTax,
		NetTax:                    netTax,
		RefundShortfall:           refundShortfall,
		InterimPayment:            in.InterimPayment,
		TaxDue:                    taxDue,
		LocalTaxDue:               localTax,
		TotalDue:                  taxDue + localTax,
		ProportionalCreditApplied: proportional,
	}, nil
}

// purchasesTax は方式別の控除対象仕入税額 (国税部分) を計算する。
// 戻り値の bool は一括比例配分方式を適用したかどうか。
func (s *ConsumptionTaxService) purchasesTax(in *ConsumptionTaxInput, nationalTax model.Money) (model.Money, bool) {
	switch in.Method {
	case MethodSpecial20Pct:
		// 2割特例: 仕入控除税額 = 消費税額 (国税) × 80%
		return nationalTax.MulDiv(100-policy.Special20PctRatePct, 100), false
	case MethodSimplified:
		// 簡易課税: 仕入控除税額 = 消費税額 (国税) × みなし仕入率
		ratio := policy.SimplifiedDeemedRatios[in.SimplifiedBusinessType]
		return nationalTax.MulDiv(ratio, 100), false
	default: // MethodStandard (本則課税)
		// 仕入税額 = 税込仕入額 × 7.8/110 (10%品目) + 6.24/108 (8%品目)
		full := in.TaxablePurchases10.MulDiv(
			policy.ConsumptionStandardPurchaseRateNum, policy.ConsumptionStandardPurchaseRateDenom) +
			in.TaxablePurchases8.MulDiv(
				policy.ConsumptionReducedPurchaseRateNum, policy.ConsumptionReducedPurchaseRateDenom)

		// 全額控除の要件 (消費税法第30条第2項):
		// 課税売上高 (税抜) 5億円以下かつ課税売上割合95%以上。
		// 満たさない場合は一括比例配分方式で按分する (個別対応方式は未対応)。
		taxableExcl := in.TaxableSales10.MulDiv(100, policy.ConsumptionTaxIncludedDenom10) +
			in.TaxableSales8.MulDiv(100, policy.ConsumptionTaxIncludedDenom8)
		totalSales := taxableExcl + in.NonTaxableSales
		if totalSales <= 0 {
			return full, false
		}
		ratioOK := taxableExcl.Yen()*100 >= totalSales.Yen()*policy.ConsumptionFullCreditRatioPct
		if taxableExcl <= policy.ConsumptionFullCreditSalesMax && ratioOK {
			return full, false
		}
		return mulDivBig(full, taxableExcl, totalSales), true
	}
}

// validate は入力を検証する。
func (s *ConsumptionTaxService) validate(in *ConsumptionTaxInput) error {
	if err := validateFiscalYear(in.FiscalYear); err != nil {
		return err
	}
	if err := in.Method.Validate(); err != nil {
		return err
	}
	if in.Method == MethodSimplified && !in.SimplifiedBusinessType.Valid() {
		return apperrors.New(apperrors.CodeBadRequest,
			"簡易課税では事業区分 (1〜6) の指定が必須です")
	}
	amounts := []struct {
		name   string
		amount model.Money
	}{
		{"課税売上高(10%)", in.TaxableSales10}, {"課税売上高(8%)", in.TaxableSales8},
		{"課税仕入高(10%)", in.TaxablePurchases10}, {"課税仕入高(8%)", in.TaxablePurchases8},
		{"非課税売上高", in.NonTaxableSales}, {"中間納付税額", in.InterimPayment},
	}
	for _, v := range amounts {
		if v.amount < 0 || !v.amount.ValidateAmountRange() {
			return apperrors.Newf(apperrors.CodeBadRequest, "%sが不正です: %d", v.name, v.amount.Yen())
		}
	}
	return nil
}
