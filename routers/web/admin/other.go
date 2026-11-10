package admin

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	audit_model "git.cu3rd.ru/cu3rd/dash/models/audit"
	category_model "git.cu3rd.ru/cu3rd/dash/models/category"
	"git.cu3rd.ru/cu3rd/dash/models/db"
	member_model "git.cu3rd.ru/cu3rd/dash/models/member"
	sitesetting_model "git.cu3rd.ru/cu3rd/dash/models/sitesetting"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
	"git.cu3rd.ru/cu3rd/dash/modules/templates"
	backup_service "git.cu3rd.ru/cu3rd/dash/services/backup"
	"git.cu3rd.ru/cu3rd/dash/services/context"
	rates_service "git.cu3rd.ru/cu3rd/dash/services/rates"
)

const auditLimit = 300

// ---- categories ----

// Categories handles GET /admin/categories
func Categories(ctx *context.Context) {
	cats, err := category_model.List(ctx.Req.Context())
	if err != nil {
		ctx.ServerError("List categories", err)
		return
	}
	ctx.Data["Categories"] = cats
	page(ctx, "categories", "Категории")
	ctx.HTML(http.StatusOK, "admin/categories")
}

func readCategory(ctx *context.Context) *category_model.Category {
	return &category_model.Category{
		Name:        ctx.FormString("name"),
		Slug:        ctx.FormString("slug"),
		Description: ctx.FormString("description"),
	}
}

// CreateCategory handles POST /admin/categories
func CreateCategory(ctx *context.Context) {
	c := readCategory(ctx)
	if err := category_model.Create(ctx.Req.Context(), c); err != nil {
		ctx.NotFoundOrServerError("Create category", err)
		return
	}
	audit_model.Record(ctx.Req.Context(), ctx.DoerLogin(), "category.create", c.Name)
	ctx.RedirectWithFlash("/admin/categories", "Категория «"+c.Name+"» создана")
}

// UpdateCategory handles POST /admin/categories/{id}
func UpdateCategory(ctx *context.Context) {
	existing, err := category_model.Get(ctx.Req.Context(), ctx.PathParamInt64("id"))
	if err != nil {
		ctx.NotFoundOrServerError("Get category", err)
		return
	}
	c := readCategory(ctx)
	c.ID = existing.ID
	if err := category_model.Update(ctx.Req.Context(), c); err != nil {
		ctx.NotFoundOrServerError("Update category", err)
		return
	}
	audit_model.Record(ctx.Req.Context(), ctx.DoerLogin(), "category.update", c.Name)
	ctx.RedirectWithFlash("/admin/categories", "Категория сохранена")
}

// MoveCategory shifts a category up or down the list: POST /admin/categories/{id}/move (dir=up|down)
func MoveCategory(ctx *context.Context) {
	delta := 1
	if ctx.FormString("dir") == "up" {
		delta = -1
	}
	if err := category_model.Move(ctx.Req.Context(), ctx.PathParamInt64("id"), delta); err != nil {
		ctx.NotFoundOrServerError("Move category", err)
		return
	}
	ctx.Redirect("/admin/categories")
}

// DeleteCategory removes an empty category: POST /admin/categories/{id}/delete
func DeleteCategory(ctx *context.Context) {
	c, err := category_model.Get(ctx.Req.Context(), ctx.PathParamInt64("id"))
	if err != nil {
		ctx.NotFoundOrServerError("Get category", err)
		return
	}
	if err := category_model.Delete(ctx.Req.Context(), c.ID); err != nil {
		ctx.NotFoundOrServerError("Delete category", err)
		return
	}
	audit_model.Record(ctx.Req.Context(), ctx.DoerLogin(), "category.delete", c.Name)
	ctx.RedirectWithFlash("/admin/categories", "Категория «"+c.Name+"» удалена")
}

// ---- members ----

// Members handles GET /admin/members
func Members(ctx *context.Context) {
	members, err := member_model.List(ctx.Req.Context())
	if err != nil {
		ctx.ServerError("List members", err)
		return
	}
	bootstrap := map[string]bool{}
	for _, l := range setting.BootstrapOwners {
		bootstrap[l] = true
	}
	ctx.Data["Members"] = members
	ctx.Data["Bootstrap"] = bootstrap
	ctx.Data["ForgejoURL"] = setting.ForgejoURL
	page(ctx, "members", "Пользователи")
	ctx.HTML(http.StatusOK, "admin/members")
}

// SaveMember grants access or changes a role: POST /admin/members
func SaveMember(ctx *context.Context) {
	c := ctx.Req.Context()
	login, err := member_model.NormalizeLogin(ctx.FormString("login"))
	if err != nil {
		ctx.NotFoundOrServerError("NormalizeLogin", err)
		return
	}
	role := ctx.FormString("role")
	if setting.IsBootstrapOwner(login) && role != member_model.RoleOwner {
		ctx.PlainText(http.StatusUnprocessableEntity, login+" задан в DASH_BOOTSTRAP_OWNERS и всегда владелец")
		return
	}
	existing, err := member_model.GetByLogin(c, login)
	if err != nil && !db.IsErrNotExist(err) {
		ctx.ServerError("GetByLogin", err)
		return
	}
	if existing != nil && existing.Role == member_model.RoleOwner && role != member_model.RoleOwner {
		owners, err := member_model.CountOwners(c)
		if err != nil {
			ctx.ServerError("CountOwners", err)
			return
		}
		if owners <= 1 {
			ctx.PlainText(http.StatusUnprocessableEntity, "Нельзя понизить последнего владельца")
			return
		}
	}
	m := &member_model.Member{Login: login, Name: ctx.FormString("name"), Role: role, CreatedBy: ctx.DoerLogin()}
	if err := member_model.Upsert(c, m); err != nil {
		ctx.NotFoundOrServerError("Upsert member", err)
		return
	}
	if existing == nil {
		audit_model.Record(c, ctx.DoerLogin(), "member.add", login+" → "+member_model.RoleLabel(role))
		ctx.RedirectWithFlash("/admin/members", "Доступ выдан: "+login)
		return
	}
	audit_model.Record(c, ctx.DoerLogin(), "member.role", fmt.Sprintf("%s: %s → %s", login, member_model.RoleLabel(existing.Role), member_model.RoleLabel(role)))
	ctx.RedirectWithFlash("/admin/members", login+": "+member_model.RoleLabel(role))
}

