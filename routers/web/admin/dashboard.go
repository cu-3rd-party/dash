// Package admin holds the handlers of the admin pages (/admin). Viewers read,
// editors change expenses, payments, donations and categories, owners also manage
// members and settings; the role checks are in routers/web.
package admin

import (
	"net/http"
	"time"

	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	expense_model "git.cu3rd.ru/cu3rd/dash/models/expense"
	sitesetting_model "git.cu3rd.ru/cu3rd/dash/models/sitesetting"
	"git.cu3rd.ru/cu3rd/dash/services/context"
	finance_service "git.cu3rd.ru/cu3rd/dash/services/finance"
)

const (
	recentPayments  = 8
	recentDonations = 6
)

func page(ctx *context.Context, tab, title string) {
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["Tab"] = tab
	ctx.Data["Title"] = title
}

// Dashboard is the overview: GET /admin
func Dashboard(ctx *context.Context) {
	c := ctx.Req.Context()
	now := time.Now()
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
	payments, err := expense_model.ListPayments(c, 0)
	if err != nil {
		ctx.ServerError("ListPayments", err)
		return
	}
	donations, err := donation_model.List(c, false)
	if err != nil {
		ctx.ServerError("List donations", err)
		return
	}
	ctx.Data["Summary"] = finance_service.Summarize(finance_service.Rows(expenses, rates, now), payments, donations, rates, now)
	ctx.Data["Payments"] = payments[:min(len(payments), recentPayments)]
	ctx.Data["Donations"] = donations[:min(len(donations), recentDonations)]
	page(ctx, "dashboard", "Обзор")
	ctx.HTML(http.StatusOK, "admin/dashboard")
}
