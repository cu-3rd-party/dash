// Package rates loads currency rates from the Bank of Russia once a day and
// stores them in the site settings, where they can still be edited by hand.
package rates

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	audit_model "git.cu3rd.ru/cu3rd/dash/models/audit"
	sitesetting_model "git.cu3rd.ru/cu3rd/dash/models/sitesetting"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
)

// Settings keys with the state of the last update.
const (
	KeySourceDate = "rates_source_date" // date of the rates in the source, YYYY-MM-DD
	KeyFetched    = "rates_fetched"     // when they were loaded, unix seconds
)

var client = &http.Client{Timeout: 20 * time.Second}

type valCurs struct {
	Date    string `xml:"Date,attr"`
	Valutes []struct {
		CharCode  string `xml:"CharCode"`
		Nominal   string `xml:"Nominal"`
		Value     string `xml:"Value"`
		VunitRate string `xml:"VunitRate"`
	} `xml:"Valute"`
}

// Parse reads the Bank of Russia daily XML (XML_daily.asp): roubles per one unit
// of every currency we use, and the date of the rates.
func Parse(r io.Reader) (money.Rates, string, error) {
	dec := xml.NewDecoder(r)
	// the feed is windows-1251 and Go's decoder accepts only valid UTF-8. Only the ASCII
	// fields (codes and numbers) are read, so the non-ASCII bytes of the Russian names
	// are masked instead of pulling in a charset library.
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return asciiOnly{in}, nil }
	var doc valCurs
	if err := dec.Decode(&doc); err != nil {
		return nil, "", fmt.Errorf("parse rates: %w", err)
	}
	date, err := time.Parse("02.01.2006", doc.Date)
	if err != nil {
		return nil, "", fmt.Errorf("parse rates date %q: %w", doc.Date, err)
	}
	out := money.Rates{}
	for _, v := range doc.Valutes {
		rate, err := perUnit(v.VunitRate, v.Value, v.Nominal)
		if err != nil {
			return nil, "", fmt.Errorf("rate of %s: %w", v.CharCode, err)
		}
		out[v.CharCode] = rate
	}
	for _, c := range money.Currencies {
		if c != money.Base && out[c] <= 0 {
			return nil, "", fmt.Errorf("no rate for %s in the feed", c)
		}
	}
	return out, date.Format(money.DateLayout), nil
}

// asciiOnly replaces every byte above 0x7f with '?'.
type asciiOnly struct{ r io.Reader }

func (a asciiOnly) Read(p []byte) (int, error) {
	n, err := a.r.Read(p)
	for i := range p[:n] {
		if p[i] > 0x7f {
			p[i] = '?'
		}
	}
	return n, err
}

func number(s string) (float64, error) {
	return strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", "."), 64)
}

// perUnit prefers VunitRate (per one unit) and falls back to Value / Nominal.
func perUnit(unit, value, nominal string) (float64, error) {
	if unit != "" {
		return number(unit)
	}
	v, err := number(value)
	if err != nil {
		return 0, err
	}
	n, err := number(nominal)
	if err != nil || n <= 0 {
		return 0, errors.New("bad nominal " + nominal)
	}
	return v / n, nil
}

// Fetch downloads and parses the feed.
func Fetch(ctx context.Context, url string) (money.Rates, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "cu3rd-dash (+https://git.cu3rd.ru/cu3rd/dash)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("rates feed: status %d", resp.StatusCode)
	}
	return Parse(io.LimitReader(resp.Body, 1<<20))
}

// Update loads the rates and stores those of our currencies; actor is who asked
// ("system" for the daily timer). It returns the date of the rates.
func Update(ctx context.Context, actor string) (string, error) {
	if setting.RatesURL == "" {
		return "", errors.New("загрузка курсов выключена (DASH_RATES_URL пуст)")
	}
	r, date, err := Fetch(ctx, setting.RatesURL)
	if err != nil {
		return "", err
	}
	values := map[string]string{
		KeySourceDate: date,
		KeyFetched:    strconv.FormatInt(time.Now().Unix(), 10),
	}
	var details []string
	for _, c := range money.Currencies {
		if c == money.Base {
			continue
		}
		v := strconv.FormatFloat(r[c], 'f', 4, 64)
		values[sitesetting_model.KeyRatePrefix+c] = v
		details = append(details, c+" "+v)
	}
	if err := sitesetting_model.Set(ctx, values); err != nil {
		return "", err
	}
	audit_model.Record(ctx, actor, "rates.update", "курс ЦБ на "+date+": "+strings.Join(details, ", "))
	return date, nil
}
