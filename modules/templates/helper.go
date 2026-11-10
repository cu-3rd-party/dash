package templates

import (
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	member_model "git.cu3rd.ru/cu3rd/dash/models/member"
	"git.cu3rd.ru/cu3rd/dash/modules/assets"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
)

func readPublic(path string) string {
	content, err := fs.ReadFile(assets.Public(), path)
	if err != nil {
		slog.Error("read asset", "path", path, "err", err)
		return ""
	}
	return string(content)
}

// favicon draws the logo as a data: URL; on a light browser tab it is dark.
func favicon(logo string) template.HTML {
	svg := strings.NewReplacer(`"`, "'", ` fill='#ddd'`, "", "#", "%23").Replace(logo)
	svg = strings.Replace(svg, "<path", "<style>path{fill:%23ddd}@media(prefers-color-scheme:light){path{fill:%23050505}}</style><path", 1)
	return template.HTML(`<link rel="icon" href="data:image/svg+xml,` + svg + `">`) //nolint:gosec // our own file
}

// AssetVersion busts browser caches of /assets between builds.
var AssetVersion = time.Now().Format("20060102150405")

var months = []string{"янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"}

// Plural picks the Russian word form for n: Plural(5, "день", "дня", "дней").
func Plural(n int, one, few, many string) string {
	n = int(math.Abs(float64(n)))
	switch {
	case n%10 == 1 && n%100 != 11:
		return one
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 10 || n%100 >= 20):
		return few
	}
	return many
}

// Date renders a stored date: "15 ноя", with the year when it is not the current one.
func Date(s string) string {
	t, ok := money.ParseDate(s)
	if !ok {
		return "—"
	}
	out := fmt.Sprintf("%d %s", t.Day(), months[t.Month()-1])
	if t.Year() != money.Today(time.Now()).Year() {
		out += fmt.Sprintf(" %d", t.Year())
	}
	return out
}

// Relative renders how far a date is from today: "через 5 дней", "2 дня назад".
func Relative(s string) string {
	d, ok := money.DaysUntil(s, time.Now())
	if !ok {
		return ""
	}
	switch {
	case d == 0:
		return "сегодня"
	case d == 1:
		return "завтра"
	case d == -1:
		return "вчера"
	case d > 0:
		return fmt.Sprintf("через %d %s", d, Plural(d, "день", "дня", "дней"))
	}
	return fmt.Sprintf("%d %s назад", -d, Plural(-d, "день", "дня", "дней"))
}

// Initial is the first letter for an avatar placeholder.
func Initial(s string) string {
	r, _ := utf8.DecodeRuneInString(strings.ToUpper(strings.TrimSpace(s)))
	if r == utf8.RuneError {
		return "?"
	}
	return string(r)
}

// NewFuncMap returns the functions available in templates.
func NewFuncMap() template.FuncMap {
	return template.FuncMap{
		"AssetURL":    func(path string) string { return setting.AppSubURL + "/assets/" + path + "?v=" + AssetVersion },
		"AppSubURL":   func() string { return setting.AppSubURL },
		"InlineLogo":  func() template.HTML { return template.HTML(readPublic("img/logo.svg")) }, //nolint:gosec // our own file
		"FaviconLink": func() template.HTML { return favicon(readPublic("img/logo.svg")) },
		// dict builds a map for passing several values to a sub-template:
		// {{template "admin/due" dict "Row" .}}
		"dict": func(kv ...any) (map[string]any, error) {
			if len(kv)%2 != 0 {
				return nil, errors.New("dict: odd number of arguments")
			}
			m := make(map[string]any, len(kv)/2)
			for i := 0; i < len(kv); i += 2 {
				k, ok := kv[i].(string)
				if !ok {
					return nil, errors.New("dict: keys must be strings")
				}
				m[k] = kv[i+1]
			}
			return m, nil
		},
		"list":    func(items ...string) []string { return items },
		"TimeMSK": func(unix int64) string { return time.Unix(unix, 0).In(money.Moscow).Format("02.01.2006 15:04") },

		"Money":      money.Format,
		"Rub":        func(v int64) string { return money.Format(v, money.Base) },
		"MoneyInput": money.Input,
		"Date":       Date,
		"Relative":   Relative,
		"Today":      func() string { return money.Today(time.Now()).Format(money.DateLayout) },
		"Plural":     Plural,
		"Add":        func(a, b int) int { return a + b },
		"Initial":    Initial,
		"Periods":    func() []money.Period { return money.Periods },
		"Period":     money.GetPeriod,
		"Currencies": func() []string { return money.Currencies },
		"Roles":      func() []member_model.RoleInfo { return member_model.Roles },
		"RoleLabel":  member_model.RoleLabel,
		"Percent": func(part, total int64) int {
			if total <= 0 {
				return 0
			}
			return min(100, int(math.Round(float64(part)*100/float64(total))))
		},
	}
}
