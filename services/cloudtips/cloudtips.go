// Package cloudtips pulls donations from the CloudTips receiver account. CloudTips has no
// public API or webhook, so this uses what lk.cloudtips.ru itself calls: an OAuth refresh
// token (IdentityServer, scope offline_access) is exchanged for a short-lived access token,
// which reads the payment timeline. The refresh token rotates, so the current one lives in
// the database; DASH_CLOUDTIPS_REFRESH_TOKEN only bootstraps it.
package cloudtips

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	audit_model "git.cu3rd.ru/cu3rd/dash/models/audit"
	credential_model "git.cu3rd.ru/cu3rd/dash/models/credential"
	"git.cu3rd.ru/cu3rd/dash/models/db"
	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
)

// Source is the donation source of everything imported here.
const Source = "cloudtips"

const (
	// clientID is the client lk.cloudtips.ru signs in with (the "client_id" claim of its tokens).
	clientID = "MobilePhone"
	pageSize = 50
)

var client = &http.Client{Timeout: 20 * time.Second}

// Enabled reports whether a refresh token is configured.
func Enabled() bool { return setting.CloudTipsRefreshToken != "" }

// accessToken exchanges a refresh token and stores the rotated one before anything else can
// fail: a used refresh token is dead. The stored token is tried first. The environment one
// is a fallback for when it is gone, and only when it differs from the one the stored token
// came from: a redeploy with an old file must not bring back a token that was used up.
func accessToken(ctx context.Context) (string, error) {
	type attempt struct{ token, seed string }
	var attempts []attempt
	env := setting.CloudTipsRefreshToken
	stored, err := credential_model.Get(ctx, Source)
	switch {
	case err == nil:
		attempts = append(attempts, attempt{stored.Value, stored.Seed})
		if env != "" && env != stored.Seed && env != stored.Value {
			attempts = append(attempts, attempt{env, env})
		}
	case db.IsErrNotExist(err):
		attempts = append(attempts, attempt{env, env})
	default:
		return "", err
	}
	var errs []error
	for _, a := range attempts {
		access, refresh, err := exchange(ctx, a.token)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		return access, credential_model.Save(ctx, Source, refresh, a.seed)
	}
	return "", errors.Join(errs...)
}

// exchange trades a refresh token for an access token and the next refresh token.
func exchange(ctx context.Context, refresh string) (access, next string, err error) {
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, setting.CloudTipsIdentityURL+"/connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var tok struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
	}
	if err := do(req, &tok); err != nil {
		return "", "", fmt.Errorf("cloudtips token: %w", err)
	}
	if tok.Access == "" {
		return "", "", errors.New("cloudtips token: no access_token in the response")
	}
	if tok.Refresh == "" {
		tok.Refresh = refresh // not rotated
	}
	return tok.Access, tok.Refresh, nil
}

// do sends req and decodes the JSON body even for 4xx, where IdentityServer explains itself.
func do(req *http.Request, v any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if jerr := json.Unmarshal(body, v); resp.StatusCode != http.StatusOK {
		if jerr != nil {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)[:min(len(body), 200)]))
	} else if jerr != nil {
		return jerr
	}
	return nil
}

type item struct {
	TransactionID int64       `json:"transactionId"`
	CreatedDate   string      `json:"createdDate"`
	Amount        json.Number `json:"paymentAmount"`
	Currency      string      `json:"currencyCode"`
	Comment       *string     `json:"comment"`
	PayerName     *string     `json:"payerName"`
	PayerFeedback *string     `json:"payerFeedback"`
	Operation     string      `json:"operationType"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// Donation turns a timeline item into a donation; payouts to the card are not donations.
func (it item) donation() (*donation_model.Donation, bool, error) {
	if it.Operation != "Transaction" {
		return nil, false, nil
	}
	amount, err := money.Parse(it.Amount.String())
	if err != nil {
		return nil, false, fmt.Errorf("cloudtips transaction %d: amount: %w", it.TransactionID, err)
	}
	paid, err := time.Parse(time.RFC3339, it.CreatedDate)
	if err != nil {
		return nil, false, fmt.Errorf("cloudtips transaction %d: date: %w", it.TransactionID, err)
	}
	msg := str(it.PayerFeedback)
	if msg == "" {
		msg = str(it.Comment)
	}
	return &donation_model.Donation{
		Donor: str(it.PayerName), Amount: amount, Currency: strings.ToUpper(it.Currency),
		DonatedOn: money.Today(paid).Format(money.DateLayout),
		Source:    Source, ExternalID: fmt.Sprint(it.TransactionID),
		Message: msg, IsPublic: true, CreatedBy: "system",
	}, true, nil
}

// timeline reads every item from the start of the previous month: payments of the last
// days of a month must not be lost at the turn, and the import is idempotent anyway.
func timeline(ctx context.Context, token string, now time.Time) ([]item, error) {
	n := now.In(money.Moscow)
	from := time.Date(n.Year(), n.Month()-1, 1, 0, 0, 0, 0, money.Moscow)
	to := time.Date(n.Year(), n.Month()+1, 1, 0, 0, 0, 0, money.Moscow)
	var all []item
	for page := 1; ; page++ {
		q := url.Values{
			"page": {fmt.Sprint(page)}, "limit": {fmt.Sprint(pageSize)},
			"dateFrom": {from.Format(time.RFC3339)}, "dateTo": {to.Format(time.RFC3339)},
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, setting.CloudTipsAPIURL+"/timeline?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		var out struct {
			Data struct {
				Items []item `json:"items"`
			} `json:"data"`
		}
		if err := do(req, &out); err != nil {
			return nil, fmt.Errorf("cloudtips timeline: %w", err)
		}
		all = append(all, out.Data.Items...)
		if len(out.Data.Items) < pageSize {
			return all, nil
		}
	}
}

// Sync imports new donations and returns how many were created.
func Sync(ctx context.Context) (int, error) {
	token, err := accessToken(ctx)
	if err != nil {
		return 0, err
	}
	items, err := timeline(ctx, token, time.Now())
	if err != nil {
		return 0, err
	}
	var created int
	var errs []error
	for _, it := range items {
		d, ok, err := it.donation()
		if err == nil && ok {
			var isNew bool
			if isNew, err = donation_model.Create(ctx, d); isNew {
				created++
				audit_model.Record(ctx, "system", "donation.create", money.Format(d.Amount, d.Currency)+" от "+d.DisplayName()+" (CloudTips)")
			}
		}
		errs = append(errs, err)
	}
	return created, errors.Join(errs...)
}
