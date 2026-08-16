package repository

import (
	"context"

	"github.com/ZONO33LHD/kakutei/domain/model"
)

// YearScopedRepository は年度スコープの明細レコードに共通する CRUD 契約。
//
// 確定申告の控除素材 (源泉徴収票・扶養親族・寄附金・医療費等) は
// 「年度に複数件ぶら下がる明細」という同一の形をしているため、
// ジェネリクスで契約を統一する。
type YearScopedRepository[T any] interface {
	// Create は明細を保存し、採番された ID を返す。
	Create(ctx context.Context, record *T) (int64, error)

	// FindByID は明細を1件取得する。存在しなければ CodeNotFound。
	// 年度締めの検証 (対象レコードの年度確認) に使う。
	FindByID(ctx context.Context, id int64) (*T, error)

	// Update は明細を差し替える。存在しなければ CodeNotFound。
	// 保存済みレコードの年度 (FiscalYear) は変更不可とし、変更要求は CodeBadRequest。
	// (繰越損失の充当額・固定資産の償却累計額の更新等に使う)
	Update(ctx context.Context, record *T) error

	// ListByFiscalYear は年度の明細を ID 順に返す。
	ListByFiscalYear(ctx context.Context, year model.FiscalYear) ([]T, error)

	// Delete は明細を1件削除する。存在しなければ CodeNotFound。
	Delete(ctx context.Context, id int64) error
}

// 確定申告の控除素材・所得資料のリポジトリ契約。
type (
	WithholdingSlipRepository     = YearScopedRepository[model.WithholdingSlip]
	DependentRepository           = YearScopedRepository[model.Dependent]
	FurusatoDonationRepository    = YearScopedRepository[model.FurusatoDonation]
	DonationRepository            = YearScopedRepository[model.DonationRecord]
	MedicalExpenseRepository      = YearScopedRepository[model.MedicalExpense]
	SocialInsuranceRepository     = YearScopedRepository[model.SocialInsuranceItem]
	InsurancePolicyRepository     = YearScopedRepository[model.InsurancePolicy]
	BusinessWithholdingRepository = YearScopedRepository[model.BusinessWithholding]
	LossCarryforwardRepository    = YearScopedRepository[model.LossCarryforward]
	HousingLoanRepository         = YearScopedRepository[model.HousingLoanDetail]
	FixedAssetRepository          = YearScopedRepository[model.FixedAsset]
	OtherIncomeRepository         = YearScopedRepository[model.OtherIncome]
)

// SpouseRepository は配偶者情報の永続化契約 (年度につき1件)。
type SpouseRepository interface {
	// Upsert は年度をキーに配偶者情報を登録・更新する。
	Upsert(ctx context.Context, spouse *model.Spouse) error

	// FindByFiscalYear は年度の配偶者情報を返す。未登録なら (nil, nil)。
	FindByFiscalYear(ctx context.Context, year model.FiscalYear) (*model.Spouse, error)

	// DeleteByFiscalYear は年度の配偶者情報を削除する。未登録でもエラーにしない。
	DeleteByFiscalYear(ctx context.Context, year model.FiscalYear) error
}
