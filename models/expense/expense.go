// Package expense is an expense series (a server, a domain…) and its payments.
// The description belongs to the whole series; every payment has its own note.
package expense

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"git.cu3rd.ru/cu3rd/dash/models/db"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
)

// SoonDays is how close a due date has to be to need attention.
const SoonDays = 14

// Status of an expense relative to its next due date.
type Status string

// Statuses.
const (
	StatusOK       Status = "ok"
	StatusSoon     Status = "soon"
	StatusOverdue  Status = "overdue"
	StatusNoDate   Status = "nodate"
	StatusInactive Status = "inactive"
)

// Label is the Russian name of a status.
func (s Status) Label() string {
	switch s {
	case StatusSoon:
		return "скоро оплата"
	case StatusOverdue:
		return "просрочено"
	case StatusNoDate:
		return "без даты"
	case StatusInactive:
		return "архив"
	}
	return "оплачено"
}

// Expense is a recurring (or one-off) expense.
type Expense struct {
	ID          int64  `xorm:"pk autoincr"`
	CategoryID  int64  `xorm:"NOT NULL INDEX"`
	Name        string `xorm:"VARCHAR(200) NOT NULL"`
	Description string `xorm:"TEXT"`
	// Owner is who is responsible for paying: a Forgejo login or just a name.
	Owner    string `xorm:"VARCHAR(100) NOT NULL DEFAULT ''"`
	Provider string `xorm:"VARCHAR(200) NOT NULL DEFAULT ''"`
	URL      string `xorm:"VARCHAR(500) NOT NULL DEFAULT ''"`
	// Amount per Period, in minor units of Currency.
	Amount      int64  `xorm:"NOT NULL"`
	Currency    string `xorm:"VARCHAR(3) NOT NULL"`
	Period      string `xorm:"VARCHAR(16) NOT NULL"`
	NextDue     string `xorm:"VARCHAR(10) NOT NULL DEFAULT ''"`
	IsActive    bool   `xorm:"NOT NULL DEFAULT true"`
	IsPublic    bool   `xorm:"NOT NULL DEFAULT true"`
	CreatedUnix int64  `xorm:"created"`
	UpdatedUnix int64  `xorm:"updated"`

	CategoryName string   `xorm:"-"`
	LastPayment  *Payment `xorm:"-"`
	PaymentCount int      `xorm:"-"`
}

func init() {
	db.RegisterModel(new(Expense))
	db.RegisterModel(new(Payment))
}

// ErrExpenseNotExist is returned for an unknown id.
type ErrExpenseNotExist struct{ ID int64 }

func (err ErrExpenseNotExist) Error() string {
	return fmt.Sprintf("статья расхода #%d не найдена", err.ID)
}
func (err ErrExpenseNotExist) Unwrap() error { return db.ErrNotExist }

// ErrInvalidExpense lists what is wrong with the input.
type ErrInvalidExpense struct{ Reasons []string }

func (err ErrInvalidExpense) Error() string { return strings.Join(err.Reasons, "; ") }
func (err ErrInvalidExpense) Unwrap() error { return db.ErrInvalidArgument }

// Validate normalizes and checks the fields.
func (e *Expense) Validate() error {
	e.Name = strings.TrimSpace(e.Name)
	e.Description = strings.TrimSpace(e.Description)
	e.Owner = strings.TrimSpace(e.Owner)
	e.Provider = strings.TrimSpace(e.Provider)
	e.URL = strings.TrimSpace(e.URL)
	e.NextDue = strings.TrimSpace(e.NextDue)
	var errs []string
	if e.Name == "" {
		errs = append(errs, "укажите название")
	}
	if utf8.RuneCountInString(e.Name) > 200 || utf8.RuneCountInString(e.Owner) > 100 || utf8.RuneCountInString(e.Provider) > 200 || len(e.URL) > 500 {
		errs = append(errs, "слишком длинное значение в одном из полей")
	}
	if e.CategoryID <= 0 {
		errs = append(errs, "выберите категорию")
	}
	if e.Amount <= 0 {
		errs = append(errs, "стоимость должна быть больше нуля")
	}
	if !money.ValidCurrency(e.Currency) {
		errs = append(errs, "неизвестная валюта")
	}
	if !money.ValidPeriod(e.Period) {
		errs = append(errs, "неизвестный период")
	}
	if e.NextDue != "" {
		if _, ok := money.ParseDate(e.NextDue); !ok {
			errs = append(errs, "дата следующей оплаты в формате ГГГГ-ММ-ДД")
		}
	}
	if e.URL != "" {
		if u, err := url.Parse(e.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, "ссылка должна начинаться с http:// или https://")
		}
	}
	if len(errs) > 0 {
		return ErrInvalidExpense{Reasons: errs}
	}
	return nil
}

