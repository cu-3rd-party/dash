package expense_test

import (
	"testing"
	"time"

	"git.cu3rd.ru/cu3rd/dash/models/db"
	expense_model "git.cu3rd.ru/cu3rd/dash/models/expense"
	"git.cu3rd.ru/cu3rd/dash/models/unittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList(t *testing.T) {
	unittest.PrepareTestDatabase(t)

	list, err := expense_model.List(t.Context())
	require.NoError(t, err)
	require.Len(t, list, 5)
	names := make([]string, 0, len(list))
	for _, e := range list {
		names = append(names, e.Name)
	}
	assert.Equal(t, []string{"grm", "nxs", "secret-backup", "cu3rd.ru", "old-domain"}, names, "by category, then active first, then name")

	grm := list[0]
	assert.Equal(t, "Серверы", grm.CategoryName)
	assert.Equal(t, 2, grm.PaymentCount)
	require.NotNil(t, grm.LastPayment)
	assert.Equal(t, "2026-10-15", grm.LastPayment.PaidOn)
	assert.Nil(t, list[1].LastPayment)
}

func TestStatus(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	for due, want := range map[string]expense_model.Status{
		"2026-11-09": expense_model.StatusOverdue,
		"2026-11-20": expense_model.StatusSoon,
		"2027-01-01": expense_model.StatusOK,
		"":           expense_model.StatusNoDate,
	} {
		e := &expense_model.Expense{IsActive: true, NextDue: due}
		assert.Equal(t, want, e.Status(now), due)
	}
	assert.Equal(t, expense_model.StatusInactive, (&expense_model.Expense{NextDue: "2020-01-01"}).Status(now))
}

func TestCreateValidates(t *testing.T) {
	unittest.PrepareTestDatabase(t)

	err := expense_model.Create(t.Context(), &expense_model.Expense{Name: " ", Currency: "GBP", Period: "week", URL: "ftp://x", NextDue: "15.11.2026"})
	require.ErrorIs(t, err, db.ErrInvalidArgument)
	var invalid expense_model.ErrInvalidExpense
	require.ErrorAs(t, err, &invalid)
	assert.Len(t, invalid.Reasons, 7)

	e := &expense_model.Expense{CategoryID: 3, Name: " Figma ", Amount: 1500, Currency: "USD", Period: "month", IsActive: true}
	require.NoError(t, expense_model.Create(t.Context(), e))
	got, err := expense_model.Get(t.Context(), e.ID)
	require.NoError(t, err)
	assert.Equal(t, "Figma", got.Name)
	assert.Equal(t, "Подписки", got.CategoryName)
	assert.True(t, got.IsActive)
	assert.False(t, got.IsPublic, "false booleans are stored as false")
}

func TestDeleteRemovesPayments(t *testing.T) {
	unittest.PrepareTestDatabase(t)

	require.NoError(t, expense_model.Delete(t.Context(), 1))
	unittest.AssertNotExistsBean(t, &expense_model.Expense{ID: 1})
	unittest.AssertCount(t, &expense_model.Payment{ExpenseID: 1}, 0)
	unittest.AssertCount(t, new(expense_model.Payment), 1)

	_, err := expense_model.Get(t.Context(), 1)
	require.ErrorIs(t, err, db.ErrNotExist)
}

func TestListPayments(t *testing.T) {
	unittest.PrepareTestDatabase(t)

	all, err := expense_model.ListPayments(t.Context(), 0)
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, "2026-10-15", all[0].PaidOn, "newest first")
	assert.Equal(t, "grm", all[0].ExpenseName)

	one, err := expense_model.ListPayments(t.Context(), 4)
	require.NoError(t, err)
	require.Len(t, one, 1)
	assert.Equal(t, "cu3rd.ru", one[0].ExpenseName)
}
