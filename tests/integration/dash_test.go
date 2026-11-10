package integration

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	audit_model "git.cu3rd.ru/cu3rd/dash/models/audit"
	category_model "git.cu3rd.ru/cu3rd/dash/models/category"
	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	expense_model "git.cu3rd.ru/cu3rd/dash/models/expense"
	member_model "git.cu3rd.ru/cu3rd/dash/models/member"
	"git.cu3rd.ru/cu3rd/dash/models/unittest"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
	auth_service "git.cu3rd.ru/cu3rd/dash/services/auth"
	"git.cu3rd.ru/cu3rd/dash/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtures: boss is the owner, ed the editor, vi the viewer
func as(req *http.Request, login string) *http.Request {
	req.AddCookie(auth_service.SessionCookie(login))
	return req
}

func TestHomeIsPublic(t *testing.T) {
	tests.PrepareTestEnv(t)

	resp := MakeRequest(t, NewRequest(t, "GET", "/"), http.StatusOK)
	doc := NewHTMLParser(t, resp.Body)
	table := doc.Find("#public-expenses").Text()
	assert.Contains(t, table, "grm")
	assert.Contains(t, table, "cu3rd.ru")
	assert.NotContains(t, table, "secret-backup", "private expenses are hidden")
	assert.NotContains(t, table, "old-domain", "archived expenses are hidden")
	assert.Contains(t, table, "вкл. 1 непубличных", "but they are counted")
	// descriptions are folded: the name is the summary, the text opens on click
	grm := doc.Find("#public-expenses details.expense-details").First()
	assert.Contains(t, grm.Find("summary").Text(), "grm")
	assert.Contains(t, grm.Text(), "Основной узел")
	_, open := grm.Attr("open")
	assert.False(t, open)

	feed := doc.Find(".donation-feed").Text()
	assert.Contains(t, feed, "Вася")
	assert.Contains(t, feed, "Аноним")
	assert.NotContains(t, feed, "Скрытый")
	doc.AssertElement(t, `a[href="/admin"]`, false)
	doc.AssertElement(t, `a[href="/signin"]`, true)
}

func TestAdminNeedsSignIn(t *testing.T) {
	tests.PrepareTestEnv(t)

	resp := MakeRequest(t, NewRequest(t, "GET", "/admin/expenses?status=all"), http.StatusSeeOther)
	assert.Equal(t, "/signin?next=%2Fadmin%2Fexpenses%3Fstatus%3Dall", resp.Header().Get("Location"))

	// a member who lost access is signed out at once
	require.NoError(t, member_model.Delete(t.Context(), "vi"))
	MakeRequest(t, as(NewRequest(t, "GET", "/admin"), "vi"), http.StatusSeeOther)
}

func TestRoles(t *testing.T) {
	tests.PrepareTestEnv(t)

	resp := MakeRequest(t, as(NewRequest(t, "GET", "/admin/expenses"), "vi"), http.StatusOK)
	doc := NewHTMLParser(t, resp.Body)
	doc.AssertElement(t, "#expense-1", true)
	doc.AssertElement(t, `a[href="/admin/expenses/1/edit"]`, false)
	doc.AssertElement(t, `a[href="/admin/expenses/new"]`, false)
	doc.AssertElement(t, `a[href="/admin/members"]`, false)

	MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/expenses/1/edit", map[string]string{"name": "x"}), "vi"), http.StatusForbidden)
	MakeRequest(t, as(NewRequest(t, "GET", "/admin/members"), "ed"), http.StatusForbidden)
	MakeRequest(t, as(NewRequest(t, "GET", "/admin/members"), "boss"), http.StatusOK)

	resp = MakeRequest(t, as(NewRequest(t, "GET", "/admin/expenses"), "ed"), http.StatusOK)
	NewHTMLParser(t, resp.Body).AssertElement(t, `a[href="/admin/expenses/1/edit"]`, true)
}

func TestExpenseFilters(t *testing.T) {
	tests.PrepareTestEnv(t)

	count := func(query string) int {
		resp := MakeRequest(t, as(NewRequest(t, "GET", "/admin/expenses"+query), "vi"), http.StatusOK)
		return NewHTMLParser(t, resp.Body).Find(`#expenses tr[id^="expense-"]`).Length()
	}
	assert.Equal(t, 4, count(""), "active only by default")
	assert.Equal(t, 5, count("?status=all"))
	assert.Equal(t, 1, count("?status=overdue"))
	assert.Equal(t, 2, count("?status=attention"), "overdue nxs and secret-backup without a date")
	assert.Equal(t, 3, count("?cat=1"))
	assert.Equal(t, 2, count("?owner=boss"))
}

