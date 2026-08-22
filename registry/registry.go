// Package registry はアプリケーション全体の依存関係を組み立てる DI コンテナ。
//
// 依存方向:
//
//	interface (rest) → usecase → domain/repository (契約)
//	                              ↑
//	infrastructure/persistence (SQLite 実装) をここで合成する。
package registry

import (
	"context"
	"database/sql"
	"net"
	"net/http"
	"net/netip"
	"os"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"

	"github.com/ZONO33LHD/kakutei/domain/service/bookkeeping"
	"github.com/ZONO33LHD/kakutei/domain/service/taxation"
	"github.com/ZONO33LHD/kakutei/infrastructure/db"
	"github.com/ZONO33LHD/kakutei/infrastructure/persistence"
	"github.com/ZONO33LHD/kakutei/interface/rest"
	"github.com/ZONO33LHD/kakutei/usecase"
)

type Config struct {
	DBPath string
	// Addr は HTTP サーバーの待受アドレス。
	// 確定申告データはローカル利用前提のため、既定はループバックに限定する。
	Addr string
}

func DefaultConfig() Config {
	return Config{
		DBPath: "kakutei.db",
		Addr:   "127.0.0.1:8080",
	}
}

// Registry はサーバーが必要とするリソースを保持する。Close() で解放する。
type Registry struct {
	db     *sql.DB
	router http.Handler
}

// New は DB を開いてマイグレーションし、全レイヤーを組み立てる。
func New(ctx context.Context, cfg Config) (reg *Registry, err error) {
	if err := ensureLoopbackAddr(ctx, cfg.Addr); err != nil {
		return nil, err
	}
	sqlDB, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		return nil, apperrors.Wrapf(err, "DB のオープンに失敗しました")
	}
	// 後段の初期化が失敗した場合に確実に DB を閉じる。
	defer func() {
		if reg == nil {
			_ = sqlDB.Close()
		}
	}()

	if err := persistence.Migrate(ctx, sqlDB); err != nil {
		return nil, apperrors.Wrapf(err, "マイグレーションに失敗しました")
	}

	// repository (SQLite 実装)
	journalRepo := persistence.NewJournalRepository(sqlDB)
	accountRepo := persistence.NewAccountRepository(sqlDB)
	auditRepo := persistence.NewJournalAuditRepository(sqlDB)
	openingRepo := persistence.NewOpeningBalanceRepository(sqlDB)
	yearRepo := persistence.NewFiscalYearRepository(sqlDB)

	materials, err := usecase.NewMaterials(usecase.MaterialRepositories{
		WithholdingSlips:     persistence.NewWithholdingSlipRepository(sqlDB),
		Dependents:           persistence.NewDependentRepository(sqlDB),
		FurusatoDonations:    persistence.NewFurusatoDonationRepository(sqlDB),
		Donations:            persistence.NewDonationRepository(sqlDB),
		MedicalExpenses:      persistence.NewMedicalExpenseRepository(sqlDB),
		SocialInsurances:     persistence.NewSocialInsuranceRepository(sqlDB),
		InsurancePolicies:    persistence.NewInsurancePolicyRepository(sqlDB),
		BusinessWithholdings: persistence.NewBusinessWithholdingRepository(sqlDB),
		LossCarryforwards:    persistence.NewLossCarryforwardRepository(sqlDB),
		HousingLoans:         persistence.NewHousingLoanRepository(sqlDB),
		FixedAssets:          persistence.NewFixedAssetRepository(sqlDB),
		OtherIncomes:         persistence.NewOtherIncomeRepository(sqlDB),
		Spouse:               persistence.NewSpouseRepository(sqlDB),
	})
	if err != nil {
		return nil, err
	}

	// domain service
	statements := bookkeeping.NewStatementService()
	duplicates := bookkeeping.NewDuplicateService()
	incomeTax := taxation.NewIncomeTaxService()
	consumption := taxation.NewConsumptionTaxService()

	// interface (REST)。待受アドレスのホスト名を Host 検証の許可リストへ追加する
	allowedHost := ""
	if host, _, err := net.SplitHostPort(cfg.Addr); err == nil {
		allowedHost = host
	}
	router := rest.NewRouter(rest.Usecases{
		FiscalYears: usecase.NewFiscalYearUsecase(yearRepo, accountRepo),
		Journals:    usecase.NewJournalUsecase(journalRepo, auditRepo, duplicates),
		Reports:     usecase.NewReportUsecase(journalRepo, accountRepo, openingRepo, statements),
		Materials:   materials,
		Filing: usecase.NewFilingUsecase(
			journalRepo, accountRepo, materials, statements, incomeTax, consumption),
	}, allowedHost)

	return &Registry{db: sqlDB, router: router}, nil
}

// ensureLoopbackAddr は待受アドレスがループバックであることを起動時に強制する。
// 認証を持たないため、非ループバック待受は KAKUTEI_ALLOW_NONLOOPBACK=1 の
// 明示的なオプトインがない限り拒否する (HostGuard は Host ヘッダ検証にすぎず、
// 直接のネットワークアクセスに対する認証の代替にはならない)。
//
// 検証は fail closed: host:port 形式で解釈できないアドレス (空を含む) は拒否し、
// ホスト名は名前解決の結果すべてがループバックであることを確認する
// (net.Listen と同じ解決系を通すため、検証と実際の待受が乖離しない)。
func ensureLoopbackAddr(ctx context.Context, addr string) error {
	reject := func(format string, args ...any) error {
		return apperrors.Newf(apperrors.CodeBadRequest,
			format+"。認証がないため非ループバック待受は全確定申告データをネットワークに露出します。"+
				"意図的に公開する場合は KAKUTEI_ALLOW_NONLOOPBACK=1 を設定し、"+
				"リバースプロキシ等で必ず認証を追加してください", args...)
	}
	// 構文検証はオプトインでも免除しない (免除されるのはループバック要件のみ)
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return apperrors.Newf(apperrors.CodeBadRequest,
			"待受アドレス %q を host:port として解釈できません", addr)
	}
	if os.Getenv("KAKUTEI_ALLOW_NONLOOPBACK") == "1" {
		return nil
	}
	// IP リテラル (IPv6 の zone・IPv4-mapped IPv6 も含めて判定)
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Unmap().IsLoopback() {
			return nil
		}
		return reject("待受アドレス %q はループバックではありません", addr)
	}
	// ホスト名: 解決結果の全アドレスがループバックであることを検証
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return reject("待受アドレス %q のホスト名を解決できません", addr)
	}
	for _, ip := range ips {
		if !ip.IP.IsLoopback() {
			return reject("待受アドレス %q は非ループバック (%s) に解決されます", addr, ip.IP)
		}
	}
	return nil
}

func (r *Registry) Router() http.Handler { return r.router }

func (r *Registry) Close() error {
	return r.db.Close()
}
