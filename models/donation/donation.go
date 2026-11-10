// Package donation records incoming donations, entered by hand or received from a payment provider.
package donation

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"git.cu3rd.ru/cu3rd/dash/models/db"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
)

// SourceManual marks donations entered in the admin pages.
const SourceManual = "manual"

// Donation is one incoming donation. An empty Donor is anonymous.
type Donation struct {
	ID        int64  `xorm:"pk autoincr"`
	Donor     string `xorm:"VARCHAR(100) NOT NULL DEFAULT ''"`
	Amount    int64  `xorm:"NOT NULL"`
	Currency  string `xorm:"VARCHAR(3) NOT NULL"`
	DonatedOn string `xorm:"VARCHAR(10) NOT NULL INDEX"`
	Source    string `xorm:"VARCHAR(40) NOT NULL DEFAULT 'manual' INDEX(source_external)"`
	// ExternalID is the provider's transaction id: a repeated webhook does not add a duplicate.
	ExternalID  string `xorm:"VARCHAR(200) NOT NULL DEFAULT '' INDEX(source_external)"`
	Message     string `xorm:"VARCHAR(500) NOT NULL DEFAULT ''"`
	IsPublic    bool   `xorm:"NOT NULL DEFAULT true"`
	CreatedBy   string `xorm:"VARCHAR(40) NOT NULL DEFAULT ''"`
	CreatedUnix int64  `xorm:"created"`
}

func init() {
	db.RegisterModel(new(Donation))
}

// DisplayName is the donor's name or "Аноним".
func (d *Donation) DisplayName() string {
	if d.Donor == "" {
		return "Аноним"
	}
	return d.Donor
}

// ErrDonationNotExist is returned for an unknown id.
type ErrDonationNotExist struct{ ID int64 }

func (err ErrDonationNotExist) Error() string {
	return fmt.Sprintf("донат #%d не найден", err.ID)
}
func (err ErrDonationNotExist) Unwrap() error { return db.ErrNotExist }

// ErrInvalidDonation is returned for bad input.
type ErrInvalidDonation struct{ Reason string }

func (err ErrInvalidDonation) Error() string { return err.Reason }
func (err ErrInvalidDonation) Unwrap() error { return db.ErrInvalidArgument }

// Validate normalizes and checks the fields.
func (d *Donation) Validate() error {
	d.Donor = strings.TrimSpace(d.Donor)
	d.Source = strings.TrimSpace(d.Source)
	d.ExternalID = strings.TrimSpace(d.ExternalID)
	d.Message = strings.TrimSpace(d.Message)
	if d.Source == "" {
		d.Source = SourceManual
	}
	switch {
	case d.Amount <= 0:
		return ErrInvalidDonation{Reason: "сумма доната должна быть больше нуля"}
	case !money.ValidCurrency(d.Currency):
		return ErrInvalidDonation{Reason: "неизвестная валюта"}
	case !validDate(d.DonatedOn):
		return ErrInvalidDonation{Reason: "дата в формате ГГГГ-ММ-ДД"}
	case utf8.RuneCountInString(d.Donor) > 100 || utf8.RuneCountInString(d.Message) > 500 || len(d.Source) > 40 || len(d.ExternalID) > 200:
		return ErrInvalidDonation{Reason: "слишком длинное значение в одном из полей"}
	}
	return nil
}

func validDate(s string) bool {
	_, ok := money.ParseDate(s)
	return ok
}

// Create inserts d. With an external id it is idempotent: if the provider already
// sent this transaction, the existing row is returned and created is false.
func Create(ctx context.Context, d *Donation) (created bool, err error) {
	if err := d.Validate(); err != nil {
		return false, err
	}
	err = db.WithTx(ctx, func(ctx context.Context) error {
		if d.ExternalID != "" {
			existing := new(Donation)
			has, err := db.GetEngine(ctx).Where("source = ? AND external_id = ?", d.Source, d.ExternalID).Get(existing)
			if err != nil || has {
				*d = *existing
				return err
			}
		}
		created = true
		_, err := db.GetEngine(ctx).Insert(d)
		return err
	})
	return created, err
}

// Get returns a donation.
func Get(ctx context.Context, id int64) (*Donation, error) {
	d := new(Donation)
	has, err := db.GetEngine(ctx).ID(id).Get(d)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrDonationNotExist{ID: id}
	}
	return d, nil
}

// List returns donations, newest first; publicOnly hides those marked private.
func List(ctx context.Context, publicOnly bool) ([]*Donation, error) {
	list := make([]*Donation, 0, 32)
	sess := db.GetEngine(ctx).Desc("donated_on", "id")
	if publicOnly {
		sess = sess.Where("is_public = ?", true)
	}
	return list, sess.Find(&list)
}

// Update saves the editable fields.
func Update(ctx context.Context, d *Donation) error {
	if err := d.Validate(); err != nil {
		return err
	}
	_, err := db.GetEngine(ctx).ID(d.ID).Cols("donor", "amount", "currency", "donated_on", "source", "message", "is_public").Update(d)
	return err
}

// Delete removes a donation.
func Delete(ctx context.Context, id int64) error {
	_, err := db.GetEngine(ctx).ID(id).Delete(new(Donation))
	return err
}
