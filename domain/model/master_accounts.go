package model

// DefaultAccounts は個人事業・青色申告用の標準勘定科目マスタを返す。
//
// コード体系:
//
//	1xxx: 資産 / 2xxx: 負債 / 3xxx: 純資産 / 4xxx: 収益 / 5xxx: 費用
//
// TaxCategory は科目の既定の消費税区分 (仕訳明細で上書き可能)。
// 呼び出しごとに新しいスライスを返すため、呼び出し側で変更しても安全。
func DefaultAccounts() []Account {
	accounts := []Account{
		// 資産 (流動資産)
		{Code: "1001", Name: "現金", Category: CategoryAsset, SubCategory: "current_asset"},
		{Code: "1002", Name: "普通預金", Category: CategoryAsset, SubCategory: "current_asset"},
		{Code: "1003", Name: "当座預金", Category: CategoryAsset, SubCategory: "current_asset"},
		{Code: "1004", Name: "定期預金", Category: CategoryAsset, SubCategory: "current_asset"},
		{Code: "1010", Name: "売掛金", Category: CategoryAsset, SubCategory: "current_asset"},
		{Code: "1020", Name: "受取手形", Category: CategoryAsset, SubCategory: "current_asset"},
		{Code: "1030", Name: "棚卸資産", Category: CategoryAsset, SubCategory: "current_asset"},
		{Code: "1040", Name: "前払金", Category: CategoryAsset, SubCategory: "current_asset"},
		{Code: "1041", Name: "前払費用", Category: CategoryAsset, SubCategory: "current_asset"},
		{Code: "1050", Name: "立替金", Category: CategoryAsset, SubCategory: "current_asset"},
		{Code: "1060", Name: "仮払金", Category: CategoryAsset, SubCategory: "current_asset"},
		{Code: "1070", Name: "未収入金", Category: CategoryAsset, SubCategory: "current_asset"},
		{Code: "1080", Name: "貸付金", Category: CategoryAsset, SubCategory: "current_asset"},
		{Code: "1090", Name: "仮払消費税", Category: CategoryAsset, SubCategory: "current_asset"},
		// 資産 (固定資産)
		{Code: "1100", Name: "建物", Category: CategoryAsset, SubCategory: "fixed_asset", TaxCategory: ConsumptionTaxable},
		{Code: "1101", Name: "建物附属設備", Category: CategoryAsset, SubCategory: "fixed_asset", TaxCategory: ConsumptionTaxable},
		{Code: "1110", Name: "機械装置", Category: CategoryAsset, SubCategory: "fixed_asset", TaxCategory: ConsumptionTaxable},
		{Code: "1120", Name: "車両運搬具", Category: CategoryAsset, SubCategory: "fixed_asset", TaxCategory: ConsumptionTaxable},
		{Code: "1130", Name: "工具器具備品", Category: CategoryAsset, SubCategory: "fixed_asset", TaxCategory: ConsumptionTaxable},
		{Code: "1140", Name: "土地", Category: CategoryAsset, SubCategory: "fixed_asset", TaxCategory: ConsumptionNonTaxable},
		{Code: "1150", Name: "ソフトウェア", Category: CategoryAsset, SubCategory: "fixed_asset", TaxCategory: ConsumptionTaxable},
		{Code: "1160", Name: "一括償却資産", Category: CategoryAsset, SubCategory: "fixed_asset", TaxCategory: ConsumptionTaxable},
		// 資産 (事業主勘定)
		{Code: "1200", Name: "事業主貸", Category: CategoryAsset, SubCategory: "owner"},
		// 負債 (流動負債)
		{Code: "2001", Name: "買掛金", Category: CategoryLiability, SubCategory: "current_liability"},
		{Code: "2010", Name: "支払手形", Category: CategoryLiability, SubCategory: "current_liability"},
		{Code: "2020", Name: "短期借入金", Category: CategoryLiability, SubCategory: "current_liability"},
		{Code: "2030", Name: "未払金", Category: CategoryLiability, SubCategory: "current_liability"},
		{Code: "2031", Name: "未払費用", Category: CategoryLiability, SubCategory: "current_liability"},
		{Code: "2040", Name: "前受金", Category: CategoryLiability, SubCategory: "current_liability"},
		{Code: "2050", Name: "預り金", Category: CategoryLiability, SubCategory: "current_liability"},
		{Code: "2060", Name: "仮受金", Category: CategoryLiability, SubCategory: "current_liability"},
		{Code: "2070", Name: "未払消費税", Category: CategoryLiability, SubCategory: "current_liability"},
		{Code: "2080", Name: "未払事業税", Category: CategoryLiability, SubCategory: "current_liability"},
		// 負債 (固定負債)
		{Code: "2100", Name: "長期借入金", Category: CategoryLiability, SubCategory: "long_term_liability"},
		// 純資産
		{Code: "3001", Name: "元入金", Category: CategoryEquity, SubCategory: "capital"},
		{Code: "3010", Name: "事業主借", Category: CategoryEquity, SubCategory: "owner"},
		{Code: "3020", Name: "控除前所得金額", Category: CategoryEquity, SubCategory: "retained_earnings"},
		// 収益
		{Code: "4001", Name: "売上", Category: CategoryRevenue, SubCategory: "operating_revenue", TaxCategory: ConsumptionTaxable},
		{Code: "4010", Name: "売上値引・戻り", Category: CategoryRevenue, SubCategory: "operating_revenue", TaxCategory: ConsumptionTaxable},
		{Code: "4100", Name: "受取利息", Category: CategoryRevenue, SubCategory: "non_operating_revenue", TaxCategory: ConsumptionNonTaxable},
		{Code: "4110", Name: "雑収入", Category: CategoryRevenue, SubCategory: "non_operating_revenue", TaxCategory: ConsumptionTaxable},
		{Code: "4120", Name: "家事消費等", Category: CategoryRevenue, SubCategory: "non_operating_revenue", TaxCategory: ConsumptionTaxable},
		// 費用 (売上原価)
		{Code: "5001", Name: "仕入", Category: CategoryExpense, SubCategory: "cost_of_sales", TaxCategory: ConsumptionTaxable},
		// 費用 (販管費)
		{Code: "5100", Name: "租税公課", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionOutOfScope},
		{Code: "5110", Name: "荷造運賃", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5120", Name: "水道光熱費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5130", Name: "旅費交通費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5140", Name: "通信費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5150", Name: "広告宣伝費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5160", Name: "接待交際費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5170", Name: "損害保険料", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionNonTaxable},
		{Code: "5180", Name: "修繕費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5190", Name: "消耗品費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5200", Name: "減価償却費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionOutOfScope},
		{Code: "5210", Name: "福利厚生費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5220", Name: "給料賃金", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionOutOfScope},
		{Code: "5230", Name: "外注工賃", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5240", Name: "利子割引料", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionNonTaxable},
		{Code: "5250", Name: "地代家賃", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5260", Name: "貸倒金", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionOutOfScope},
		{Code: "5270", Name: "雑費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5280", Name: "専従者給与", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionOutOfScope},
		{Code: "5290", Name: "新聞図書費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5300", Name: "研修費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5310", Name: "支払手数料", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5320", Name: "車両費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5330", Name: "会議費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5340", Name: "諸会費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5350", Name: "リース料", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5360", Name: "事務用品費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5370", Name: "ソフトウェア費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
		{Code: "5380", Name: "取材費", Category: CategoryExpense, SubCategory: "operating_expense", TaxCategory: ConsumptionTaxable},
	}
	for i := range accounts {
		accounts[i].IsActive = true
		accounts[i].SortOrder = i
	}
	return accounts
}