func TestCreateAndEditExpense(t *testing.T) {
	tests.PrepareTestEnv(t)

	resp := MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/expenses/new", map[string]string{
		"name": "", "category_id": "1", "amount": "сто", "currency": "RUB", "period": "month",
	}), "ed"), http.StatusUnprocessableEntity)
	doc := NewHTMLParser(t, resp.Body)
	assert.Contains(t, doc.Find("#form-errors").Text(), "Стоимость")
	assert.Equal(t, "сто", doc.Find("#amount").AttrOr("value", ""), "the form keeps what was typed")

	resp = MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/expenses/new", map[string]string{
		"name": "Figma", "category_id": "3", "amount": "1 234,50", "currency": "USD", "period": "year",
		"owner": "ed", "next_due": "2027-01-01", "is_active": "on", "description": "дизайн\nсайта",
	}), "ed"), http.StatusSeeOther)
	e := unittest.AssertExistsAndLoadBean(t, &expense_model.Expense{Name: "Figma"})
	assert.Equal(t, "/admin/expenses/"+itoa(e.ID), resp.Header().Get("Location"))
	assert.Equal(t, int64(123450), e.Amount)
	assert.False(t, e.IsPublic)
	unittest.AssertExistsAndLoadBean(t, &audit_model.Audit{Actor: "ed", Action: "expense.create"})

	saved := MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/expenses/1/edit", map[string]string{
		"name": "grm (AX42)", "category_id": "1", "amount": "46.50", "currency": "EUR", "period": "month",
		"next_due": "2099-01-15", "is_active": "on", "is_public": "on", "owner": "boss",
	}), "ed"), http.StatusSeeOther)
	grm := unittest.AssertExistsAndLoadBean(t, &expense_model.Expense{ID: 1})
	assert.Equal(t, "grm (AX42)", grm.Name)
	assert.Equal(t, int64(4650), grm.Amount)

	next := as(NewRequest(t, "GET", "/admin/expenses/1"), "ed")
	for _, c := range saved.Result().Cookies() {
		next.AddCookie(c)
	}
	resp = MakeRequest(t, next, http.StatusOK)
	doc = NewHTMLParser(t, resp.Body)
	assert.Contains(t, doc.Find("#flash").Text(), "Сохранено", "the flash survives the redirect")
	doc.AssertElement(t, "#edit-expense", true)
}

func TestRecordPayment(t *testing.T) {
	tests.PrepareTestEnv(t)

	req := AsHTMX(as(NewRequestWithValues(t, "POST", "/admin/expenses/2/payments", map[string]string{
		"paid_on": "2026-11-10", "amount": "1500", "currency": "RUB", "periods": "1", "advance": "on", "note": "счёт 7",
	}), "ed"))
	resp := MakeRequest(t, req, http.StatusNoContent)
	assert.Equal(t, "/admin/expenses/2", resp.Header().Get("HX-Redirect"))
	nxs := unittest.AssertExistsAndLoadBean(t, &expense_model.Expense{ID: 2})
	assert.Equal(t, "2026-12-10", nxs.NextDue, "long overdue: counted from the payment date")
	unittest.AssertExistsAndLoadBean(t, &expense_model.Payment{ExpenseID: 2, Note: "счёт 7", CreatedBy: "ed"})

	resp = MakeRequest(t, AsHTMX(as(NewRequestWithValues(t, "POST", "/admin/expenses/2/payments", map[string]string{
		"paid_on": "2026-11-10", "amount": "-1", "currency": "RUB",
	}), "ed")), http.StatusUnprocessableEntity)
	assert.Contains(t, resp.Body.String(), "отрицательной", "htmx shows this text as a toast")

	MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/payments/1/delete", nil), "ed"), http.StatusSeeOther)
	unittest.AssertNotExistsBean(t, &expense_model.Payment{ID: 1})
}

func TestDeleteExpenseNeedsName(t *testing.T) {
	tests.PrepareTestEnv(t)

	MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/expenses/1/delete", map[string]string{"confirm": "nope"}), "ed"), http.StatusUnprocessableEntity)
	unittest.AssertExistsAndLoadBean(t, &expense_model.Expense{ID: 1})
	MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/expenses/1/delete", map[string]string{"confirm": "grm"}), "ed"), http.StatusSeeOther)
	unittest.AssertNotExistsBean(t, &expense_model.Expense{ID: 1})
	unittest.AssertCount(t, &expense_model.Payment{ExpenseID: 1}, 0)
}

