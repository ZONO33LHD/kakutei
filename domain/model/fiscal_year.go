package model

import (
	"time"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
)

// FiscalYear は課税年度 (暦年、例: 2025 = 令和7年分)。
// 個人の所得税は暦年課税のため 1/1〜12/31 を対象期間とする。
type FiscalYear int

const (
	// minFiscalYear / maxFiscalYear は入力ミス (例: 令和表記の 7、西暦 20250 等) を弾く境界。
	minFiscalYear = 2000
	maxFiscalYear = 2100
)

func (y FiscalYear) Validate() error {
	if y < minFiscalYear || y > maxFiscalYear {
		return apperrors.Newf(apperrors.CodeBadRequest,
			"課税年度は %d〜%d の西暦で指定してください: %d", minFiscalYear, maxFiscalYear, int(y))
	}
	return nil
}

func (y FiscalYear) Start() Date {
	return Date{year: int(y), month: time.January, day: 1}
}

// End は期末 (12月31日) を返す。年齢判定・扶養判定の基準日。
func (y FiscalYear) End() Date {
	return Date{year: int(y), month: time.December, day: 31}
}

func (y FiscalYear) Contains(d Date) bool {
	return d.Year() == int(y)
}
