// Package backup moves the data between instances as JSON: `dash export` dumps a
// database (for example the SQLite of a stand), the owner uploads the file on the
// settings page of an empty instance. Members are not carried over: a login that
// was a demo account in one place may be a real Forgejo user in another.
package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	category_model "git.cu3rd.ru/cu3rd/dash/models/category"
	"git.cu3rd.ru/cu3rd/dash/models/db"
	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	expense_model "git.cu3rd.ru/cu3rd/dash/models/expense"
	sitesetting_model "git.cu3rd.ru/cu3rd/dash/models/sitesetting"
)

// Version of the dump format.
const Version = 1

// Dump is everything that is moved.
type Dump struct {
	Version    int                        `json:"version"`
	Categories []*category_model.Category `json:"categories"`
	Expenses   []*expense_model.Expense   `json:"expenses"`
	Payments   []*expense_model.Payment   `json:"payments"`
	Donations  []*donation_model.Donation `json:"donations"`
	Settings   map[string]string          `json:"settings"`
}

// ErrInvalidDump is returned for a file that cannot be imported.
type ErrInvalidDump struct{ Reason string }

func (err ErrInvalidDump) Error() string { return err.Reason }
func (err ErrInvalidDump) Unwrap() error { return db.ErrInvalidArgument }

// Export collects the data.
func Export(ctx context.Context) (*Dump, error) {
	d := &Dump{Version: Version}
	var err error
	if d.Categories, err = category_model.List(ctx); err != nil {
		return nil, err
	}
	if d.Expenses, err = expense_model.List(ctx); err != nil {
		return nil, err
	}
	if d.Payments, err = expense_model.ListPayments(ctx, 0); err != nil {
		return nil, err
	}
	if d.Donations, err = donation_model.List(ctx, false); err != nil {
		return nil, err
	}
	if d.Settings, err = sitesetting_model.All(ctx); err != nil {
		return nil, err
	}
	return d, nil
}

// Write encodes d as indented JSON.
func Write(w io.Writer, d *Dump) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(d)
}

// Summary describes what an import added.
type Summary struct {
	Categories, Expenses, Payments, Donations int
}

func (s Summary) String() string {
	return fmt.Sprintf("категорий: %d, статей: %d, оплат: %d, донатов: %d", s.Categories, s.Expenses, s.Payments, s.Donations)
}

// Import reads a dump into an instance that has no expenses, payments or donations
// yet, so nothing is overwritten. Categories are matched by slug; ids are new.
func Import(ctx context.Context, r io.Reader) (Summary, error) {
	var d Dump
	dec := json.NewDecoder(r)
	if err := dec.Decode(&d); err != nil {
		return Summary{}, ErrInvalidDump{Reason: "это не файл экспорта dash: " + err.Error()}
	}
	if d.Version != Version {
		return Summary{}, ErrInvalidDump{Reason: fmt.Sprintf("неизвестная версия файла %d", d.Version)}
	}
	var sum Summary
	err := db.WithTx(ctx, func(ctx context.Context) error {
		if err := ensureEmpty(ctx); err != nil {
			return err
		}
		existing, err := category_model.List(ctx)
		if err != nil {
			return err
		}
		bySlug := map[string]int64{}
		for _, c := range existing {
			bySlug[c.Slug] = c.ID
		}
		catIDs := map[int64]int64{}
		for _, c := range d.Categories {
			if id, ok := bySlug[strings.ToLower(c.Slug)]; ok {
				catIDs[c.ID] = id
				continue
			}
			old := c.ID
			c.ID = 0
			if err := category_model.Create(ctx, c); err != nil {
				return fmt.Errorf("категория %q: %w", c.Name, err)
			}
			catIDs[old], bySlug[c.Slug] = c.ID, c.ID
			sum.Categories++
		}
		expIDs := map[int64]int64{}
		for _, e := range d.Expenses {
			old := e.ID
			cat, ok := catIDs[e.CategoryID]
			if !ok {
				return ErrInvalidDump{Reason: fmt.Sprintf("статья %q ссылается на неизвестную категорию", e.Name)}
			}
			e.ID, e.CategoryID = 0, cat
			if err := expense_model.Create(ctx, e); err != nil {
				return fmt.Errorf("статья %q: %w", e.Name, err)
			}
			expIDs[old] = e.ID
			sum.Expenses++
		}
		for _, p := range d.Payments {
			exp, ok := expIDs[p.ExpenseID]
			if !ok {
				return ErrInvalidDump{Reason: fmt.Sprintf("оплата #%d ссылается на неизвестную статью", p.ID)}
			}
			p.ID, p.ExpenseID = 0, exp
			if err := expense_model.CreatePayment(ctx, p); err != nil {
				return fmt.Errorf("оплата от %s: %w", p.PaidOn, err)
			}
			sum.Payments++
		}
		for _, dn := range d.Donations {
			dn.ID = 0
			if _, err := donation_model.Create(ctx, dn); err != nil {
				return fmt.Errorf("донат от %s: %w", dn.DonatedOn, err)
			}
			sum.Donations++
		}
		if len(d.Settings) > 0 {
			return sitesetting_model.Set(ctx, d.Settings)
		}
		return nil
	})
	return sum, err
}

func ensureEmpty(ctx context.Context) error {
	expenses, err := expense_model.List(ctx)
	if err != nil {
		return err
	}
	payments, err := expense_model.ListPayments(ctx, 0)
	if err != nil {
		return err
	}
	donations, err := donation_model.List(ctx, false)
	if err != nil {
		return err
	}
	if len(expenses)+len(payments)+len(donations) > 0 {
		return ErrInvalidDump{Reason: "импорт возможен только в пустую базу: здесь уже есть статьи, оплаты или донаты"}
	}
	return nil
}
