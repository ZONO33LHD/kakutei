package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/repository"
)

// filingRepo は repository.YearScopedRepository[T] の SQLite 実装。
//
// 申告資料 (源泉徴収票・扶養親族・寄附金等) は kind 別の JSON ドキュメントとして
// filing_records テーブルに保持する。エンティティの検証は domain/model の
// Validate が担い、リポジトリは年度スコープと ID 採番に責任を持つ。
//
// スキーマ進化の方針: JSON はドメイン構造体の直接シリアライズであるため、
// フィールドの改名・型変更を行う場合は migrations (migrate.go) に既存 data を
// 変換するマイグレーションを必ず追加する。追加フィールドはゼロ値で復元される
// (後方互換)。ID と年度は列の値が正であり、読み取り時に JSON を上書きする。
type filingRepo[T any] struct {
	db      *sql.DB
	kind    string // filing_records.kind の値
	label   string // エラーメッセージ用の日本語名
	yearOf  func(*T) model.FiscalYear
	setYear func(*T, model.FiscalYear)
	idOf    func(*T) int64
	setID   func(*T, int64)
}

// Create は明細を保存し、採番された ID を record にも書き戻して返す。
// record の ID はコミット成功後にのみ設定する (ロールバック時に幽霊 ID を残さない)。
func (r *filingRepo[T]) Create(ctx context.Context, record *T) (int64, error) {
	var id int64
	err := inTx(ctx, r.db, func(tx *sql.Tx) error {
		year := r.yearOf(record)
		if err := ensureYearOpen(ctx, tx, year); err != nil {
			return err
		}
		// ID は列で管理するため、JSON には確定後の値を保存する (先に採番)
		result, err := tx.ExecContext(ctx,
			"INSERT INTO filing_records (fiscal_year, kind, data) VALUES (?, ?, '{}')",
			int(year), r.kind)
		if err != nil {
			return wrapInternal(err, r.label+"の保存")
		}
		id, err = result.LastInsertId()
		if err != nil {
			return wrapInternal(err, r.label+"のID取得")
		}
		// マーシャルはコピーに ID を設定して行い、呼び出し元の record は変更しない
		clone := *record
		r.setID(&clone, id)
		data, err := json.Marshal(&clone)
		if err != nil {
			return wrapInternal(err, r.label+"のシリアライズ")
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE filing_records SET data = ? WHERE id = ?", string(data), id); err != nil {
			return wrapInternal(err, r.label+"の保存")
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	r.setID(record, id)
	return id, nil
}

// FindByID は明細を1件取得する。
// ID と年度は列の値を正とし、JSON 内の値を上書きする (二重管理の整合性保証)。
func (r *filingRepo[T]) FindByID(ctx context.Context, id int64) (*T, error) {
	var data string
	var storedYear int
	err := r.db.QueryRowContext(ctx,
		"SELECT data, fiscal_year FROM filing_records WHERE id = ? AND kind = ?", id, r.kind).
		Scan(&data, &storedYear)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notFound(fmt.Sprintf("%s (ID: %d)", r.label, id))
	}
	if err != nil {
		return nil, wrapInternal(err, r.label+"の取得")
	}
	record := new(T)
	if err := json.Unmarshal([]byte(data), record); err != nil {
		return nil, wrapInternal(err, r.label+"の復元")
	}
	r.setID(record, id)
	r.setYear(record, model.FiscalYear(storedYear))
	return record, nil
}

// Update は明細を差し替える。保存済みレコードの年度は変更不可。
func (r *filingRepo[T]) Update(ctx context.Context, record *T) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		id := r.idOf(record)
		var storedYear int
		err := tx.QueryRowContext(ctx,
			"SELECT fiscal_year FROM filing_records WHERE id = ? AND kind = ?", id, r.kind).
			Scan(&storedYear)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound(fmt.Sprintf("%s (ID: %d)", r.label, id))
		}
		if err != nil {
			return wrapInternal(err, r.label+"の取得")
		}
		if model.FiscalYear(storedYear) != r.yearOf(record) {
			return apperrors.Newf(apperrors.CodeBadRequest, "%sの年度は変更できません", r.label)
		}
		if err := ensureYearOpen(ctx, tx, model.FiscalYear(storedYear)); err != nil {
			return err
		}
		data, err := json.Marshal(record)
		if err != nil {
			return wrapInternal(err, r.label+"のシリアライズ")
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE filing_records SET data = ?, updated_at = datetime('now') WHERE id = ?",
			string(data), id); err != nil {
			return wrapInternal(err, r.label+"の更新")
		}
		return nil
	})
}

