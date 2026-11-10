package finance_test

import (
	"testing"
	"time"

	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	expense_model "git.cu3rd.ru/cu3rd/dash/models/expense"
	"git.cu3rd.ru/cu3rd/dash/models/unittest"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
	finance_service "git.cu3rd.ru/cu3rd/dash/services/finance"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var rates = money.Rates{"RUB": 1, "USD": 90, "EUR": 100}

func TestSummarize(t *testing.T) {
	unittest.PrepareTestDatabase(t)
	ctx := t.Context()
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

	expenses, err := expense_model.List(ctx)
	require.NoError(t, err)
	payments, err := expense_model.ListPayments(ctx, 0)
	require.NoError(t, err)
	donations, err := donation_model.List(ctx, false)
	require.NoError(t, err)

	s := finance_service.Summarize(finance_service.Rows(expenses, rates, now), payments, donations, rates, now)
	// grm 44.90 € = 4490 ₽, nxs 1500 ₽, secret 300 ₽, cu3rd.ru 1200 ₽/год = 100 ₽; old-domain is archived
	assert.Equal(t, int64(449000+150000+30000+10000), s.MonthlyTotal)
	assert.Equal(t, int64((449000+150000+30000)*12+120000), s.YearlyTotal)
	assert.Equal(t, 4, s.ActiveCount)
	assert.Equal(t, 1, s.HiddenCount)
	assert.Equal(t, 1, s.OverdueCount)
	require.Len(t, s.Upcoming, 1, "only nxs: the others are due in 2099")
	assert.Equal(t, "nxs", s.Upcoming[0].Name)

	// 500 + 1000 + 300 + 10 $ × 90
	assert.Equal(t, int64(50000+100000+30000+90000), s.DonationsTotal)
	assert.Equal(t, int64(0), s.DonationsMonth)
	assert.Equal(t, 4, s.DonorsCount, "three named donors and one anonymous")
	assert.Equal(t, int64(449000*2+120000), s.PaidTotal)
	assert.Equal(t, s.DonationsTotal-s.PaidTotal, s.Balance)

	require.Len(t, s.ByCategory, 2)
	assert.Equal(t, "Серверы", s.ByCategory[0].Name)
	assert.Equal(t, 3, s.ByCategory[0].Count)
	assert.Equal(t, "boss", s.ByOwner[0].Name, "sorted by monthly cost")
}

func TestRecordPaymentMovesDueDate(t *testing.T) {
	unittest.PrepareTestDatabase(t)
	ctx := t.Context()

	e, err := expense_model.Get(ctx, 1)
	require.NoError(t, err)
	next, err := finance_service.RecordPayment(ctx, e, &expense_model.Payment{PaidOn: "2099-01-10", Amount: 4490, Currency: "EUR", Periods: 2, CreatedBy: "boss"}, true)
	require.NoError(t, err)
	assert.Equal(t, "2099-03-15", next)
	assert.Equal(t, "2099-03-15", unittest.AssertExistsAndLoadBean(t, &expense_model.Expense{ID: 1}).NextDue)
	unittest.AssertCount(t, &expense_model.Payment{ExpenseID: 1}, 3)

	// without advance the date stays
	e, err = expense_model.Get(ctx, 2)
	require.NoError(t, err)
	next, err = finance_service.RecordPayment(ctx, e, &expense_model.Payment{PaidOn: "2026-11-10", Amount: 150000, Currency: "RUB", Periods: 1}, false)
	require.NoError(t, err)
	assert.Equal(t, "2020-01-01", next)

	// an invalid payment leaves nothing behind
	_, err = finance_service.RecordPayment(ctx, e, &expense_model.Payment{PaidOn: "bad", Amount: 1, Currency: "RUB", Periods: 1}, true)
	require.Error(t, err)
	assert.Equal(t, "2020-01-01", unittest.AssertExistsAndLoadBean(t, &expense_model.Expense{ID: 2}).NextDue)
}

func TestSeedDemoOnlyOnEmptyDatabase(t *testing.T) {
	unittest.PrepareTestDatabase(t)
	require.NoError(t, finance_service.SeedDemo(t.Context()))
	unittest.AssertCount(t, new(expense_model.Expense), 5)
}