func TestCrossSitePostIsRejected(t *testing.T) {
	tests.PrepareTestEnv(t)

	req := as(NewRequestWithValues(t, "POST", "/admin/expenses/1/delete", map[string]string{"confirm": "grm"}), "boss")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	MakeRequest(t, req, http.StatusForbidden)
	unittest.AssertExistsAndLoadBean(t, &expense_model.Expense{ID: 1})
}

func TestDevSignIn(t *testing.T) {
	tests.PrepareTestEnv(t)
	MakeRequest(t, NewRequestWithValues(t, "POST", "/signin/dev", map[string]string{"login": "ed"}), http.StatusNotFound)

	setting.DevLogin = true
	t.Cleanup(func() { setting.DevLogin = false })
	resp := MakeRequest(t, NewRequestWithValues(t, "POST", "/signin/dev", map[string]string{"login": "ED", "next": "/admin/donations"}), http.StatusSeeOther)
	assert.Equal(t, "/admin/donations", resp.Header().Get("Location"))
	require.NotEmpty(t, resp.Result().Cookies())

	resp = MakeRequest(t, NewRequestWithValues(t, "POST", "/signin/dev", map[string]string{"login": "stranger", "next": "//evil.example"}), http.StatusForbidden)
	assert.Contains(t, NewHTMLParser(t, resp.Body).Find("#signin-error").Text(), "нет доступа")
}

func TestMembers(t *testing.T) {
	tests.PrepareTestEnv(t)

	resp := MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/members", map[string]string{"login": "boss", "role": "viewer"}), "boss"), http.StatusUnprocessableEntity)
	assert.Contains(t, resp.Body.String(), "последнего владельца")

	MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/members", map[string]string{"login": "@NewGuy", "role": "editor", "name": "Новенький"}), "boss"), http.StatusSeeOther)
	m := unittest.AssertExistsAndLoadBean(t, &member_model.Member{Login: "newguy"})
	assert.Equal(t, "editor", m.Role)
	assert.Equal(t, "boss", m.CreatedBy)

	MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/members/boss/delete", nil), "boss"), http.StatusUnprocessableEntity)
	MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/members/vi/delete", nil), "boss"), http.StatusSeeOther)
	unittest.AssertNotExistsBean(t, &member_model.Member{Login: "vi"})

	setting.BootstrapOwners = []string{"ed"}
	t.Cleanup(func() { setting.BootstrapOwners = nil })
	MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/members", map[string]string{"login": "ed", "role": "viewer"}), "boss"), http.StatusUnprocessableEntity)
}

func TestDonations(t *testing.T) {
	tests.PrepareTestEnv(t)

	MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/donations", map[string]string{
		"donor": "Петя", "anonymous": "on", "amount": "300", "currency": "RUB", "donated_on": "2026-11-09", "is_public": "on",
	}), "ed"), http.StatusSeeOther)
	d := unittest.AssertExistsAndLoadBean(t, &donation_model.Donation{Amount: 30000, DonatedOn: "2026-11-09"})
	assert.Empty(t, d.Donor, "the anonymous box wins over the name")
	assert.Equal(t, "manual", d.Source)

	MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/donations/1/edit", map[string]string{
		"donor": "Василий", "amount": "500", "currency": "RUB", "donated_on": "2026-10-01", "source": "card",
	}), "ed"), http.StatusSeeOther)
	v := unittest.AssertExistsAndLoadBean(t, &donation_model.Donation{ID: 1})
	assert.Equal(t, "Василий", v.Donor)
	assert.False(t, v.IsPublic)
}