// ListByFiscalYear は年度の明細を ID 順に返す。
func (r *filingRepo[T]) ListByFiscalYear(ctx context.Context, year model.FiscalYear) ([]T, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, data FROM filing_records WHERE fiscal_year = ? AND kind = ? ORDER BY id",
		int(year), r.kind)
	if err != nil {
		return nil, wrapInternal(err, r.label+"の取得")
	}
	defer func() { _ = rows.Close() }()

	var records []T
	for rows.Next() {
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, wrapInternal(err, r.label+"の読み取り")
		}
		record := new(T)
		if err := json.Unmarshal([]byte(data), record); err != nil {
			return nil, wrapInternal(err, r.label+"の復元")
		}
		r.setID(record, id)
		r.setYear(record, year)
		records = append(records, *record)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapInternal(err, r.label+"の走査")
	}
	return records, nil
}

func (r *filingRepo[T]) Delete(ctx context.Context, id int64) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		var storedYear int
		err := tx.QueryRowContext(ctx,
			"SELECT fiscal_year FROM filing_records WHERE id = ? AND kind = ?", id, r.kind).
			Scan(&storedYear)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound(fmt.Sprintf("%s (ID: %d)", r.label, id))
		}
		if err != nil {
			return wrapInternal(err, r.label+"の取得")
		}
		if err := ensureYearOpen(ctx, tx, model.FiscalYear(storedYear)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM filing_records WHERE id = ?", id); err != nil {
			return wrapInternal(err, r.label+"の削除")
		}
		return nil
	})
}

// --- 各申告資料のコンストラクタ ---

func NewWithholdingSlipRepository(db *sql.DB) repository.WithholdingSlipRepository {
	return &filingRepo[model.WithholdingSlip]{
		db: db, kind: "withholding_slip", label: "源泉徴収票",
		yearOf:  func(r *model.WithholdingSlip) model.FiscalYear { return r.FiscalYear },
		setYear: func(r *model.WithholdingSlip, y model.FiscalYear) { r.FiscalYear = y },
		idOf:    func(r *model.WithholdingSlip) int64 { return r.ID },
		setID:   func(r *model.WithholdingSlip, id int64) { r.ID = id },
	}
}

func NewDependentRepository(db *sql.DB) repository.DependentRepository {
	return &filingRepo[model.Dependent]{
		db: db, kind: "dependent", label: "扶養親族",
		yearOf:  func(r *model.Dependent) model.FiscalYear { return r.FiscalYear },
		setYear: func(r *model.Dependent, y model.FiscalYear) { r.FiscalYear = y },
		idOf:    func(r *model.Dependent) int64 { return r.ID },
		setID:   func(r *model.Dependent, id int64) { r.ID = id },
	}
}

func NewFurusatoDonationRepository(db *sql.DB) repository.FurusatoDonationRepository {
	return &filingRepo[model.FurusatoDonation]{
		db: db, kind: "furusato_donation", label: "ふるさと納税",
		yearOf:  func(r *model.FurusatoDonation) model.FiscalYear { return r.FiscalYear },
		setYear: func(r *model.FurusatoDonation, y model.FiscalYear) { r.FiscalYear = y },
		idOf:    func(r *model.FurusatoDonation) int64 { return r.ID },
		setID:   func(r *model.FurusatoDonation, id int64) { r.ID = id },
	}
}

func NewDonationRepository(db *sql.DB) repository.DonationRepository {
	return &filingRepo[model.DonationRecord]{
		db: db, kind: "donation", label: "寄附金",
		yearOf:  func(r *model.DonationRecord) model.FiscalYear { return r.FiscalYear },
		setYear: func(r *model.DonationRecord, y model.FiscalYear) { r.FiscalYear = y },
		idOf:    func(r *model.DonationRecord) int64 { return r.ID },
		setID:   func(r *model.DonationRecord, id int64) { r.ID = id },
	}
}

func NewMedicalExpenseRepository(db *sql.DB) repository.MedicalExpenseRepository {
	return &filingRepo[model.MedicalExpense]{
		db: db, kind: "medical_expense", label: "医療費明細",
		yearOf:  func(r *model.MedicalExpense) model.FiscalYear { return r.FiscalYear },
		setYear: func(r *model.MedicalExpense, y model.FiscalYear) { r.FiscalYear = y },
		idOf:    func(r *model.MedicalExpense) int64 { return r.ID },
		setID:   func(r *model.MedicalExpense, id int64) { r.ID = id },
	}
}

