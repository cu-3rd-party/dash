package rates_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	audit_model "git.cu3rd.ru/cu3rd/dash/models/audit"
	sitesetting_model "git.cu3rd.ru/cu3rd/dash/models/sitesetting"
	"git.cu3rd.ru/cu3rd/dash/models/unittest"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
	rates_service "git.cu3rd.ru/cu3rd/dash/services/rates"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	unittest.MainTest(m)
}

// Feed is a trimmed XML_daily.asp answer; the names are windows-1251 bytes, as in the real one.
const Feed = `<?xml version="1.0" encoding="windows-1251"?>` +
	`<ValCurs Date="10.11.2026" name="Foreign Currency Market">` +
	`<Valute ID="R01235"><NumCode>840</NumCode><CharCode>USD</CharCode><Nominal>1</Nominal>` +
	"<Name>\xc4\xee\xeb\xeb\xe0\xf0 \xd1\xd8\xc0</Name><Value>82,7536</Value><VunitRate>82,7536</VunitRate></Valute>" +
	`<Valute ID="R01239"><NumCode>978</NumCode><CharCode>EUR</CharCode><Nominal>1</Nominal>` +
	"<Name>\xc5\xe2\xf0\xee</Name><Value>92,3944</Value><VunitRate>92,3944</VunitRate></Valute>" +
	`<Valute ID="R01240"><NumCode>818</NumCode><CharCode>EGP</CharCode><Nominal>10</Nominal>` +
	"<Name>\xc5\xe3\xe8\xef\xe5\xf2</Name><Value>17,2100</Value></Valute>" +
	`</ValCurs>`

func TestParse(t *testing.T) {
	r, date, err := rates_service.Parse(strings.NewReader(Feed))
	require.NoError(t, err)
	assert.Equal(t, "2026-11-10", date)
	assert.InDelta(t, 82.7536, r["USD"], 1e-9)
	assert.InDelta(t, 92.3944, r["EUR"], 1e-9)
	assert.InDelta(t, 1.721, r["EGP"], 1e-9, "without VunitRate: Value / Nominal")

	_, _, err = rates_service.Parse(strings.NewReader(strings.Replace(Feed, "<CharCode>EUR</CharCode>", "<CharCode>XXX</CharCode>", 1)))
	require.ErrorContains(t, err, "EUR", "a feed without one of our currencies is rejected")
	_, _, err = rates_service.Parse(strings.NewReader("<html>DDoS-Guard</html>"))
	require.Error(t, err)
}

func TestUpdate(t *testing.T) {
	unittest.PrepareTestDatabase(t)
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(Feed))
	}))
	defer srv.Close()
	old := setting.RatesURL
	setting.RatesURL = srv.URL
	t.Cleanup(func() { setting.RatesURL = old })

	date, err := rates_service.Update(t.Context(), "system")
	require.NoError(t, err)
	assert.Equal(t, "2026-11-10", date)
	all, err := sitesetting_model.All(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "82.7536", all["rate_USD"])
	assert.Equal(t, "92.3944", all["rate_EUR"])
	assert.Equal(t, "2026-11-10", all[rates_service.KeySourceDate])
	assert.NotEmpty(t, all[rates_service.KeyFetched])
	unittest.AssertExistsAndLoadBean(t, &audit_model.Audit{Actor: "system", Action: "rates.update"})

	// a failed download keeps the previous rates
	status = http.StatusServiceUnavailable
	_, err = rates_service.Update(t.Context(), "system")
	require.Error(t, err)
	all, err = sitesetting_model.All(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "82.7536", all["rate_USD"])
}
