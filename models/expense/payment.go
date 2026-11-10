package expense

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"git.cu3rd.ru/cu3rd/dash/models/db"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
)

// Payment is one actual payment of an expense.
type Payment struct {
	ID        int64  `xorm:"pk autoincr"`
	ExpenseID int64  `xorm:"NOT NULL INDEX"`
	PaidOn    string `xorm:"VARCHAR(10) NOT NULL INDEX"`
	Amount    int64  `xorm:"NOT NULL"`
	Currency  string `xorm:"VARCHAR(3) NOT NULL"`
	// Periods is how many billing periods the payment covers (a domain for 2 years: 2).
	Periods     int    `xorm:"NOT NULL DEFAULT 1"`
	Note        string `xorm:"VARCHAR(500) NOT NULL DEFAULT ''"`
	CreatedBy   string `xorm:"VARCHAR(40) NOT NULL DEFAULT ''"`
	CreatedUnix int64  `xorm:"created"`

	ExpenseName string `xorm:"-"`
}

// ErrPaymentNotExist is returned for an unknown id.
type ErrPaymentNotExist struct{ ID int64 }

func (err ErrPaymentNotExist) Error() string {
	return fmt.Sprintf("оплата #%d не найдена", err.ID)
}
func (err ErrPaymentNotExist) Unwrap() error { return db.ErrNotExist }

// Validate normalizes and checks the fields.
func (p *Payment) Validate() error {
	p.Note = strings.TrimSpace(p.Note)
	var errs []string
	if p.Amount <= 0 {
		errs = append(errs, "сумма оплаты должна быть больше нуля")
	}
	if !money.ValidCurrency(p.Currency) {
		errs = append(errs, "неизвестная валюта")
	}
	if _, ok := money.ParseDate(p.PaidOn); !ok {
		errs = append(errs, "дата оплаты в формате ГГГГ-ММ-ДД")
	}
	if p.Periods < 1 || p.Periods > 120 {
		errs = append(errs, "число периодов от 1 до 120")
	}
	if utf8.RuneCountInString(p.Note) > 500 {
		errs = append(errs, "комментарий длиннее 500 символов")
	}
	if len(errs) > 0 {
		return ErrInvalidExpense{Reasons: errs}
	}
	return nil
}

// CreatePayment inserts a payment.
func CreatePayment(ctx context.Context, p *Payment) error {
	if err := p.Validate(); err != nil {
		return err
	}
	_, err := db.GetEngine(ctx).Insert(p)
	return err
}

// GetPayment returns a payment.
func GetPayment(ctx context.Context, id int64) (*Payment, error) {
	p := new(Payment)
	has, err := db.GetEngine(ctx).ID(id).Get(p)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrPaymentNotExist{ID: id}
	}
	return p, nil
}

// ListPayments returns the payments of one expense (expenseID > 0) or of all, newest first.
func ListPayments(ctx context.Context, expenseID int64) ([]*Payment, error) {
	payments := make([]*Payment, 0, 16)
	sess := db.GetEngine(ctx).Desc("paid_on", "id")
	if expenseID > 0 {
		sess = sess.Where("expense_id = ?", expenseID)
	}
	if err := sess.Find(&payments); err != nil {
		return nil, err
	}
	type row struct {
		ID   int64
		Name string
	}
	var rows []row
	if err := db.GetEngine(ctx).Table("expense").Cols("id", "name").Find(&rows); err != nil {
		return nil, err
	}
	names := map[int64]string{}
	for _, r := range rows {
		names[r.ID] = r.Name
	}
	for _, p := range payments {
		p.ExpenseName = names[p.ExpenseID]
	}
	return payments, nil
}

// DeletePayment removes a payment.
func DeletePayment(ctx context.Context, id int64) error {
	_, err := db.GetEngine(ctx).ID(id).Delete(new(Payment))
	return err
}
