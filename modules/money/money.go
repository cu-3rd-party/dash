// Package money parses and formats amounts (kept in minor units: kopecks, cents),
// converts them to roubles and computes billing dates.
package money

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Base is the currency all totals are reported in.
const Base = "RUB"

// Currencies are the accepted currency codes.
var Currencies = []string{"RUB", "USD", "EUR"}

var symbols = map[string]string{"RUB": "₽", "USD": "$", "EUR": "€"}

// ValidCurrency reports whether c is accepted.
func ValidCurrency(c string) bool { return slices.Contains(Currencies, c) }

// Parse reads "1 234,56", "1234.56" or "1234" into minor units.
func Parse(s string) (int64, error) {
	s = strings.NewReplacer(" ", "", " ", "", " ", "", "_", "").Replace(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, ",", ".")
	if s == "" {
		return 0, errors.New("сумма не указана")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("не похоже на сумму: %q", s)
	}
	if f < 0 {
		return 0, errors.New("сумма не может быть отрицательной")
	}
	if f > 1e12 {
		return 0, errors.New("слишком большая сумма")
	}
	return int64(math.Round(f * 100)), nil
}

// Input is the value for a form field.
func Input(v int64) string {
	if v == 0 {
		return ""
	}
	if v%100 == 0 {
		return strconv.FormatInt(v/100, 10)
	}
	return fmt.Sprintf("%d.%02d", v/100, v%100)
}

func groupThousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Format renders minor units with the currency sign: "1 234,56 ₽".
func Format(v int64, currency string) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := groupThousands(v / 100)
	if v%100 != 0 {
		s += fmt.Sprintf(",%02d", v%100)
	}
	if neg {
		s = "−" + s
	}
	sym := symbols[currency]
	if sym == "" {
		sym = currency
	}
	return s + " " + sym
}

// Rates is how many roubles one unit of a currency costs.
type Rates map[string]float64

// ToBase converts minor units of currency into rouble kopecks; unknown rates give 0.
func (r Rates) ToBase(amount int64, currency string) int64 {
	if currency == Base || currency == "" {
		return amount
	}
	rate, ok := r[currency]
	if !ok || rate <= 0 {
		return 0
	}
	return int64(math.Round(float64(amount) * rate))
}

// Period is how often an expense is billed.
type Period struct {
	Key    string
	Label  string
	Short  string
	Months int
}

// Periods are the billing periods offered in forms.
var Periods = []Period{
	{"month", "Ежемесячно", "мес", 1},
	{"quarter", "Ежеквартально", "квартал", 3},
	{"year", "Ежегодно", "год", 12},
	{"once", "Разово", "разово", 0},
}

// GetPeriod returns the period by key (monthly for unknown keys).
func GetPeriod(key string) Period {
	for _, p := range Periods {
		if p.Key == key {
			return p
		}
	}
	return Periods[0]
}

// ValidPeriod reports whether key is a known period.
func ValidPeriod(key string) bool {
	for _, p := range Periods {
		if p.Key == key {
			return true
		}
	}
	return false
}

// Monthly is what amount per period costs per month (0 for one-off).
func Monthly(amount int64, period string) int64 {
	m := GetPeriod(period).Months
	if m == 0 {
		return 0
	}
	return int64(math.Round(float64(amount) / float64(m)))
}

// Yearly is what amount per period costs per year (0 for one-off).
func Yearly(amount int64, period string) int64 {
	m := GetPeriod(period).Months
	if m == 0 {
		return 0
	}
	return int64(math.Round(float64(amount) * 12 / float64(m)))
}

// DateLayout is how calendar dates are stored: billing happens on days, not instants.
const DateLayout = "2006-01-02"

// Moscow is the time zone of the team.
var Moscow = time.FixedZone("MSK", 3*60*60)

// ParseDate parses a stored date.
func ParseDate(s string) (time.Time, bool) {
	t, err := time.ParseInLocation(DateLayout, s, Moscow)
	return t, err == nil
}

// Today is the current date in Moscow.
func Today(now time.Time) time.Time {
	n := now.In(Moscow)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, Moscow)
}

// DaysUntil is the number of days from today to date (negative when past).
func DaysUntil(date string, now time.Time) (int, bool) {
	d, ok := ParseDate(date)
	if !ok {
		return 0, false
	}
	return int(math.Round(d.Sub(Today(now)).Hours() / 24)), true
}

// AdvanceDue is the next due date after a payment that covers `periods` periods.
// It counts from the current due date, so paying a few days early or late keeps
// the billing day; a long-lapsed expense restarts from the payment date.
func AdvanceDue(currentDue, paidOn, period string, periods int) string {
	months := GetPeriod(period).Months
	if months == 0 {
		return ""
	}
	if periods < 1 {
		periods = 1
	}
	paid, ok := ParseDate(paidOn)
	if !ok {
		return currentDue
	}
	base, ok := ParseDate(currentDue)
	if !ok || base.AddDate(0, months*periods, 0).Before(paid) {
		base = paid
	}
	return base.AddDate(0, months*periods, 0).Format(DateLayout)
}

// FunnyAmounts are whole-rouble amounts that are not accepted as donations.
var FunnyAmounts = []int64{67, 69, 228, 420, 666, 1337, 1488, 6767, 6969, 69420}

// Funny reports whether a donation of amount (minor units) is one of FunnyAmounts.
// Kopecks do not help: only the whole roubles count.
func Funny(amount int64) bool { return slices.Contains(FunnyAmounts, amount/100) }
