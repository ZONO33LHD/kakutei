package rest

import (
	"testing"

	"github.com/ZONO33LHD/kakutei/domain/model"
)

// setRecordID が全申告資料型を網羅していること。
func TestSetRecordID(t *testing.T) {
	records := []struct {
		name   string
		record any
		idOf   func() int64
	}{
		{"WithholdingSlip", &model.WithholdingSlip{}, nil},
		{"Dependent", &model.Dependent{}, nil},
		{"FurusatoDonation", &model.FurusatoDonation{}, nil},
		{"DonationRecord", &model.DonationRecord{}, nil},
		{"MedicalExpense", &model.MedicalExpense{}, nil},
		{"SocialInsuranceItem", &model.SocialInsuranceItem{}, nil},
		{"InsurancePolicy", &model.InsurancePolicy{}, nil},
		{"BusinessWithholding", &model.BusinessWithholding{}, nil},
		{"LossCarryforward", &model.LossCarryforward{}, nil},
		{"HousingLoanDetail", &model.HousingLoanDetail{}, nil},
		{"FixedAsset", &model.FixedAsset{}, nil},
		{"OtherIncome", &model.OtherIncome{}, nil},
	}
	idGetters := []func(any) int64{
		func(r any) int64 { return r.(*model.WithholdingSlip).ID },
		func(r any) int64 { return r.(*model.Dependent).ID },
		func(r any) int64 { return r.(*model.FurusatoDonation).ID },
		func(r any) int64 { return r.(*model.DonationRecord).ID },
		func(r any) int64 { return r.(*model.MedicalExpense).ID },
		func(r any) int64 { return r.(*model.SocialInsuranceItem).ID },
		func(r any) int64 { return r.(*model.InsurancePolicy).ID },
		func(r any) int64 { return r.(*model.BusinessWithholding).ID },
		func(r any) int64 { return r.(*model.LossCarryforward).ID },
		func(r any) int64 { return r.(*model.HousingLoanDetail).ID },
		func(r any) int64 { return r.(*model.FixedAsset).ID },
		func(r any) int64 { return r.(*model.OtherIncome).ID },
	}
	for i, tt := range records {
		t.Run(tt.name, func(t *testing.T) {
			setRecordID(tt.record, 42)
			if idGetters[i](tt.record) != 42 {
				t.Errorf("%s: ID が設定されていない", tt.name)
			}
		})
	}

	defer func() {
		if recover() == nil {
			t.Error("未対応型は panic すべき")
		}
	}()
	setRecordID(&struct{}{}, 1)
}
