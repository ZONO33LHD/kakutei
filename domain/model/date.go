package model

import (
	"fmt"
	"time"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
)

// Date は時刻を持たない暦日 (JST 前提の申告書上の日付)。
// タイムゾーンによる日付ずれを避けるため time.Time ではなく専用型で扱う。
type Date struct {
	year  int
	month time.Month
	day   int
}

// NewDate は年月日から Date を作る。実在しない日付はエラー。
func NewDate(year int, month time.Month, day int) (Date, error) {
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if t.Year() != year || t.Month() != month || t.Day() != day {
		return Date{}, apperrors.Newf(apperrors.CodeBadRequest,
			"実在しない日付です: %04d-%02d-%02d", year, int(month), day)
	}
	return Date{year: year, month: month, day: day}, nil
}

// ParseDate は "YYYY-MM-DD" 形式の文字列を Date に変換する。
func ParseDate(s string) (Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return Date{}, apperrors.Newf(apperrors.CodeBadRequest,
			"日付は YYYY-MM-DD 形式で指定してください: %q", s)
	}
	return Date{year: t.Year(), month: t.Month(), day: t.Day()}, nil
}

// Year は年を返す。
func (d Date) Year() int { return d.year }

// Month は月を返す。
func (d Date) Month() time.Month { return d.month }

// Day は日を返す。
func (d Date) Day() int { return d.day }

// IsZero はゼロ値 (未設定) かどうか。
func (d Date) IsZero() bool { return d == Date{} }

// String は "YYYY-MM-DD" 形式で返す。ゼロ値は空文字列。
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.year, int(d.month), d.day)
}

// Before は d が other より前の日付かどうか。
func (d Date) Before(other Date) bool {
	if d.year != other.year {
		return d.year < other.year
	}
	if d.month != other.month {
		return d.month < other.month
	}
	return d.day < other.day
}

// After は d が other より後の日付かどうか。
func (d Date) After(other Date) bool { return other.Before(d) }

// AgeAt は d を生年月日として at 時点の満年齢 (誕生日当日に加齢する通俗的な数え方) を返す。
// 税法上の年齢判定には TaxAgeAt を使うこと。
func (d Date) AgeAt(at Date) int {
	age := at.year - d.year
	if at.month < d.month || (at.month == d.month && at.day < d.day) {
		age--
	}
	return age
}

// TaxAgeAt は税法上の満年齢を返す。
//
// 年齢計算ニ関スル法律・民法143条により、法律上は誕生日の「前日」の満了時に加齢する。
// 例: 1961-01-01 生まれは 2025-12-31 時点で 65 歳 (国税庁の年齢区分表と一致)。
// 扶養控除・老人控除対象配偶者・公的年金等控除の年齢判定はこちらを使う。
func (d Date) TaxAgeAt(at Date) int {
	// 基準日の翌日を通俗的な満年齢で評価すると「前日加齢」と同値になる。
	next := time.Date(at.year, at.month, at.day, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	return d.AgeAt(Date{year: next.Year(), month: next.Month(), day: next.Day()})
}

// MarshalJSON は "YYYY-MM-DD" の JSON 文字列にする。
func (d Date) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.String() + `"`), nil
}

// UnmarshalJSON は "YYYY-MM-DD" の JSON 文字列から復元する。
func (d *Date) UnmarshalJSON(b []byte) error {
	if len(b) < 2 || b[0] != '"' || b[len(b)-1] != '"' {
		return apperrors.Newf(apperrors.CodeBadRequest, "日付は文字列で指定してください: %s", string(b))
	}
	s := string(b[1 : len(b)-1])
	if s == "" {
		*d = Date{}
		return nil
	}
	parsed, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
