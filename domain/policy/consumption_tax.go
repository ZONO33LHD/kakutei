package policy

// ============================================================
// 消費税 (消費税法)
// ============================================================

const (
	// 国税と地方消費税の配分 (10% = 国税7.8% + 地方2.2%)
	NationalTaxRatio = 78 // 国税分 78/100
	LocalTaxRatio    = 22 // 地方消費税分 22/78

	// 課税標準額の計算 (消費税法第28条、国税通則法第118条)
	// 課税標準額 = 税込金額 × 100/110 (10%) or × 100/108 (8%)、1,000円未満切捨
	ConsumptionTaxIncludedDenom10 = 110 // 税込→税抜の分母 (標準税率)
	ConsumptionTaxIncludedDenom8  = 108 // 税込→税抜の分母 (軽減税率)
	ConsumptionTaxBaseRounding    = 1_000

	// 消費税額 (国税) = 課税標準額 × 7.8% (標準) or 6.24% (軽減)
	ConsumptionStandardNationalRateNum   = 78 // 7.8% = 78/1000
	ConsumptionStandardNationalRateDenom = 1000
	ConsumptionReducedNationalRateNum    = 624 // 6.24% = 624/10000
	ConsumptionReducedNationalRateDenom  = 10000
)

// SimplifiedBusinessType は簡易課税の事業区分 (1〜6)。
type SimplifiedBusinessType int

const (
	SimplifiedWholesale     SimplifiedBusinessType = 1 // 卸売業
	SimplifiedRetail        SimplifiedBusinessType = 2 // 小売業
	SimplifiedManufacturing SimplifiedBusinessType = 3 // 製造業等
	SimplifiedOther         SimplifiedBusinessType = 4 // その他
	SimplifiedService       SimplifiedBusinessType = 5 // サービス業等
	SimplifiedRealEstate    SimplifiedBusinessType = 6 // 不動産業
)

// Valid は定義済みの事業区分 (1〜6) かどうか。
// ゼロ値 (未指定) は不正。簡易課税の計算では必ず事業区分の指定を要求し、
// 暗黙のフォールバックはしない (みなし仕入率 40〜90% の差は税額に直結するため)。
func (t SimplifiedBusinessType) Valid() bool {
	return t >= SimplifiedWholesale && t <= SimplifiedRealEstate
}

// SimplifiedDeemedRatios は簡易課税のみなし仕入率 (%、消費税法第37条)。
var SimplifiedDeemedRatios = map[SimplifiedBusinessType]int64{
	SimplifiedWholesale:     90,
	SimplifiedRetail:        80,
	SimplifiedManufacturing: 70,
	SimplifiedOther:         60,
	SimplifiedService:       50,
	SimplifiedRealEstate:    40,
}

// Special20PctRatePct は 2 割特例の乗率 (インボイス経過措置、令和8年9月30日まで)。
const Special20PctRatePct = 20
