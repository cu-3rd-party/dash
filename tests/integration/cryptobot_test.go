package integration

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	audit_model "git.cu3rd.ru/cu3rd/dash/models/audit"
	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	"git.cu3rd.ru/cu3rd/dash/models/unittest"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
	"git.cu3rd.ru/cu3rd/dash/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func enableCryptoBot(t *testing.T, apiURL string) {
	t.Helper()
	setting.CryptoBotToken, setting.CryptoBotURL = "123:test-token", apiURL
	t.Cleanup(func() { setting.CryptoBotToken, setting.CryptoBotURL = "", "https://pay.crypt.bot/api" })
}

func TestCryptoBotDonateCreatesRubleInvoice(t *testing.T) {
	tests.PrepareTestEnv(t)
	var got map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/createInvoice", r.URL.Path)
		assert.Equal(t, "123:test-token", r.Header.Get("Crypto-Pay-API-Token"))
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		_, _ = w.Write([]byte(`{"ok":true,"result":{"invoice_id":7,"bot_invoice_url":"https://t.me/CryptoBot?start=IV7"}}`))
	}))
	defer api.Close()

	post := func(values map[string]string) *httptest.ResponseRecorder {
		return MakeRequest(t, NewRequestWithValues(t, "POST", "/donate", values), NoExpectedStatus)
	}
	assert.Equal(t, http.StatusNotFound, post(map[string]string{"amount": "500"}).Code, "disabled without a token")
	enableCryptoBot(t, api.URL)

	assert.Equal(t, http.StatusUnprocessableEntity, post(map[string]string{"amount": "0"}).Code)
	assert.Equal(t, http.StatusUnprocessableEntity, post(map[string]string{"amount": "abc"}).Code)
	for _, funny := range []string{"67", "1488", "69,50"} {
		assert.Equal(t, http.StatusUnprocessableEntity, post(map[string]string{"amount": funny}).Code, funny)
	}
	assert.Nil(t, got, "no invoice for a funny amount")
	assert.Equal(t, http.StatusUnprocessableEntity, post(map[string]string{"amount": "5", "donor": strings.Repeat("я", 101)}).Code)

	resp := post(map[string]string{"amount": "250,50", "donor": "Маша", "message": "держитесь"})
	require.Equal(t, http.StatusSeeOther, resp.Code)
	assert.Equal(t, "https://t.me/CryptoBot?start=IV7", resp.Header().Get("Location"))
	assert.Equal(t, "fiat", got["currency_type"])
	assert.Equal(t, "RUB", got["fiat"])
	assert.Equal(t, "250.50", got["amount"])
	assert.NotContains(t, got, "swap_to", "no conversion into crypto")
	assert.JSONEq(t, `{"donor":"Маша","message":"держитесь"}`, got["payload"].(string))

	// the htmx form follows the link with HX-Redirect
	resp = MakeRequest(t, AsHTMX(NewRequestWithValues(t, "POST", "/donate", map[string]string{"amount": "100"})), http.StatusNoContent)
	assert.Equal(t, "https://t.me/CryptoBot?start=IV7", resp.Header().Get("HX-Redirect"))

	unittest.AssertCount(t, &donation_model.Donation{Source: "cryptobot"}, 0)
}

func TestCryptoBotDonateAPIError(t *testing.T) {
	tests.PrepareTestEnv(t)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error":{"code":400,"name":"AMOUNT_TOO_SMALL"}}`))
	}))
	defer api.Close()
	enableCryptoBot(t, api.URL)

	resp := MakeRequest(t, NewRequestWithValues(t, "POST", "/donate", map[string]string{"amount": "1"}), http.StatusBadGateway)
	assert.NotContains(t, resp.Body.String(), "AMOUNT_TOO_SMALL", "provider details stay in the log")
}

