package usecase

import (
	"context"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/repository"
)

// Validatable は自己検証でき、課税年度を持つエンティティのポインタ制約。
type Validatable[T any] interface {
	*T
	Validate() error
	Year() model.FiscalYear
}

// MaterialUsecase は年度スコープの申告資料 (控除素材・所得資料) の共通
// アプリケーションサービス。ドメイン検証してから永続化する。
type MaterialUsecase[T any] interface {
	Add(ctx context.Context, record *T) (int64, error)
	Get(ctx context.Context, id int64) (*T, error)
	Update(ctx context.Context, record *T) error
	List(ctx context.Context, year model.FiscalYear) ([]T, error)
	Delete(ctx context.Context, id int64) error
}

type materialUsecase[T any, PT Validatable[T]] struct {
	repo repository.YearScopedRepository[T]
}

// NewMaterialUsecase は MaterialUsecase を生成する。
// PT は *T が Validate() error を持つことを保証する型パラメータ。
func NewMaterialUsecase[T any, PT Validatable[T]](
	repo repository.YearScopedRepository[T],
) MaterialUsecase[T] {
	return &materialUsecase[T, PT]{repo: repo}
}

// validateForPersist は永続化前の検証 (ドメイン検証 + 年度の必須化)。
// 計算入力では年度 0 を許容するが、保存するレコードは有効な年度が必須。
func (u *materialUsecase[T, PT]) validateForPersist(record *T) error {
	if err := PT(record).Year().Validate(); err != nil {
		return err
	}
	return PT(record).Validate()
}

func (u *materialUsecase[T, PT]) Add(ctx context.Context, record *T) (int64, error) {
	if err := u.validateForPersist(record); err != nil {
		return 0, err
	}
	return u.repo.Create(ctx, record)
}

// validateID は正の ID であることを検証する。
func validateID(id int64) error {
	if id <= 0 {
		return apperrors.New(apperrors.CodeBadRequest, "IDが不正です")
	}
	return nil
}

func (u *materialUsecase[T, PT]) Get(ctx context.Context, id int64) (*T, error) {
	if err := validateID(id); err != nil {
		return nil, err
	}
	return u.repo.FindByID(ctx, id)
}

func (u *materialUsecase[T, PT]) Update(ctx context.Context, record *T) error {
	if err := u.validateForPersist(record); err != nil {
		return err
	}
	return u.repo.Update(ctx, record)
}

func (u *materialUsecase[T, PT]) List(ctx context.Context, year model.FiscalYear) ([]T, error) {
	if err := year.Validate(); err != nil {
		return nil, err
	}
	return u.repo.ListByFiscalYear(ctx, year)
}

func (u *materialUsecase[T, PT]) Delete(ctx context.Context, id int64) error {
	if err := validateID(id); err != nil {
		return err
	}
	return u.repo.Delete(ctx, id)
}

// SpouseUsecase は配偶者情報 (年度1件) のアプリケーションサービス。
type SpouseUsecase interface {
	Set(ctx context.Context, spouse *model.Spouse) error
	Get(ctx context.Context, year model.FiscalYear) (*model.Spouse, error)
	Delete(ctx context.Context, year model.FiscalYear) error
}

type spouseUsecase struct {
	repo repository.SpouseRepository
}

// NewSpouseUsecase は SpouseUsecase を生成する。
func NewSpouseUsecase(repo repository.SpouseRepository) SpouseUsecase {
	return &spouseUsecase{repo: repo}
}

func (u *spouseUsecase) Set(ctx context.Context, spouse *model.Spouse) error {
	if err := spouse.Validate(); err != nil {
		return err
	}
	if err := spouse.FiscalYear.Validate(); err != nil {
		return err
	}
	return u.repo.Upsert(ctx, spouse)
}

func (u *spouseUsecase) Get(ctx context.Context, year model.FiscalYear) (*model.Spouse, error) {
	if err := year.Validate(); err != nil {
		return nil, err
	}
	return u.repo.FindByFiscalYear(ctx, year)
}

func (u *spouseUsecase) Delete(ctx context.Context, year model.FiscalYear) error {
	if err := year.Validate(); err != nil {
		return err
	}
	return u.repo.DeleteByFiscalYear(ctx, year)
}

