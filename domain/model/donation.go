package model

import "github.com/ZONO33LHD/kakutei/domain/apperrors"

// DonationKind は寄附金の種別 (ふるさと納税以外)。
type DonationKind string

const (
	DonationPolitical      DonationKind = "political"       // 政治活動寄附金 (税額控除の選択可)
	DonationNPO            DonationKind = "npo"             // 認定NPO法人 (税額控除の選択可)
	DonationPublicInterest DonationKind = "public_interest" // 公益社団法人等 (税額控除の選択可)
	DonationSpecified      DonationKind = "specified"       // 特定公益増進法人 (所得控除のみ)
	DonationOther          DonationKind = "other"           // その他の特定寄附金
)

// Validate は定義済みの種別かを検証する。
func (k DonationKind) Validate() error {
	switch k {
	case DonationPolitical, DonationNPO, DonationPublicInterest, DonationSpecified, DonationOther:
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正な寄附金種別です: %q", string(k))
}

// CreditSelectable は税額控除との選択適用が可能な種別かどうか。
func (k DonationKind) CreditSelectable() bool {
	switch k {
	case DonationPolitical, DonationNPO, DonationPublicInterest:
		return true
	}
	return false
}

// DonationRecord はふるさと納税以外の寄附金1件。
type DonationRecord struct {
	ID            int64
	FiscalYear    FiscalYear
	Kind          DonationKind
	RecipientName string // 寄附先名
	Amount        Money
	Date          Date
	ReceiptNumber string // 受領証明書番号。空 = 未設定
	SourceFile    string // 読み取り元ファイル。空 = 未設定
}

// Validate は寄附金の自己検証を行う。
func (d *DonationRecord) Validate() error {
	if err := d.Kind.Validate(); err != nil {
		return err
	}
	if d.RecipientName == "" {
		return apperrors.New(apperrors.CodeBadRequest, "寄附先名は必須です")
	}
	if d.Amount <= 0 || !d.Amount.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "寄附金額が不正です: %d", d.Amount.Yen())
	}
	if d.Date.IsZero() {
		return apperrors.New(apperrors.CodeBadRequest, "寄附日は必須です")
	}
	return nil
}

// FurusatoDonation はふるさと納税の寄附1件。
type FurusatoDonation struct {
	ID             int64
	FiscalYear     FiscalYear
	Municipality   string // 寄附先自治体名
	Prefecture     string // 都道府県。空 = 未設定
	Amount         Money
	Date           Date
	ReceiptNumber  string // 受領証明書番号。空 = 未設定
	OneStopApplied bool   // ワンストップ特例を申請済みか
	SourceFile     string
}

// Validate はふるさと納税寄附の自己検証を行う。
func (f *FurusatoDonation) Validate() error {
	if f.Municipality == "" {
		return apperrors.New(apperrors.CodeBadRequest, "寄附先自治体名は必須です")
	}
	if f.Amount <= 0 || !f.Amount.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "寄附金額が不正です: %d", f.Amount.Yen())
	}
	if f.Date.IsZero() {
		return apperrors.New(apperrors.CodeBadRequest, "寄附日は必須です")
	}
	return nil
}
