// Package cryptobot takes donations through Crypto Pay (Telegram @CryptoBot).
//
// The invoice is priced in roubles (currency_type=fiat, fiat=RUB): the donor pays the
// equivalent in a crypto asset of their choice, and the donation is recorded as roubles,
// exactly as asked. Nothing is stored before payment: the donor's name and message travel in
// the invoice payload and come back in the signed "invoice_paid" webhook.
package cryptobot

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
)

// Source marks donations received through Crypto Pay.
const Source = "cryptobot"

// SignatureHeader carries the hex HMAC-SHA256 of the webhook body.
const SignatureHeader = "Crypto-Pay-API-Signature"

const (
	invoiceTTL  = time.Hour
	description = "Донат на инфраструктуру cu3rd"
)

var client = &http.Client{Timeout: 20 * time.Second}

// Enabled reports whether Crypto Pay is configured.
func Enabled() bool { return setting.CryptoBotToken != "" }

// Invoice is the part of the Crypto Pay Invoice object that we read.
type Invoice struct {
	ID            int64  `json:"invoice_id"`
	CurrencyType  string `json:"currency_type"`
	Fiat          string `json:"fiat"`
	Amount        string `json:"amount"`
	Status        string `json:"status"`
	PaidAt        string `json:"paid_at"`
	Payload       string `json:"payload"`
	BotInvoiceURL string `json:"bot_invoice_url"`
}

// payload is what we keep in the invoice between the form and the webhook.
type payload struct {
	Donor   string `json:"donor"`
	Message string `json:"message"`
}

// CreateInvoice asks Crypto Pay for a payment link for amount (minor units of roubles).
// An empty donor is an anonymous donation.
func CreateInvoice(ctx context.Context, amount int64, donor, message string) (string, error) {
	meta, err := json.Marshal(payload{Donor: donor, Message: message})
	if err != nil {
		return "", err
	}
	var inv Invoice
	err = call(ctx, "createInvoice", map[string]any{
		"currency_type":  "fiat",
		"fiat":           money.Base,
		"amount":         money.Input(amount),
		"description":    description,
		"payload":        string(meta),
		"allow_comments": false,
		"expires_in":     int(invoiceTTL.Seconds()),
	}, &inv)
	if err != nil {
		return "", err
	}
	if inv.BotInvoiceURL == "" {
		return "", errors.New("cryptobot: no payment link in the response")
	}
	return inv.BotInvoiceURL, nil
}

func call(ctx context.Context, method string, params map[string]any, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, setting.CryptoBotURL+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Crypto-Pay-API-Token", setting.CryptoBotToken)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cryptobot %s: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("cryptobot %s: %w", method, err)
	}
	var env struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  struct {
			Code int    `json:"code"`
			Name string `json:"name"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("cryptobot %s: status %d, bad response: %w", method, resp.StatusCode, err)
	}
	if !env.OK {
		return fmt.Errorf("cryptobot %s: %d %s", method, env.Error.Code, env.Error.Name)
	}
	return json.Unmarshal(env.Result, out)
}

// ValidSignature checks the webhook signature: HMAC-SHA256 of the raw body
// keyed with SHA256 of the app token.
func ValidSignature(body []byte, signature string) bool {
	key := sha256.Sum256([]byte(setting.CryptoBotToken))
	mac := hmac.New(sha256.New, key[:])
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(signature))
}

// ErrIgnored marks webhook updates that are not a paid rouble invoice of ours.
var ErrIgnored = errors.New("cryptobot: update ignored")

// DonationFromUpdate turns a webhook body into a donation. It returns ErrIgnored for
// other update types and for invoices that are not paid or not priced in roubles.
func DonationFromUpdate(body []byte) (*donation_model.Donation, error) {
	var upd struct {
		Type    string  `json:"update_type"`
		Invoice Invoice `json:"payload"`
	}
	if err := json.Unmarshal(body, &upd); err != nil {
		return nil, err
	}
	inv := upd.Invoice
	if upd.Type != "invoice_paid" || inv.Status != "paid" || inv.CurrencyType != "fiat" || inv.Fiat != money.Base {
		return nil, ErrIgnored
	}
	amount, err := money.Parse(inv.Amount)
	if err != nil {
		return nil, fmt.Errorf("cryptobot invoice %d: amount: %w", inv.ID, err)
	}
	var meta payload
	_ = json.Unmarshal([]byte(inv.Payload), &meta) // a foreign invoice: no name, no message
	paid := time.Now()
	if t, err := time.Parse(time.RFC3339, inv.PaidAt); err == nil {
		paid = t
	}
	return &donation_model.Donation{
		Donor: meta.Donor, Amount: amount, Currency: money.Base,
		DonatedOn: money.Today(paid).Format(money.DateLayout),
		Source:    Source, ExternalID: strconv.FormatInt(inv.ID, 10),
		Message: meta.Message, IsPublic: true, CreatedBy: "webhook",
	}, nil
}