// DeleteMember revokes access: POST /admin/members/{login}/delete
func DeleteMember(ctx *context.Context) {
	c := ctx.Req.Context()
	m, err := member_model.GetByLogin(c, ctx.PathParam("login"))
	if err != nil {
		ctx.NotFoundOrServerError("GetByLogin", err)
		return
	}
	switch {
	case m.Login == ctx.DoerLogin():
		ctx.PlainText(http.StatusUnprocessableEntity, "Нельзя отозвать доступ у самого себя")
		return
	case setting.IsBootstrapOwner(m.Login):
		ctx.PlainText(http.StatusUnprocessableEntity, m.Login+" задан в DASH_BOOTSTRAP_OWNERS: уберите его из переменной окружения")
		return
	}
	if err := member_model.Delete(c, m.Login); err != nil {
		ctx.ServerError("Delete member", err)
		return
	}
	audit_model.Record(c, ctx.DoerLogin(), "member.remove", m.Login)
	ctx.RedirectWithFlash("/admin/members", "Доступ отозван: "+m.Login)
}

// ---- settings and audit ----

var settingKeys = []string{
	sitesetting_model.KeySiteTitle, sitesetting_model.KeyPublicIntro, sitesetting_model.KeyDonateText,
	sitesetting_model.KeyRatePrefix + "USD", sitesetting_model.KeyRatePrefix + "EUR",
}

// Settings handles GET /admin/settings
func Settings(ctx *context.Context) {
	values, err := sitesetting_model.All(ctx.Req.Context())
	if err != nil {
		ctx.ServerError("Settings", err)
		return
	}
	ctx.Data["Values"] = values
	ctx.Data["RatesURL"] = setting.RatesURL
	ctx.Data["RatesDate"] = values[rates_service.KeySourceDate]
	ctx.Data["RatesFetched"], _ = strconv.ParseInt(values[rates_service.KeyFetched], 10, 64)
	ctx.Data["Env"] = map[string]any{
		"DB": setting.DBType, "BaseURL": setting.BaseURL, "ForgejoURL": setting.ForgejoURL, "OAuth": setting.OAuthEnabled(),
		"DevLogin": setting.DevLogin, "Webhook": setting.DonationWebhookToken != "", "Bootstrap": setting.BootstrapOwners,
	}
	page(ctx, "settings", "Настройки")
	ctx.HTML(http.StatusOK, "admin/settings")
}

// SaveSettings handles POST /admin/settings
func SaveSettings(ctx *context.Context) {
	values := map[string]string{}
	for _, k := range settingKeys {
		values[k] = ctx.FormString(k)
	}
	if err := sitesetting_model.Set(ctx.Req.Context(), values); err != nil {
		ctx.NotFoundOrServerError("Set settings", err)
		return
	}
	audit_model.Record(ctx.Req.Context(), ctx.DoerLogin(), "settings.update", "")
	ctx.RedirectWithFlash("/admin/settings", "Настройки сохранены")
}

// UpdateRates loads the Bank of Russia rates right now: POST /admin/settings/rates
func UpdateRates(ctx *context.Context) {
	date, err := rates_service.Update(ctx.Req.Context(), ctx.DoerLogin())
	if err != nil {
		slog.Warn("update rates", "err", err)
		ctx.PlainText(http.StatusBadGateway, "Не удалось загрузить курсы ЦБ: "+err.Error())
		return
	}
	ctx.RedirectWithFlash("/admin/settings", "Курсы ЦБ на "+templates.Date(date)+" загружены")
}

// ExportData downloads all data as JSON: GET /admin/settings/export
func ExportData(ctx *context.Context) {
	d, err := backup_service.Export(ctx.Req.Context())
	if err != nil {
		ctx.ServerError("Export", err)
		return
	}
	h := ctx.Resp.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Content-Disposition", `attachment; filename="dash-`+time.Now().In(money.Moscow).Format("2006-01-02")+`.json"`)
	if err := backup_service.Write(ctx.Resp, d); err != nil {
		slog.Error("write export", "err", err)
	}
}

// ImportData loads a JSON dump into an empty instance: POST /admin/settings/import
func ImportData(ctx *context.Context) {
	f, _, err := ctx.Req.FormFile("dump")
	if err != nil {
		ctx.PlainText(http.StatusBadRequest, "Выберите файл экспорта")
		return
	}
	defer func() { _ = f.Close() }()
	sum, err := backup_service.Import(ctx.Req.Context(), f)
	if err != nil {
		ctx.NotFoundOrServerError("Import", err)
		return
	}
	audit_model.Record(ctx.Req.Context(), ctx.DoerLogin(), "data.import", sum.String())
	ctx.RedirectWithFlash("/admin/settings", "Импортировано — "+sum.String())
}

// Audit handles GET /admin/audit
func Audit(ctx *context.Context) {
	entries, err := audit_model.List(ctx.Req.Context(), auditLimit)
	if err != nil {
		ctx.ServerError("List audit", err)
		return
	}
	ctx.Data["Entries"] = entries
	page(ctx, "audit", "Журнал")
	ctx.HTML(http.StatusOK, "admin/audit")
}
