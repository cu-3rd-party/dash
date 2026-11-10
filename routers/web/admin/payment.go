package admin

import (
	"fmt"
	"net/http"
	"strconv"

	audit_model "git.cu3rd.ru/cu3rd/dash/models/audit"
	expense_model "git.cu3rd.ru/cu3rd/dash/models/expense"
	sitesetting_model "git.cu3rd.ru/cu3rd/dash/models/sitesetting"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
	"git.cu3rd.ru/cu3rd/dash/modules/templates"
	"git.cu3rd.ru/cu3rd/dash/services/context"
	finance_service "git.cu3rd.ru/cu3rd/dash/services/finance"
)

// Payments lists all payments: GET /admin/payments
func Payments(ctx *context.Context) {
	c := ctx.Req.Context()
	payments, err := expense_model.ListPayments(c, 0)
	if err != nil {
		ctx.ServerError("ListPayments", err)
		return
	}
	rates, err := sitesetting_model.Rates(c)
	if err != nil {
		ctx.ServerError("Rates", err)
		return
	}
	var total int64
	for _, p := range payments {
		total += rates.ToBase(p.Amount, p.Currency)
	}
	ctx.Data["Payments"] = payments
	ctx.Data["Total"] = total
	page(ctx, "payments", "Оплаты")
	ctx.HTML(http.StatusOK, "admin/payments")
}

// CreatePayment records a payment and moves the due date: POST /admin/expenses/{id}/payments
func CreatePayment(ctx *context.Context) {
	e := loadExpense(ctx)
	if e == nil {
		return
	}
	amount, err := money.Parse(ctx.FormString("amount"))
	if err != nil {
		ctx.PlainText(http.StatusUnprocessableEntity, "Сумма оплаты: "+err.Error())
		return
	}
	periods, _ := strconv.Atoi(ctx.FormString("periods"))
	p := &expense_model.Payment{
		PaidOn:    ctx.FormString("paid_on"),
		Amount:    amount,
		Currency:  ctx.FormString("currency"),
		Periods:   periods,
		Note:      ctx.FormString("note"),
		CreatedBy: ctx.DoerLogin(),
	}
	next, err := finance_service.RecordPayment(ctx.Req.Context(), e, p, ctx.FormBool("advance"))
	if err != nil {
		ctx.NotFoundOrServerError("RecordPayment", err)
		return
	}
	audit_model.Record(ctx.Req.Context(), ctx.DoerLogin(), "payment.create",
		fmt.Sprintf("%s: %s от %s", e.Name, money.Format(p.Amount, p.Currency), p.PaidOn))
	msg := "Оплата записана"
	if next != e.NextDue && next != "" {
		msg += ", следующая — " + templates.Date(next)
	}
	ctx.RedirectWithFlash(fmt.Sprintf("/admin/expenses/%d", e.ID), msg)
}

// DeletePayment handles POST /admin/payments/{id}/delete
func DeletePayment(ctx *context.Context) {
	c := ctx.Req.Context()
	p, err := expense_model.GetPayment(c, ctx.PathParamInt64("id"))
	if err != nil {
		ctx.NotFoundOrServerError("GetPayment", err)
		return
	}
	if err := expense_model.DeletePayment(c, p.ID); err != nil {
		ctx.ServerError("DeletePayment", err)
		return
	}
	audit_model.Record(c, ctx.DoerLogin(), "payment.delete", fmt.Sprintf("expense #%d: %s от %s", p.ExpenseID, money.Format(p.Amount, p.Currency), p.PaidOn))
	ctx.RedirectWithFlash(fmt.Sprintf("/admin/expenses/%d", p.ExpenseID), "Оплата удалена. Срок следующей оплаты не менялся — поправьте его вручную, если нужно.")
}