func TestDonationWebhook(t *testing.T) {
	tests.PrepareTestEnv(t)
	post := func(token, body string) *http.Response {
		req := NewRequestWithBody(t, "POST", "/api/donations", strings.NewReader(body))
		req.Header.Del("Sec-Fetch-Site") // a server, not a browser
		req.Header.Set("Authorization", "Bearer "+token)
		return MakeRequest(t, req, NoExpectedStatus).Result()
	}
	body := `{"source":"boosty","external_id":"tx-9","donor":"Маша","amount":"250,50","currency":"rub","donated_at":"2026-11-09T22:30:00Z"}`

	assert.Equal(t, http.StatusNotFound, post("x", body).StatusCode, "disabled without a token")
	setting.DonationWebhookToken = "hook-secret"
	t.Cleanup(func() { setting.DonationWebhookToken = "" })
	assert.Equal(t, http.StatusUnauthorized, post("wrong", body).StatusCode)
	assert.Equal(t, http.StatusCreated, post("hook-secret", body).StatusCode)
	assert.Equal(t, http.StatusOK, post("hook-secret", body).StatusCode, "a repeat is not a new donation")
	assert.Equal(t, http.StatusUnprocessableEntity, post("hook-secret", `{"amount":0}`).StatusCode)

	d := unittest.AssertExistsAndLoadBean(t, &donation_model.Donation{ExternalID: "tx-9"})
	assert.Equal(t, int64(25050), d.Amount)
	assert.Equal(t, "RUB", d.Currency)
	assert.Equal(t, "2026-11-10", d.DonatedOn, "22:30 UTC is already the next day in Moscow")
	unittest.AssertCount(t, &donation_model.Donation{Source: "boosty"}, 2)
}

