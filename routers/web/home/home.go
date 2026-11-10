// Package home is the public page: where the money goes and who donated.
package home

import (
	"net/http"
	"time"

	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	expense_model "git.cu3rd.ru/cu3rd/dash/models/expense"
	member_model "git.cu3rd.ru/cu3rd/dash/models/member"
	sitesetting_model "git.cu3rd.ru/cu3rd/dash/models/sitesetting"
	"git.cu3rd.ru/cu3rd/dash/services/context"
	finance_service "git.cu3rd.ru/cu3rd/dash/services/finance"
)

const publicDonationsLimit = 50

// Home renders the public page: GET /
func Home(ctx *context.Context) {
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
	settings, err := sitesetting_model.All(c)
	if err != nil {
		ctx.ServerError("Settings", err)
		return
	}

	admins, err := member_model.Admins(c)
	if err != nil {
		ctx.ServerError("Admins", err)
		return
	}
	contacts := make([]string, 0, len(admins))
	for _, a := range admins {
		contacts = append(contacts, a.TelegramHandle())
	}
	ctx.Data["Contacts"] = contacts

	rows := finance_service.Rows(expenses, rates, now)
	// totals include private expenses and donations: only their details are hidden
	ctx.Data["Summary"] = finance_service.Summarize(rows, payments, donations, rates, now)
	visible := make([]*finance_service.Row, 0, len(rows))
	for _, r := range rows {
		if r.IsActive && r.IsPublic {
			visible = append(visible, r)
		}
	}
	ctx.Data["Groups"] = finance_service.GroupRows(visible)
	public := make([]*donation_model.Donation, 0, publicDonationsLimit)
	for _, d := range donations {
		if d.IsPublic && len(public) < publicDonationsLimit {
			public = append(public, d)
		}
	}
	ctx.Data["Donations"] = public
	ctx.Data["Settings"] = settings
	ctx.Data["PageIsHome"] = true
	ctx.HTML(http.StatusOK, "home")
}

// NotFound renders the 404 page.
func NotFound(ctx *context.Context) {
	ctx.Data["Title"] = "Не найдено"
	ctx.Data["Message"] = "Такой страницы нет."
	ctx.HTML(http.StatusNotFound, "status")
}