func NewSocialInsuranceRepository(db *sql.DB) repository.SocialInsuranceRepository {
	return &filingRepo[model.SocialInsuranceItem]{
		db: db, kind: "social_insurance", label: "社会保険料",
		yearOf:  func(r *model.SocialInsuranceItem) model.FiscalYear { return r.FiscalYear },
		setYear: func(r *model.SocialInsuranceItem, y model.FiscalYear) { r.FiscalYear = y },
		idOf:    func(r *model.SocialInsuranceItem) int64 { return r.ID },
		setID:   func(r *model.SocialInsuranceItem, id int64) { r.ID = id },
	}
}

func NewInsurancePolicyRepository(db *sql.DB) repository.InsurancePolicyRepository {
	return &filingRepo[model.InsurancePolicy]{
		db: db, kind: "insurance_policy", label: "保険契約",
		yearOf:  func(r *model.InsurancePolicy) model.FiscalYear { return r.FiscalYear },
		setYear: func(r *model.InsurancePolicy, y model.FiscalYear) { r.FiscalYear = y },
		idOf:    func(r *model.InsurancePolicy) int64 { return r.ID },
		setID:   func(r *model.InsurancePolicy, id int64) { r.ID = id },
	}
}

func NewBusinessWithholdingRepository(db *sql.DB) repository.BusinessWithholdingRepository {
	return &filingRepo[model.BusinessWithholding]{
		db: db, kind: "business_withholding", label: "事業源泉徴収",
		yearOf:  func(r *model.BusinessWithholding) model.FiscalYear { return r.FiscalYear },
		setYear: func(r *model.BusinessWithholding, y model.FiscalYear) { r.FiscalYear = y },
		idOf:    func(r *model.BusinessWithholding) int64 { return r.ID },
		setID:   func(r *model.BusinessWithholding, id int64) { r.ID = id },
	}
}

func NewLossCarryforwardRepository(db *sql.DB) repository.LossCarryforwardRepository {
	return &filingRepo[model.LossCarryforward]{
		db: db, kind: "loss_carryforward", label: "繰越損失",
		yearOf:  func(r *model.LossCarryforward) model.FiscalYear { return r.FiscalYear },
		setYear: func(r *model.LossCarryforward, y model.FiscalYear) { r.FiscalYear = y },
		idOf:    func(r *model.LossCarryforward) int64 { return r.ID },
		setID:   func(r *model.LossCarryforward, id int64) { r.ID = id },
	}
}

func NewHousingLoanRepository(db *sql.DB) repository.HousingLoanRepository {
	return &filingRepo[model.HousingLoanDetail]{
		db: db, kind: "housing_loan", label: "住宅ローン控除明細",
		yearOf:  func(r *model.HousingLoanDetail) model.FiscalYear { return r.FiscalYear },
		setYear: func(r *model.HousingLoanDetail, y model.FiscalYear) { r.FiscalYear = y },
		idOf:    func(r *model.HousingLoanDetail) int64 { return r.ID },
		setID:   func(r *model.HousingLoanDetail, id int64) { r.ID = id },
	}
}

func NewFixedAssetRepository(db *sql.DB) repository.FixedAssetRepository {
	return &filingRepo[model.FixedAsset]{
		db: db, kind: "fixed_asset", label: "固定資産",
		yearOf:  func(r *model.FixedAsset) model.FiscalYear { return r.FiscalYear },
		setYear: func(r *model.FixedAsset, y model.FiscalYear) { r.FiscalYear = y },
		idOf:    func(r *model.FixedAsset) int64 { return r.ID },
		setID:   func(r *model.FixedAsset, id int64) { r.ID = id },
	}
}

func NewOtherIncomeRepository(db *sql.DB) repository.OtherIncomeRepository {
	return &filingRepo[model.OtherIncome]{
		db: db, kind: "other_income", label: "その他所得",
		yearOf:  func(r *model.OtherIncome) model.FiscalYear { return r.FiscalYear },
		setYear: func(r *model.OtherIncome, y model.FiscalYear) { r.FiscalYear = y },
		idOf:    func(r *model.OtherIncome) int64 { return r.ID },
		setID:   func(r *model.OtherIncome, id int64) { r.ID = id },
	}
}