// Status is the state of the expense on now.
func (e *Expense) Status(now time.Time) Status {
	if !e.IsActive {
		return StatusInactive
	}
	days, ok := money.DaysUntil(e.NextDue, now)
	switch {
	case !ok:
		return StatusNoDate
	case days < 0:
		return StatusOverdue
	case days <= SoonDays:
		return StatusSoon
	}
	return StatusOK
}

// PeriodInfo describes the billing period.
func (e *Expense) PeriodInfo() money.Period { return money.GetPeriod(e.Period) }

// Get returns an expense with its category name and last payment.
func Get(ctx context.Context, id int64) (*Expense, error) {
	e := new(Expense)
	has, err := db.GetEngine(ctx).ID(id).Get(e)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrExpenseNotExist{ID: id}
	}
	if err := loadAttributes(ctx, []*Expense{e}); err != nil {
		return nil, err
	}
	return e, nil
}

// List returns all expenses by category order, active first, then by name.
func List(ctx context.Context) ([]*Expense, error) {
	list := make([]*Expense, 0, 32)
	err := db.GetEngine(ctx).Table("expense").
		Join("INNER", "category", "category.id = expense.category_id").
		Asc("category.sort_order", "category.name").Desc("expense.is_active").Asc("expense.name").
		Select("expense.*").Find(&list)
	if err != nil {
		return nil, err
	}
	return list, loadAttributes(ctx, list)
}

func loadAttributes(ctx context.Context, list []*Expense) error {
	if len(list) == 0 {
		return nil
	}
	type cat struct {
		ID   int64
		Name string
	}
	var cats []cat
	if err := db.GetEngine(ctx).Table("category").Cols("id", "name").Find(&cats); err != nil {
		return err
	}
	names := map[int64]string{}
	for _, c := range cats {
		names[c.ID] = c.Name
	}
	ids := make([]int64, 0, len(list))
	for _, e := range list {
		e.CategoryName = names[e.CategoryID]
		ids = append(ids, e.ID)
	}
	payments := make([]*Payment, 0, len(list))
	if err := db.GetEngine(ctx).In("expense_id", ids).Asc("paid_on", "id").Find(&payments); err != nil {
		return err
	}
	byID := map[int64]*Expense{}
	for _, e := range list {
		byID[e.ID] = e
	}
	for _, p := range payments {
		e := byID[p.ExpenseID]
		e.PaymentCount++
		e.LastPayment = p // ordered ascending: the last one wins
	}
	return nil
}

// Create inserts an expense.
func Create(ctx context.Context, e *Expense) error {
	if err := e.Validate(); err != nil {
		return err
	}
	_, err := db.GetEngine(ctx).Insert(e)
	return err
}

// Update saves the editable fields.
func Update(ctx context.Context, e *Expense) error {
	if err := e.Validate(); err != nil {
		return err
	}
	_, err := db.GetEngine(ctx).ID(e.ID).
		Cols("category_id", "name", "description", "owner", "provider", "url", "amount", "currency", "period", "next_due", "is_active", "is_public").
		Update(e)
	return err
}

// SetNextDue changes only the due date.
func SetNextDue(ctx context.Context, id int64, due string) error {
	_, err := db.GetEngine(ctx).ID(id).Cols("next_due").Update(&Expense{NextDue: due})
	return err
}

// Delete removes the expense and all its payments.
func Delete(ctx context.Context, id int64) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		if _, err := db.GetEngine(ctx).Where("expense_id = ?", id).Delete(new(Payment)); err != nil {
			return err
		}
		_, err := db.GetEngine(ctx).ID(id).Delete(new(Expense))
		return err
	})
}
