package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	audit_model "git.cu3rd.ru/cu3rd/dash/models/audit"
	category_model "git.cu3rd.ru/cu3rd/dash/models/category"
	expense_model "git.cu3rd.ru/cu3rd/dash/models/expense"
	member_model "git.cu3rd.ru/cu3rd/dash/models/member"
	sitesetting_model "git.cu3rd.ru/cu3rd/dash/models/sitesetting"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
	"git.cu3rd.ru/cu3rd/dash/services/context"
	finance_service "git.cu3rd.ru/cu3rd/dash/services/finance"
)

const tplExpenseForm = "admin/expense_form"

// Expenses lists expenses with filters: GET /admin/expenses?cat=&status=&owner=
func Expenses(ctx *context.Context) {
	c := ctx.Req.Context()
	rates, err := sitesetting_model.Rates(c)
	if err != nil {
		ctx.ServerError("Rates", err)
		return
	}
	expenses, err := expense_model.List(c)
	if err != nil {
		ctx.ServerError("List expenses", err)
		return
	}
	cats, err := category_model.List(c)
	if err != nil {
		ctx.ServerError("List categories", err)
		return
	}
	q := ctx.Req.URL.Query()
	cat, _ := strconv.ParseInt(q.Get("cat"), 10, 64)
	status, owner := q.Get("status"), q.Get("owner")

	all := finance_service.Rows(expenses, rates, time.Now())
	rows := make([]*finance_service.Row, 0, len(all))
	var owners []string
	seen := map[string]bool{}
	for _, r := range all {
		if r.Owner != "" && !seen[r.Owner] {
			seen[r.Owner] = true
			owners = append(owners, r.Owner)
		}
		if (cat != 0 && r.CategoryID != cat) || (owner != "" && r.Owner != owner) || !matchStatus(r, status) {
			continue
		}
		rows = append(rows, r)
	}
	groups := finance_service.GroupRows(rows)
	var monthly int64
	for _, g := range groups {
		monthly += g.Monthly
	}
	ctx.Data["Groups"] = groups
	ctx.Data["Count"] = len(rows)
	ctx.Data["Monthly"] = monthly
	ctx.Data["Categories"] = cats
	ctx.Data["Owners"] = owners
	ctx.Data["Cat"] = cat
	ctx.Data["Status"] = status
	ctx.Data["Owner"] = owner
	page(ctx, "expenses", "Расходы")
	ctx.HTML(http.StatusOK, "admin/expenses")
}

func matchStatus(r *finance_service.Row, status string) bool {
	switch status {
	case "":
		return r.IsActive
	case "all":
		return true
	case "attention":
		return r.Status == expense_model.StatusOverdue || r.Status == expense_model.StatusSoon || r.Status == expense_model.StatusNoDate
	}
	return string(r.Status) == status
}

func loadExpense(ctx *context.Context) *expense_model.Expense {
	e, err := expense_model.Get(ctx.Req.Context(), ctx.PathParamInt64("id"))
	if err != nil {
		ctx.NotFoundOrServerError("Get expense", err)
		return nil
	}
	return e
}

// Expense shows an expense and its payments: GET /admin/expenses/{id}
func Expense(ctx *context.Context) {
	e := loadExpense(ctx)
	if e == nil {
		return
	}
	c := ctx.Req.Context()
	rates, err := sitesetting_model.Rates(c)
	if err != nil {
		ctx.ServerError("Rates", err)
		return
	}
	payments, err := expense_model.ListPayments(c, e.ID)
	if err != nil {
		ctx.ServerError("ListPayments", err)
		return
	}
	var paid int64
	for _, p := range payments {
		paid += rates.ToBase(p.Amount, p.Currency)
	}
	ctx.Data["Row"] = finance_service.Rows([]*expense_model.Expense{e}, rates, time.Now())[0]
	ctx.Data["Payments"] = payments
	ctx.Data["Paid"] = paid
	page(ctx, "expenses", e.Name)
	ctx.HTML(http.StatusOK, "admin/expense")
}

func renderExpenseForm(ctx *context.Context, status int, e *expense_model.Expense, amount string, errs []string) {
	c := ctx.Req.Context()
	cats, err := category_model.List(c)
	if err != nil {
		ctx.ServerError("List categories", err)
		return
	}
	members, err := member_model.List(c)
	if err != nil {
		ctx.ServerError("List members", err)
		return
	}
	title := "Новая статья расхода"
	if e.ID != 0 {
		title = "Редактирование: " + e.Name
	}
	ctx.Data["Expense"] = e
	ctx.Data["AmountInput"] = amount
	ctx.Data["Errors"] = errs
	ctx.Data["Categories"] = cats
	ctx.Data["Members"] = members
	page(ctx, "expenses", title)
	ctx.HTML(status, tplExpenseForm)
}

