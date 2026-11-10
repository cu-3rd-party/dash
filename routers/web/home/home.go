// Package home is the public page: where the money goes and who donated.
package home

import (
	"cmp"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	expense_model "git.cu3rd.ru/cu3rd/dash/models/expense"
	sitesetting_model "git.cu3rd.ru/cu3rd/dash/models/sitesetting"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
	"git.cu3rd.ru/cu3rd/dash/services/context"
	cryptobot_service "git.cu3rd.ru/cu3rd/dash/services/cryptobot"
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

	rows := finance_service.Rows(expenses, rates, now)
	// totals include private expenses and donations: only their details are hidden
	ctx.Data["Summary"] = finance_service.Summarize(rows, payments, donations, rates, now)
	visible := make([]*finance_service.Row, 0, len(rows))
	for _, r := range rows {
		if r.IsActive && r.IsPublic {
			visible = append(visible, r)
		}
	}
	groups := finance_service.GroupRows(visible)
	for _, g := range groups {
		// the most expensive first inside a category; equal ones keep their order
		slices.SortStableFunc(g.Rows, func(a, b *finance_service.Row) int { return cmp.Compare(b.Monthly, a.Monthly) })
	}
	ctx.Data["Groups"] = groups
	public := make([]*donation_model.Donation, 0, publicDonationsLimit)
	for _, d := range donations {
		if d.IsPublic && len(public) < publicDonationsLimit {
			public = append(public, d)
		}
	}
	ctx.Data["Donations"] = public
	ctx.Data["Settings"] = settings
	ctx.Data["CryptoBot"] = cryptobot_service.Enabled()
	ctx.Data["CloudTips"] = setting.CloudTipsURL
	funny := make([]string, len(money.FunnyAmounts))
	for i, v := range money.FunnyAmounts {
		funny[i] = strconv.FormatInt(v, 10)
	}
	ctx.Data["FunnyAmounts"] = strings.Join(funny, ",")
	ctx.Data["PageIsHome"] = true
	ctx.HTML(http.StatusOK, "home")
}

// NotFound renders the 404 page.
func NotFound(ctx *context.Context) {
	ctx.Data["Title"] = "Не найдено"
	ctx.Data["Message"] = "Такой страницы нет."
	ctx.HTML(http.StatusNotFound, "status")
}
