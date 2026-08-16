package model

import "testing"

func TestAccountCodeValidate(t *testing.T) {
	valid := []AccountCode{"1001", "5380", "0000"}
	for _, c := range valid {
		if err := c.Validate(); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", c, err)
		}
	}
	invalid := []AccountCode{"", "100", "10011", "abcd", "10a1", "１００１"}
	for _, c := range invalid {
		if err := c.Validate(); err == nil {
			t.Errorf("Validate(%q) はエラーになるべき", c)
		}
	}
}

func TestAccountCategoryNormalSide(t *testing.T) {
	tests := []struct {
		category AccountCategory
		want     EntrySide
	}{
		{CategoryAsset, SideDebit},
		{CategoryExpense, SideDebit},
		{CategoryLiability, SideCredit},
		{CategoryEquity, SideCredit},
		{CategoryRevenue, SideCredit},
	}
	for _, tt := range tests {
		if got := tt.category.NormalSide(); got != tt.want {
			t.Errorf("%s.NormalSide() = %s, want %s", tt.category, got, tt.want)
		}
	}
}

func TestAccountValidate(t *testing.T) {
	a := Account{Code: "1001", Name: "現金", Category: CategoryAsset, IsActive: true}
	if err := a.Validate(); err != nil {
		t.Errorf("valid account がエラー: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(a *Account)
	}{
		{"名前なし", func(a *Account) { a.Name = "" }},
		{"不正コード", func(a *Account) { a.Code = "x" }},
		{"不正分類", func(a *Account) { a.Category = "income" }},
		{"不正税区分", func(a *Account) { a.TaxCategory = "vat" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bad := a
			tt.mutate(&bad)
			if err := bad.Validate(); err == nil {
				t.Error("エラーになるべき")
			}
		})
	}
}