// NewExpense renders the empty form: GET /admin/expenses/new
func NewExpense(ctx *context.Context) {
	e := &expense_model.Expense{Currency: money.Base, Period: "month", IsActive: true, IsPublic: true, Owner: ctx.DoerLogin()}
	e.CategoryID, _ = strconv.ParseInt(ctx.Req.URL.Query().Get("cat"), 10, 64)
	renderExpenseForm(ctx, http.StatusOK, e, "", nil)
}

// readExpense fills e from the form; the amount is parsed separately so the field keeps what was typed.
func readExpense(ctx *context.Context, e *expense_model.Expense) (string, []string) {
	e.CategoryID = ctx.FormInt64("category_id")
	e.Name = ctx.FormString("name")
	e.Description = ctx.FormString("description")
	e.Owner = ctx.FormString("owner")
	e.Provider = ctx.FormString("provider")
	e.URL = ctx.FormString("url")
	e.Currency = ctx.FormString("currency")
	e.Period = ctx.FormString("period")
	e.NextDue = ctx.FormString("next_due")
	e.IsActive = ctx.FormBool("is_active")
	e.IsPublic = ctx.FormBool("is_public")
	amount := ctx.FormString("amount")
	v, err := money.Parse(amount)
	if err != nil {
		e.Amount = 0
		return amount, []string{"Стоимость: " + err.Error()}
	}
	e.Amount = v
	return amount, nil
}

func saveExpense(ctx *context.Context, e *expense_model.Expense, save func() error) bool {
	amount, errs := readExpense(ctx, e)
	if len(errs) == 0 {
		err := save()
		var invalid expense_model.ErrInvalidExpense
		switch {
		case errors.As(err, &invalid):
			errs = invalid.Reasons
		case err != nil:
			ctx.ServerError("save expense", err)
			return false
		}
	}
	if len(errs) > 0 {
		renderExpenseForm(ctx, http.StatusUnprocessableEntity, e, amount, errs)
		return false
	}
	return true
}

// CreateExpense handles POST /admin/expenses/new
func CreateExpense(ctx *context.Context) {
	e := new(expense_model.Expense)
	if !saveExpense(ctx, e, func() error { return expense_model.Create(ctx.Req.Context(), e) }) {
		return
	}
	audit_model.Record(ctx.Req.Context(), ctx.DoerLogin(), "expense.create",
		fmt.Sprintf("#%d %s — %s/%s", e.ID, e.Name, money.Format(e.Amount, e.Currency), e.PeriodInfo().Short))
	ctx.RedirectWithFlash(fmt.Sprintf("/admin/expenses/%d", e.ID), "Статья расхода создана")
}

// EditExpense renders the form: GET /admin/expenses/{id}/edit
func EditExpense(ctx *context.Context) {
	if e := loadExpense(ctx); e != nil {
		renderExpenseForm(ctx, http.StatusOK, e, money.Input(e.Amount), nil)
	}
}

// UpdateExpense handles POST /admin/expenses/{id}/edit
func UpdateExpense(ctx *context.Context) {
	e := loadExpense(ctx)
	if e == nil || !saveExpense(ctx, e, func() error { return expense_model.Update(ctx.Req.Context(), e) }) {
		return
	}
	audit_model.Record(ctx.Req.Context(), ctx.DoerLogin(), "expense.update", fmt.Sprintf("#%d %s", e.ID, e.Name))
	ctx.RedirectWithFlash(fmt.Sprintf("/admin/expenses/%d", e.ID), "Сохранено")
}

// DeleteExpense removes an expense with its payments when the name is typed in: POST /admin/expenses/{id}/delete
func DeleteExpense(ctx *context.Context) {
	e := loadExpense(ctx)
	if e == nil {
		return
	}
	if ctx.FormString("confirm") != e.Name {
		ctx.PlainText(http.StatusUnprocessableEntity, "Для удаления введите точное название статьи")
		return
	}
	if err := expense_model.Delete(ctx.Req.Context(), e.ID); err != nil {
		ctx.ServerError("Delete expense", err)
		return
	}
	audit_model.Record(ctx.Req.Context(), ctx.DoerLogin(), "expense.delete", fmt.Sprintf("#%d %s (оплат: %d)", e.ID, e.Name, e.PaymentCount))
	ctx.RedirectWithFlash("/admin/expenses", "Статья «"+e.Name+"» удалена вместе с историей оплат")
}
