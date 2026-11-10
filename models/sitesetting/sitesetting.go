// Package sitesetting keeps settings edited in the admin pages: texts of the
// public page and currency rates.
package sitesetting

import (
	"context"
	"strconv"
	"strings"

	"git.cu3rd.ru/cu3rd/dash/models/db"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
)

// Keys of the settings.
const (
	KeySiteTitle   = "site_title"
	KeyPublicIntro = "public_intro"
	KeyDonateText  = "donate_text"
	KeyRatePrefix  = "rate_"
)

// SiteSetting is one key/value pair.
type SiteSetting struct {
	ID    int64  `xorm:"pk autoincr"`
	Name  string `xorm:"VARCHAR(64) NOT NULL UNIQUE"`
	Value string `xorm:"TEXT"`
}

func init() {
	db.RegisterModel(new(SiteSetting))
}

// ErrInvalidSetting is returned for bad input.
type ErrInvalidSetting struct{ Reason string }

func (err ErrInvalidSetting) Error() string { return err.Reason }
func (err ErrInvalidSetting) Unwrap() error { return db.ErrInvalidArgument }

// All returns every setting.
func All(ctx context.Context) (map[string]string, error) {
	var rows []*SiteSetting
	if err := db.GetEngine(ctx).Find(&rows); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Name] = r.Value
	}
	return out, nil
}

// Set stores values; rates must be positive numbers.
func Set(ctx context.Context, values map[string]string) error {
	for k, v := range values {
		v = strings.TrimSpace(v)
		if strings.HasPrefix(k, KeyRatePrefix) {
			f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
			if err != nil || f <= 0 || f > 1e6 {
				return ErrInvalidSetting{Reason: "курс " + strings.TrimPrefix(k, KeyRatePrefix) + " должен быть положительным числом"}
			}
			v = strconv.FormatFloat(f, 'f', -1, 64)
		}
		values[k] = v
	}
	return db.WithTx(ctx, func(ctx context.Context) error {
		for k, v := range values {
			n, err := db.GetEngine(ctx).Where("name = ?", k).Cols("value").Update(&SiteSetting{Value: v})
			if err != nil {
				return err
			}
			if n == 0 {
				if _, err := db.GetEngine(ctx).Insert(&SiteSetting{Name: k, Value: v}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Rates returns the currency rates to roubles.
func Rates(ctx context.Context) (money.Rates, error) {
	all, err := All(ctx)
	if err != nil {
		return nil, err
	}
	r := money.Rates{money.Base: 1}
	for _, c := range money.Currencies {
		if v, err := strconv.ParseFloat(all[KeyRatePrefix+c], 64); err == nil && v > 0 {
			r[c] = v
		}
	}
	return r, nil
}