func TestSummaryAPI(t *testing.T) {
	tests.PrepareTestEnv(t)

	resp := MakeRequest(t, NewRequest(t, "GET", "/api/summary"), http.StatusOK)
	var out struct {
		Monthly   float64 `json:"monthly_total"`
		Hidden    int     `json:"hidden_expenses"`
		Expenses  []struct{ Name string }
		Donations []struct{ Donor string }
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	assert.InDelta(t, 4490+1500+300+100, out.Monthly, 0.01)
	assert.Equal(t, 1, out.Hidden)
	assert.Len(t, out.Expenses, 3)
	assert.Len(t, out.Donations, 3)
	assert.NotContains(t, resp.Body.String(), "Скрытый")
}

func TestSettings(t *testing.T) {
	tests.PrepareTestEnv(t)

	values := map[string]string{"site_title": "Касса", "public_intro": "", "donate_text": "карта 0000", "rate_USD": "abc", "rate_EUR": "100"}
	MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/settings", values), "boss"), http.StatusUnprocessableEntity)
	values["rate_USD"] = "91,2"
	MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/settings", values), "boss"), http.StatusSeeOther)

	resp := MakeRequest(t, NewRequest(t, "GET", "/"), http.StatusOK)
	doc := NewHTMLParser(t, resp.Body)
	assert.Equal(t, "Касса", strings.TrimSpace(doc.Find("h1").Text()))
	assert.Contains(t, doc.Text(), "карта 0000")
}

func TestEveryAdminPageRenders(t *testing.T) {
	tests.PrepareTestEnv(t)
	for _, path := range []string{
		"/admin", "/admin/expenses", "/admin/expenses?status=all", "/admin/expenses/1", "/admin/expenses/3",
		"/admin/expenses/new", "/admin/expenses/1/edit", "/admin/payments", "/admin/donations",
		"/admin/donations/2/edit", "/admin/categories", "/admin/audit", "/admin/members", "/admin/settings",
	} {
		MakeRequest(t, as(NewRequest(t, "GET", path), "boss"), http.StatusOK)
	}
	MakeRequest(t, as(NewRequest(t, "GET", "/admin/expenses/999"), "boss"), http.StatusNotFound)
	MakeRequest(t, NewRequest(t, "GET", "/nope"), http.StatusNotFound)
	MakeRequest(t, NewRequest(t, "GET", "/signin"), http.StatusOK)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestUpdateRatesButton(t *testing.T) {
	tests.PrepareTestEnv(t)
	feed := `<?xml version="1.0" encoding="windows-1251"?><ValCurs Date="09.11.2026">` +
		`<Valute><CharCode>USD</CharCode><Nominal>1</Nominal><Value>80,5</Value></Valute>` +
		`<Valute><CharCode>EUR</CharCode><Nominal>1</Nominal><Value>90,25</Value></Valute></ValCurs>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(feed)) }))
	defer srv.Close()
	old := setting.RatesURL
	setting.RatesURL = srv.URL
	t.Cleanup(func() { setting.RatesURL = old })

	MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/settings/rates", nil), "ed"), http.StatusForbidden)
	MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/settings/rates", nil), "boss"), http.StatusSeeOther)

	resp := MakeRequest(t, as(NewRequest(t, "GET", "/admin/settings"), "boss"), http.StatusOK)
	doc := NewHTMLParser(t, resp.Body)
	assert.Equal(t, "80.5", doc.Find("#rate_USD").AttrOr("value", ""))
	assert.Contains(t, doc.Find("#rates-status").Text(), "Курс ЦБ на 9 ноя")

	setting.RatesURL = srv.URL + "/missing"
	srv.Config.Handler = http.NotFoundHandler()
	MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/settings/rates", nil), "boss"), http.StatusBadGateway)
}

// Behind de.devbox.cu3rd.ru/finance the proxy strips the prefix; links, redirects and cookies add it back.
func TestUnderSubPath(t *testing.T) {
	tests.PrepareTestEnv(t)
	setting.AppSubURL = "/finance"
	t.Cleanup(func() { setting.AppSubURL = "" })

	resp := MakeRequest(t, NewRequest(t, "GET", "/"), http.StatusOK)
	doc := NewHTMLParser(t, resp.Body)
	doc.AssertElement(t, `a[href="/finance/signin"]`, true)
	doc.AssertElement(t, `link[href^="/finance/assets/css/index.css"]`, true)
	doc.AssertElement(t, `a[href="/signin"]`, false)

	resp = MakeRequest(t, NewRequest(t, "GET", "/admin/expenses"), http.StatusSeeOther)
	assert.Equal(t, "/finance/signin?next=%2Fadmin%2Fexpenses", resp.Header().Get("Location"))

	resp = MakeRequest(t, as(NewRequestWithValues(t, "POST", "/admin/categories", map[string]string{"name": "VPN", "slug": "vpn"}), "ed"), http.StatusSeeOther)
	assert.Equal(t, "/finance/admin/categories", resp.Header().Get("Location"))
	for _, c := range resp.Result().Cookies() {
		assert.Equal(t, "/finance/", c.Path, c.Name)
	}

	setting.DevLogin = true
	t.Cleanup(func() { setting.DevLogin = false })
	resp = MakeRequest(t, NewRequestWithValues(t, "POST", "/signin/dev", map[string]string{"login": "ed", "next": "/admin"}), http.StatusSeeOther)
	assert.Equal(t, "/finance/admin", resp.Header().Get("Location"))
	resp = MakeRequest(t, as(NewRequest(t, "GET", "/admin/expenses/1"), "ed"), http.StatusOK)
	doc = NewHTMLParser(t, resp.Body)
	doc.AssertElement(t, `form[hx-post="/finance/admin/expenses/1/payments"]`, true)
	doc.AssertElement(t, `a[href="/finance/admin/expenses/1/edit"]`, true)
}

func TestPasswordSignIn(t *testing.T) {
	tests.PrepareTestEnv(t)
	auth_service.ResetAttempts()
	post := func(login, password, ip string) *httptest.ResponseRecorder {
		req := NewRequestWithValues(t, "POST", "/signin/password", map[string]string{"login": login, "password": password, "next": "/admin/expenses"})
		req.Header.Set("X-Forwarded-For", ip)
		return MakeRequest(t, req, NoExpectedStatus)
	}
	assert.Equal(t, http.StatusNotFound, post("boss", "x", "10.0.0.1").Code, "off without a configured password")
	resp := MakeRequest(t, NewRequest(t, "GET", "/signin"), http.StatusOK)
	NewHTMLParser(t, resp.Body).AssertElement(t, "#password-form", false)

	setting.PasswordLogin, setting.Password = "boss", "test-only-password-123"
	t.Cleanup(func() { setting.PasswordLogin, setting.Password = "", ""; auth_service.ResetAttempts() })
	resp = MakeRequest(t, NewRequest(t, "GET", "/signin"), http.StatusOK)
	NewHTMLParser(t, resp.Body).AssertElement(t, "#password-form", true)

	assert.Equal(t, http.StatusForbidden, post("boss", "wrong", "10.0.0.1").Code)
	assert.Equal(t, http.StatusForbidden, post("ed", "test-only-password-123", "10.0.0.1").Code, "the password is for the configured login only")

	ok := post(" BOSS ", "test-only-password-123", "10.0.0.1")
	require.Equal(t, http.StatusSeeOther, ok.Code)
	assert.Equal(t, "/admin/expenses", ok.Header().Get("Location"))
	var session *http.Cookie
	for _, c := range ok.Result().Cookies() {
		if c.Name == "dash_session" {
			session = c
		}
	}
	require.NotNil(t, session)
	req := NewRequest(t, "GET", "/admin/members")
	req.AddCookie(session)
	MakeRequest(t, req, http.StatusOK)

	// ten failures lock the address out, even for the right password; other addresses are not affected
	for range 8 {
		post("boss", "guess", "10.0.0.2")
	}
	assert.Equal(t, http.StatusForbidden, post("boss", "guess", "10.0.0.2").Code)
	assert.Equal(t, http.StatusForbidden, post("boss", "guess", "10.0.0.2").Code)
	assert.Equal(t, http.StatusTooManyRequests, post("boss", "test-only-password-123", "10.0.0.2").Code)
	assert.Equal(t, http.StatusSeeOther, post("boss", "test-only-password-123", "10.0.0.3").Code)
}

func TestExportImport(t *testing.T) {
	tests.PrepareTestEnv(t)
	ctx := t.Context()

	MakeRequest(t, as(NewRequest(t, "GET", "/admin/settings/export"), "ed"), http.StatusForbidden)
	resp := MakeRequest(t, as(NewRequest(t, "GET", "/admin/settings/export"), "boss"), http.StatusOK)
	assert.Contains(t, resp.Header().Get("Content-Disposition"), "attachment")
	dump := resp.Body.Bytes()

	upload := func(body []byte) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		fw, err := w.CreateFormFile("dump", "dash.json")
		require.NoError(t, err)
		_, _ = fw.Write(body)
		require.NoError(t, w.Close())
		req := as(NewRequestWithBody(t, "POST", "/admin/settings/import", &buf), "boss")
		req.Header.Set("Content-Type", w.FormDataContentType())
		return MakeRequest(t, req, NoExpectedStatus)
	}
	resp = upload(dump)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
	assert.Contains(t, resp.Body.String(), "пустую базу")
	assert.Equal(t, http.StatusUnprocessableEntity, upload([]byte("<html>")).Code)

	// empty the data, keep the categories: they are matched by slug
	for id := int64(1); id <= 5; id++ {
		require.NoError(t, expense_model.Delete(ctx, id))
	}
	for id := int64(1); id <= 4; id++ {
		require.NoError(t, donation_model.Delete(ctx, id))
	}
	require.NoError(t, category_model.Delete(ctx, 3))

	resp = upload(dump)
	require.Equal(t, http.StatusSeeOther, resp.Code, resp.Body.String())
	unittest.AssertCount(t, new(expense_model.Expense), 5)
	unittest.AssertCount(t, new(expense_model.Payment), 3)
	unittest.AssertCount(t, new(donation_model.Donation), 4)
	unittest.AssertCount(t, new(category_model.Category), 3)
	grm := unittest.AssertExistsAndLoadBean(t, &expense_model.Expense{Name: "grm"})
	assert.Equal(t, int64(1), grm.CategoryID, "matched to the existing servers category")
	assert.Equal(t, "2099-01-15", grm.NextDue)
	unittest.AssertCount(t, &expense_model.Payment{ExpenseID: grm.ID}, 2)
	unittest.AssertExistsAndLoadBean(t, &donation_model.Donation{ExternalID: "tx-1", Source: "boosty"})
	unittest.AssertCount(t, new(member_model.Member), 3)
	unittest.AssertExistsAndLoadBean(t, &audit_model.Audit{Action: "data.import"})
}

func TestSignInGoesStraightToForgejo(t *testing.T) {
	tests.PrepareTestEnv(t)
	setting.OAuthClientID, setting.OAuthClientSecret = "client", "secret"
	t.Cleanup(func() { setting.OAuthClientID, setting.OAuthClientSecret = "", "" })

	resp := MakeRequest(t, NewRequest(t, "GET", "/signin?next=%2Fadmin%2Fexpenses"), http.StatusSeeOther)
	assert.Equal(t, "/auth/login?next=%2Fadmin%2Fexpenses", resp.Header().Get("Location"))
	resp = MakeRequest(t, NewRequest(t, "GET", "/auth/login?next=%2Fadmin%2Fexpenses"), http.StatusSeeOther)
	assert.True(t, strings.HasPrefix(resp.Header().Get("Location"), setting.ForgejoURL+"/login/oauth/authorize?"))

	// an error from the callback is shown instead of looping back to Forgejo
	resp = MakeRequest(t, NewRequest(t, "GET", "/signin?error=nope"), http.StatusOK)
	assert.Contains(t, NewHTMLParser(t, resp.Body).Find("#signin-error").Text(), "nope")

	// with another way in the page offers the choice
	setting.PasswordLogin, setting.Password = "boss", "test-only-password-123"
	t.Cleanup(func() { setting.PasswordLogin, setting.Password = "", "" })
	resp = MakeRequest(t, NewRequest(t, "GET", "/signin"), http.StatusOK)
	NewHTMLParser(t, resp.Body).AssertElement(t, "#password-form", true)
}
