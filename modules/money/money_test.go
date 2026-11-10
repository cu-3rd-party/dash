package money

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	for in, want := range map[string]int64{"1500": 150000, "1 234,56": 123456, "12.5": 1250, "0,01": 1, "44.90": 4490} {
		got, err := Parse(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", "abc", "-5", "1e13"} {
		_, err := Parse(bad)
		assert.Error(t, err, bad)
	}
}

func TestFormat(t *testing.T) {
	assert.Equal(t, "1 234 567 ₽", Format(123456700, "RUB"))
	assert.Equal(t, "44,90 €", Format(4490, "EUR"))
	assert.Equal(t, "−5 $", Format(-500, "USD"))
	assert.Equal(t, "44.90", Input(4490))
	assert.Equal(t, "1500", Input(150000))
}

func TestRatesAndPeriods(t *testing.T) {
	r := Rates{"EUR": 100}
	assert.Equal(t, int64(449000), r.ToBase(4490, "EUR"))
	assert.Equal(t, int64(0), r.ToBase(100, "USD"), "no rate, no guess")
	assert.Equal(t, int64(10000), Monthly(120000, "year"))
	assert.Equal(t, int64(120000), Yearly(120000, "year"))
	assert.Equal(t, int64(36000), Yearly(9000, "quarter"))
	assert.Equal(t, int64(0), Monthly(5000, "once"))
}

func TestAdvanceDue(t *testing.T) {
	cases := []struct {
		due, paid, period string
		n                 int
		want              string
	}{
		{"2026-11-15", "2026-11-10", "month", 1, "2026-12-15"}, // paid early: keeps the billing day
		{"2026-11-15", "2026-11-20", "month", 1, "2026-12-15"}, // a little late
		{"2025-01-01", "2026-11-10", "month", 1, "2026-12-10"}, // long lapsed: from the payment
		{"", "2026-11-10", "year", 1, "2027-11-10"},
		{"2026-11-15", "2026-11-10", "year", 2, "2028-11-15"},
		{"2026-11-15", "2026-11-10", "once", 1, ""},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, AdvanceDue(c.due, c.paid, c.period, c.n), "%+v", c)
	}
}

func TestDaysUntil(t *testing.T) {
	now := time.Date(2026, 11, 10, 22, 0, 0, 0, time.UTC) // 01:00 on Nov 11 in Moscow
	d, ok := DaysUntil("2026-11-12", now)
	require.True(t, ok)
	assert.Equal(t, 1, d)
	_, ok = DaysUntil("", now)
	assert.False(t, ok)
}

func TestFunny(t *testing.T) {
	for _, in := range []string{"67", "1488", "1488,99", "69.10"} {
		v, err := Parse(in)
		require.NoError(t, err, in)
		assert.True(t, Funny(v), in)
	}
	for _, in := range []string{"500", "100", "1000"} {
		v, _ := Parse(in)
		assert.False(t, Funny(v), in)
	}
}
