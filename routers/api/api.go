// Package api holds the machine endpoints: the public summary and the donation webhook.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	audit_model "git.cu3rd.ru/cu3rd/dash/models/audit"
	"git.cu3rd.ru/cu3rd/dash/models/db"
	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	expense_model "git.cu3rd.ru/cu3rd/dash/models/expense"
	sitesetting_model "git.cu3rd.ru/cu3rd/dash/models/sitesetting"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
	finance_service "git.cu3rd.ru/cu3rd/dash/services/finance"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func rub(v int64) float64 { return float64(v) / 100 }

// Summary is the public totals and lists as JSON (for a widget elsewhere): GET /api/summary
func Summary(w http.ResponseWriter, r *http.Request) {
	ctx, now := r.Context(), time.Now()
	rates, err := sitesetting_model.Rates(ctx)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	expenses, err1 := expense_model.List(ctx)
	payments, err2 := expense_model.ListPayments(ctx, 0)
	donations, err3 := donation_model.List(ctx, false)
	if err := errors.Join(err1, err2, err3); err != nil {
		slog.Error("summary", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	rows := finance_service.Rows(expenses, rates, now)
	s := finance_service.Summarize(rows, payments, donations, rates, now)

	type item struct {
		Name       string  `json:"name"`
		Category   string  `json:"category"`
		Amount     float64 `json:"amount"`
		Currency   string  `json:"currency"`
		Period     string  `json:"period"`
		MonthlyRub float64 `json:"monthly_rub"`
		NextDue    string  `json:"next_due"`
	}
	type don struct {
		Donor    string  `json:"donor"`
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
		Date     string  `json:"date"`
		Message  string  `json:"message"`
	}
	items := []item{}
	for _, row := range rows {
		if row.IsActive && row.IsPublic {
			items = append(items, item{row.Name, row.CategoryName, rub(row.Amount), row.Currency, row.Period, rub(row.Monthly), row.NextDue})
		}
	}
	dons := []don{}
	for _, d := range donations {
		if d.IsPublic {
			dons = append(dons, don{d.DisplayName(), rub(d.Amount), d.Currency, d.DonatedOn, d.Message})
		}
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeJSON(w, http.StatusOK, map[string]any{
		"currency":         money.Base,
		"monthly_total":    rub(s.MonthlyTotal),
		"yearly_total":     rub(s.YearlyTotal),
		"donations_total":  rub(s.DonationsTotal),
		"donations_12m":    rub(s.Donations12m),
		"coverage_percent": s.Coverage,
		"hidden_expenses":  s.HiddenCount,
		"expenses":         items,
		"donations":        dons,
	})
}

type webhookInput struct {
	Source     string          `json:"source"`
	ExternalID string          `json:"external_id"`
	Donor      string          `json:"donor"`
	Anonymous  bool            `json:"anonymous"`
	Amount     json.RawMessage `json:"amount"`
	Currency   string          `json:"currency"`
	DonatedAt  string          `json:"donated_at"`
	Message    string          `json:"message"`
}

// DonationWebhook receives donations from a payment provider or an adapter script:
// POST /api/donations with Authorization: Bearer $DASH_DONATION_WEBHOOK_TOKEN.
// The same source + external_id is accepted once; repeats answer 200 with the existing id.
func DonationWebhook(w http.ResponseWriter, r *http.Request) {
	if setting.DonationWebhookToken == "" {
		http.Error(w, "webhook disabled", http.StatusNotFound)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(token), []byte(setting.DonationWebhookToken)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var in webhookInput
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err == nil {
		err = json.Unmarshal(body, &in)
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	// amount may come as a number (500.5) or a string ("500,50")
	amountText := strings.Trim(string(in.Amount), `"`)
	if f, err := strconv.ParseFloat(amountText, 64); err == nil {
		amountText = strconv.FormatFloat(f, 'f', -1, 64)
	}
	amount, err := money.Parse(amountText)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "amount: " + err.Error()})
		return
	}
	d := &donation_model.Donation{
		Donor: in.Donor, Amount: amount, Currency: strings.ToUpper(strings.TrimSpace(in.Currency)),
		DonatedOn: donatedOn(in.DonatedAt), Source: in.Source, ExternalID: in.ExternalID,
		Message: in.Message, IsPublic: true, CreatedBy: "webhook",
	}
	if d.Currency == "" {
		d.Currency = money.Base
	}
	if d.Source == "" {
		d.Source = "webhook"
	}
	if in.Anonymous {
		d.Donor = ""
	}
	created, err := donation_model.Create(r.Context(), d)
	switch {
	case errors.Is(err, db.ErrInvalidArgument):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	case err != nil:
		slog.Error("webhook donation", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
		audit_model.Record(r.Context(), "webhook:"+d.Source, "donation.create", money.Format(d.Amount, d.Currency)+" от "+d.DisplayName())
	}
	writeJSON(w, status, map[string]any{"id": d.ID, "created": created})
}

// donatedOn accepts RFC 3339 or YYYY-MM-DD and falls back to today in Moscow.
func donatedOn(s string) string {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.In(money.Moscow).Format(money.DateLayout)
	}
	if _, ok := money.ParseDate(s); ok {
		return s
	}
	return money.Today(time.Now()).Format(money.DateLayout)
}
