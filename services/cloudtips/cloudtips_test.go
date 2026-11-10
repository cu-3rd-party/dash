package cloudtips_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	credential_model "git.cu3rd.ru/cu3rd/dash/models/credential"
	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
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

// provider is a fake IdentityServer + API: refresh tokens are one-time, only the issued ones work.
type provider struct {
	valid     map[string]bool
	refreshes []string
}

func (p *provider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/connect/token":
		_ = r.ParseForm()
		tok := r.PostForm.Get("refresh_token")
		p.refreshes = append(p.refreshes, tok)
		if !p.valid[tok] {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		delete(p.valid, tok)
		next := "rotated-" + string(rune('0'+len(p.refreshes)))
		p.valid[next] = true
		_, _ = w.Write([]byte(`{"access_token":"AT","refresh_token":"` + next + `"}`))
	case "/api/timeline":
		_, _ = w.Write([]byte(timelineBody))
	default:
		http.NotFound(w, r)
	}
}

func TestSync(t *testing.T) {
	unittest.PrepareTestDatabase(t)
	p := &provider{valid: map[string]bool{"seed-1": true}}
	srv := httptest.NewServer(p)
	defer srv.Close()
	oldAPI, oldID, oldTok := setting.CloudTipsAPIURL, setting.CloudTipsIdentityURL, setting.CloudTipsRefreshToken
	setting.CloudTipsAPIURL, setting.CloudTipsIdentityURL, setting.CloudTipsRefreshToken = srv.URL+"/api", srv.URL, "seed-1"
	t.Cleanup(func() {
		setting.CloudTipsAPIURL, setting.CloudTipsIdentityURL, setting.CloudTipsRefreshToken = oldAPI, oldID, oldTok
	})

	// the environment bootstraps the first sync
	n, err := cloudtips_service.Sync(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the payout is not a donation")
	d := unittest.AssertExistsAndLoadBean(t, &donation_model.Donation{Source: "cloudtips", ExternalID: "1"})
	assert.Equal(t, "Вася", d.Donor)
	assert.Equal(t, int64(20050), d.Amount)
	assert.Equal(t, "2026-11-10", d.DonatedOn)
	assert.Equal(t, "спасибо", d.Message)

	// a redeploy keeps the same, already used environment token: the stored one is used,
	// the dead one is never tried again, and the donation is not duplicated
	n, err = cloudtips_service.Sync(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, []string{"seed-1", "rotated-1"}, p.refreshes)
	unittest.AssertCount(t, &donation_model.Donation{Source: "cloudtips"}, 1)

	// a fresh token pasted into the environment is ignored while the stored one still works...
	p.valid["seed-2"] = true
	setting.CloudTipsRefreshToken = "seed-2"
	_, err = cloudtips_service.Sync(t.Context())
	require.NoError(t, err)
	assert.NotContains(t, p.refreshes[2:], "seed-2")

	// ...and takes over once the stored one is dead
	p.valid = map[string]bool{"seed-2": true}
	_, err = cloudtips_service.Sync(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "seed-2", p.refreshes[len(p.refreshes)-1])
	c, err := credential_model.Get(t.Context(), cloudtips_service.Source)
	require.NoError(t, err)
	assert.Equal(t, "seed-2", c.Seed)
	assert.True(t, p.valid[c.Value], "the rotated token is stored")
}
