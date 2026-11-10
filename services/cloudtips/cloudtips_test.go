package cloudtips_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	sitesetting_model "git.cu3rd.ru/cu3rd/dash/models/sitesetting"
	"git.cu3rd.ru/cu3rd/dash/models/unittest"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
	cloudtips_service "git.cu3rd.ru/cu3rd/dash/services/cloudtips"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const timelineBody = `{"data":{"items":[
 {"transactionId":2,"createdDate":"2026-11-10T19:51:18.166+03:00","paymentAmount":200.0,"currencyCode":"RUB","operationType":"Payout","payerName":null},
 {"transactionId":1,"createdDate":"2026-11-10T23:51:16.258+03:00","paymentAmount":200.5,"currencyCode":"RUB","operationType":"Transaction","payerName":"Вася","payerFeedback":null,"comment":"спасибо"}
]},"succeed":true}`

func TestSync(t *testing.T) {
	unittest.PrepareTestDatabase(t)
	var refreshes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/connect/token":
			require.NoError(t, r.ParseForm())
			refreshes = append(refreshes, r.PostForm.Get("refresh_token"))
			next := "rotated-" + string(rune('0'+len(refreshes)))
			_, _ = w.Write([]byte(`{"access_token":"AT","refresh_token":"` + next + `"}`))
		case "/api/timeline":
			assert.Equal(t, "Bearer AT", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(timelineBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	oldAPI, oldID, oldTok := setting.CloudTipsAPIURL, setting.CloudTipsIdentityURL, setting.CloudTipsRefreshToken
	setting.CloudTipsAPIURL, setting.CloudTipsIdentityURL, setting.CloudTipsRefreshToken = srv.URL+"/api", srv.URL, "seed-1"
	t.Cleanup(func() {
		setting.CloudTipsAPIURL, setting.CloudTipsIdentityURL, setting.CloudTipsRefreshToken = oldAPI, oldID, oldTok
	})

	n, err := cloudtips_service.Sync(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the payout is not a donation")
	d := unittest.AssertExistsAndLoadBean(t, &donation_model.Donation{Source: "cloudtips", ExternalID: "1"})
	assert.Equal(t, "Вася", d.Donor)
	assert.Equal(t, int64(20050), d.Amount)
	assert.Equal(t, "2026-11-10", d.DonatedOn)
	assert.Equal(t, "спасибо", d.Message)

	// the rotated token is used next time, and a repeat does not duplicate the donation
	n, err = cloudtips_service.Sync(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, []string{"seed-1", "rotated-1"}, refreshes)
	unittest.AssertCount(t, &donation_model.Donation{Source: "cloudtips"}, 1)

	// a new token pasted into the env replaces the stored one
	setting.CloudTipsRefreshToken = "seed-2"
	_, err = cloudtips_service.Sync(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "seed-2", refreshes[2])
	all, err := sitesetting_model.All(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "rotated-3", all[cloudtips_service.KeyRefresh])
}
