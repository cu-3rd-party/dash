package home

import (
	"log/slog"
	"net/http"
	"unicode/utf8"

	"git.cu3rd.ru/cu3rd/dash/modules/money"
	"git.cu3rd.ru/cu3rd/dash/services/context"
	cryptobot_service "git.cu3rd.ru/cu3rd/dash/services/cryptobot"
)

const (
	maxDonor    = 100
	maxMessage  = 500
	maxDonation = 1_000_000 * 100 // minor units
)

// Donate creates a Crypto Pay invoice in roubles and sends the donor to pay it: POST /donate.
// The donation itself is recorded when the paid-invoice webhook arrives.
func Donate(ctx *context.Context) {
	if !cryptobot_service.Enabled() {
		ctx.PlainText(http.StatusNotFound, "Приём донатов через Crypto Bot не настроен.")
		return
	}
	amount, err := money.Parse(ctx.FormString("amount"))
	donor, message := ctx.FormString("donor"), ctx.FormString("message")
	switch {
	case err != nil:
		ctx.PlainText(http.StatusUnprocessableEntity, err.Error())
		return
	case amount <= 0 || amount > maxDonation:
		ctx.PlainText(http.StatusUnprocessableEntity, "Сумма доната — от 1 ₽ до 1 000 000 ₽.")
		return
	case money.Funny(amount):
		ctx.PlainText(http.StatusUnprocessableEntity, "Такая сумма не принимается, выберите другую.")
		return
	case utf8.RuneCountInString(donor) > maxDonor || utf8.RuneCountInString(message) > maxMessage:
		ctx.PlainText(http.StatusUnprocessableEntity, "Слишком длинное имя или сообщение.")
		return
	}
	url, err := cryptobot_service.CreateInvoice(ctx.Req.Context(), amount, donor, message)
	if err != nil {
		slog.Error("create cryptobot invoice", "err", err)
		ctx.PlainText(http.StatusBadGateway, "Не удалось создать счёт в Crypto Bot, попробуйте позже.")
		return
	}
	ctx.Redirect(url)
}