// Materials は申告資料の全アプリケーションサービスをまとめた束。
// registry での組み立てと interface 層への受け渡しを簡潔にする。
type Materials struct {
	WithholdingSlips     MaterialUsecase[model.WithholdingSlip]
	Dependents           MaterialUsecase[model.Dependent]
	FurusatoDonations    MaterialUsecase[model.FurusatoDonation]
	Donations            MaterialUsecase[model.DonationRecord]
	MedicalExpenses      MaterialUsecase[model.MedicalExpense]
	SocialInsurances     MaterialUsecase[model.SocialInsuranceItem]
	InsurancePolicies    MaterialUsecase[model.InsurancePolicy]
	BusinessWithholdings MaterialUsecase[model.BusinessWithholding]
	LossCarryforwards    MaterialUsecase[model.LossCarryforward]
	HousingLoans         MaterialUsecase[model.HousingLoanDetail]
	FixedAssets          MaterialUsecase[model.FixedAsset]
	OtherIncomes         MaterialUsecase[model.OtherIncome]
	Spouse               SpouseUsecase
}

// MaterialRepositories は Materials の組み立てに必要なリポジトリの束。
type MaterialRepositories struct {
	WithholdingSlips     repository.WithholdingSlipRepository
	Dependents           repository.DependentRepository
	FurusatoDonations    repository.FurusatoDonationRepository
	Donations            repository.DonationRepository
	MedicalExpenses      repository.MedicalExpenseRepository
	SocialInsurances     repository.SocialInsuranceRepository
	InsurancePolicies    repository.InsurancePolicyRepository
	BusinessWithholdings repository.BusinessWithholdingRepository
	LossCarryforwards    repository.LossCarryforwardRepository
	HousingLoans         repository.HousingLoanRepository
	FixedAssets          repository.FixedAssetRepository
	OtherIncomes         repository.OtherIncomeRepository
	Spouse               repository.SpouseRepository
}

// Validate は全フィールドが注入済みであることを検証する (DI 設定ミスの早期検出)。
func (r *MaterialRepositories) Validate() error {
	missing := func(name string) error {
		return apperrors.Newf(apperrors.CodeInternal, "リポジトリ %s が未注入です", name)
	}
	switch {
	case r.WithholdingSlips == nil:
		return missing("WithholdingSlips")
	case r.Dependents == nil:
		return missing("Dependents")
	case r.FurusatoDonations == nil:
		return missing("FurusatoDonations")
	case r.Donations == nil:
		return missing("Donations")
	case r.MedicalExpenses == nil:
		return missing("MedicalExpenses")
	case r.SocialInsurances == nil:
		return missing("SocialInsurances")
	case r.InsurancePolicies == nil:
		return missing("InsurancePolicies")
	case r.BusinessWithholdings == nil:
		return missing("BusinessWithholdings")
	case r.LossCarryforwards == nil:
		return missing("LossCarryforwards")
	case r.HousingLoans == nil:
		return missing("HousingLoans")
	case r.FixedAssets == nil:
		return missing("FixedAssets")
	case r.OtherIncomes == nil:
		return missing("OtherIncomes")
	case r.Spouse == nil:
		return missing("Spouse")
	}
	return nil
}

// NewMaterials は申告資料の全アプリケーションサービスを組み立てる。
func NewMaterials(repos MaterialRepositories) (*Materials, error) {
	if err := repos.Validate(); err != nil {
		return nil, err
	}
	return &Materials{
		WithholdingSlips:     NewMaterialUsecase[model.WithholdingSlip](repos.WithholdingSlips),
		Dependents:           NewMaterialUsecase[model.Dependent](repos.Dependents),
		FurusatoDonations:    NewMaterialUsecase[model.FurusatoDonation](repos.FurusatoDonations),
		Donations:            NewMaterialUsecase[model.DonationRecord](repos.Donations),
		MedicalExpenses:      NewMaterialUsecase[model.MedicalExpense](repos.MedicalExpenses),
		SocialInsurances:     NewMaterialUsecase[model.SocialInsuranceItem](repos.SocialInsurances),
		InsurancePolicies:    NewMaterialUsecase[model.InsurancePolicy](repos.InsurancePolicies),
		BusinessWithholdings: NewMaterialUsecase[model.BusinessWithholding](repos.BusinessWithholdings),
		LossCarryforwards:    NewMaterialUsecase[model.LossCarryforward](repos.LossCarryforwards),
		HousingLoans:         NewMaterialUsecase[model.HousingLoanDetail](repos.HousingLoans),
		FixedAssets:          NewMaterialUsecase[model.FixedAsset](repos.FixedAssets),
		OtherIncomes:         NewMaterialUsecase[model.OtherIncome](repos.OtherIncomes),
		Spouse:               NewSpouseUsecase(repos.Spouse),
	}, nil
}
