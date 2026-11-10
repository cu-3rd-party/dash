package api

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	audit_model "git.cu3rd.ru/cu3rd/dash/models/audit"
	"git.cu3rd.ru/cu3rd/dash/models/db"
	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
	cryptobot_service "git.cu3rd.ru/cu3rd/dash/services/cryptobot"
)

// CryptoBotWebhook receives Crypto Pay updates: POST /api/cryptobot.
// Enable it in @CryptoBot → Crypto Pay → My Apps → Webhooks with https://<host>/api/cryptobot.
// A paid rouble invoice becomes a donation once: Crypto Pay retries until it gets a 2xx.
func CryptoBotWebhook(w http.ResponseWriter, r *http.Request) {
	if !cryptobot_service.Enabled() {
		http.Error(w, "webhook disabled", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !cryptobot_service.ValidSignature(body, r.Header.Get(cryptobot_service.SignatureHeader)) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	d, err := cryptobot_service.DonationFromUpdate(body)
	if errors.Is(err, cryptobot_service.ErrIgnored) {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err != nil {
		// a retry cannot fix a body we cannot read
		slog.Error("cryptobot update", "err", err)
		http.Error(w, "bad update", http.StatusBadRequest)
		return
	}
	created, err := donation_model.Create(r.Context(), d)
	switch {
	case errors.Is(err, db.ErrInvalidArgument):
		// likewise: log it for the admin and stop the retries
		slog.Error("cryptobot donation rejected", "err", err)
	case err != nil:
		slog.Error("cryptobot webhook", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	case created:
		audit_model.Record(r.Context(), "webhook:"+d.Source, "donation.create", money.Format(d.Amount, d.Currency)+" от "+d.DisplayName())
	}
	w.WriteHeader(http.StatusOK)
}