func TestCryptoBotWebhook(t *testing.T) {
	tests.PrepareTestEnv(t)
	body := `{"update_id":1,"update_type":"invoice_paid","request_date":"2026-11-09T22:31:00Z","payload":{
		"invoice_id":42,"status":"paid","currency_type":"fiat","fiat":"RUB","amount":"250.50",
		"paid_asset":"TON","paid_amount":"1.2","paid_at":"2026-11-09T22:30:00Z",
		"payload":"{\"donor\":\"Маша\",\"message\":\"держитесь\"}"}}`
	post := func(body, signature string) int {
		req := NewRequestWithBody(t, "POST", "/api/cryptobot", strings.NewReader(body))
		req.Header.Del("Sec-Fetch-Site") // Telegram, not a browser
		req.Header.Set("Crypto-Pay-API-Signature", signature)
		return MakeRequest(t, req, NoExpectedStatus).Code
	}
	sign := func(body string) string {
		key := sha256.Sum256([]byte(setting.CryptoBotToken))
		mac := hmac.New(sha256.New, key[:])
		mac.Write([]byte(body))
		return hex.EncodeToString(mac.Sum(nil))
	}

	assert.Equal(t, http.StatusNotFound, post(body, "x"), "disabled without a token")
	enableCryptoBot(t, "http://unused.invalid")
	assert.Equal(t, http.StatusUnauthorized, post(body, "deadbeef"))
	assert.Equal(t, http.StatusUnauthorized, post(body, ""))
	unittest.AssertCount(t, &donation_model.Donation{Source: "cryptobot"}, 0)

	assert.Equal(t, http.StatusOK, post(body, sign(body)))
	assert.Equal(t, http.StatusOK, post(body, sign(body)), "a retry is not a new donation")
	unittest.AssertCount(t, &donation_model.Donation{Source: "cryptobot"}, 1)
	d := unittest.AssertExistsAndLoadBean(t, &donation_model.Donation{Source: "cryptobot", ExternalID: "42"})
	assert.Equal(t, int64(25050), d.Amount)
	assert.Equal(t, "RUB", d.Currency, "recorded in roubles, whatever the donor paid with")
	assert.Equal(t, "Маша", d.Donor)
	assert.Equal(t, "держитесь", d.Message)
	assert.Equal(t, "2026-11-10", d.DonatedOn, "22:30 UTC is already the next day in Moscow")
	assert.True(t, d.IsPublic)
	unittest.AssertCount(t, &audit_model.Audit{Action: "donation.create", Actor: "webhook:cryptobot"}, 1)

	// not ours to record: other update types, unpaid, crypto-priced or non-rouble invoices
	for _, other := range []string{
		strings.Replace(body, "invoice_paid", "something_else", 1),
		strings.Replace(body, `"status":"paid"`, `"status":"active"`, 1),
		strings.Replace(body, `"currency_type":"fiat"`, `"currency_type":"crypto"`, 1),
		strings.Replace(body, `"fiat":"RUB"`, `"fiat":"USD"`, 1),
	} {
		other = strings.Replace(other, `"invoice_id":42`, `"invoice_id":43`, 1)
		assert.Equal(t, http.StatusOK, post(other, sign(other)))
	}
	assert.Equal(t, http.StatusBadRequest, post(`{"update_type":"invoice_paid","payload":{"invoice_id":44,"status":"paid","currency_type":"fiat","fiat":"RUB","amount":"?"}}`, sign(`{"update_type":"invoice_paid","payload":{"invoice_id":44,"status":"paid","currency_type":"fiat","fiat":"RUB","amount":"?"}}`)))
	unittest.AssertCount(t, &donation_model.Donation{Source: "cryptobot"}, 1)
}

func TestHomeShowsDonateFormOnlyWhenEnabled(t *testing.T) {
	tests.PrepareTestEnv(t)
	doc := NewHTMLParser(t, MakeRequest(t, NewRequest(t, "GET", "/"), http.StatusOK).Body)
	doc.AssertElement(t, "#donate-form", false)
	enableCryptoBot(t, "http://unused.invalid")
	doc = NewHTMLParser(t, MakeRequest(t, NewRequest(t, "GET", "/"), http.StatusOK).Body)
	doc.AssertElement(t, "#donate-form", true)
}
