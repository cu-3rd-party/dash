package admin

import (
	"fmt"
	"net/http"
	"time"

	audit_model "git.cu3rd.ru/cu3rd/dash/models/audit"
	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	sitesetting_model "git.cu3rd.ru/cu3rd/dash/models/sitesetting"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
	"git.cu3rd.ru/cu3rd/dash/services/context"
)

// Donations lists donations and the manual entry form: GET /admin/donations
func Donations(ctx *context.Context) {
	c := ctx.Req.Context()
	list, err := donation_model.List(c, false)
	if err != nil {
		ctx.ServerError("List donations", err)
		return
	}
	rates, err := sitesetting_model.Rates(c)
	if err != nil {
		ctx.ServerError("Rates", err)
		return
	}
	var total int64
	for _, d := range list {
		total += rates.ToBase(d.Amount, d.Currency)
	}
	ctx.Data["Donations"] = list
	ctx.Data["Total"] = total
	ctx.Data["WebhookEnabled"] = setting.DonationWebhookToken != ""
	ctx.Data["BaseURL"] = setting.BaseURL
	ctx.Data["Today"] = money.Today(time.Now()).Format(money.DateLayout)
	page(ctx, "donations", "Донаты")
	ctx.HTML(http.StatusOK, "admin/donations")
}

func readDonation(ctx *context.Context, d *donation_model.Donation) error {
	amount, err := money.Parse(ctx.FormString("amount"))
	if err != nil {
		return donation_model.ErrInvalidDonation{Reason: "Сумма: " + err.Error()}
	}
	d.Amount = amount
	d.Donor = ctx.FormString("donor")
	if ctx.FormBool("anonymous") {
		d.Donor = ""
	}
	d.Currency = ctx.FormString("currency")
	d.DonatedOn = ctx.FormString("donated_on")
	d.Source = ctx.FormString("source")
	d.Message = ctx.FormString("message")
	d.IsPublic = ctx.FormBool("is_public")
	return nil
}

// CreateDonation adds a donation by hand: POST /admin/donations
func CreateDonation(ctx *context.Context) {
	d := &donation_model.Donation{CreatedBy: ctx.DoerLogin()}
	err := readDonation(ctx, d)
	if err == nil {
		_, err = donation_model.Create(ctx.Req.Context(), d)
	}
	if err != nil {
		ctx.NotFoundOrServerError("Create donation", err)
		return
	}
	audit_model.Record(ctx.Req.Context(), ctx.DoerLogin(), "donation.create", money.Format(d.Amount, d.Currency)+" от "+d.DisplayName())
	ctx.RedirectWithFlash("/admin/donations", "Донат добавлен")
}

func loadDonation(ctx *context.Context) *donation_model.Donation {
	d, err := donation_model.Get(ctx.Req.Context(), ctx.PathParamInt64("id"))
	if err != nil {
		ctx.NotFoundOrServerError("Get donation", err)
		return nil
	}
	return d
}

// EditDonation handles GET /admin/donations/{id}/edit
func EditDonation(ctx *context.Context) {
	d := loadDonation(ctx)
	if d == nil {
		return
	}
	ctx.Data["Donation"] = d
	page(ctx, "donations", fmt.Sprintf("Донат #%d", d.ID))
	ctx.HTML(http.StatusOK, "admin/donation_form")
}

// UpdateDonation handles POST /admin/donations/{id}/edit
func UpdateDonation(ctx *context.Context) {
	d := loadDonation(ctx)
	if d == nil {
		return
	}
	err := readDonation(ctx, d)
	if err == nil {
		err = donation_model.Update(ctx.Req.Context(), d)
	}
	if err != nil {
		ctx.NotFoundOrServerError("Update donation", err)
		return
	}
	audit_model.Record(ctx.Req.Context(), ctx.DoerLogin(), "donation.update", fmt.Sprintf("#%d %s от %s", d.ID, money.Format(d.Amount, d.Currency), d.DisplayName()))
	ctx.RedirectWithFlash("/admin/donations", "Донат сохранён")
}

// DeleteDonation handles POST /admin/donations/{id}/delete
func DeleteDonation(ctx *context.Context) {
	d := loadDonation(ctx)
	if d == nil {
		return
	}
	if err := donation_model.Delete(ctx.Req.Context(), d.ID); err != nil {
		ctx.ServerError("Delete donation", err)
		return
	}
	audit_model.Record(ctx.Req.Context(), ctx.DoerLogin(), "donation.delete", fmt.Sprintf("#%d %s от %s", d.ID, money.Format(d.Amount, d.Currency), d.DisplayName()))
	ctx.RedirectWithFlash("/admin/donations", "Донат удалён")
}
