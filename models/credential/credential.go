// Package credential keeps the secrets that outlive one request and change by themselves:
// the rotating OAuth refresh tokens of the payment providers we poll. They live in their own
// table, not in site settings, so they never reach templates, backups or exports.
package credential

import (
	"context"
	"fmt"

	"git.cu3rd.ru/cu3rd/dash/models/db"
)

// Credential is the current secret of one provider.
type Credential struct {
	ID   int64  `xorm:"pk autoincr"`
	Name string `xorm:"VARCHAR(64) NOT NULL UNIQUE"`
	// Value is the latest secret, the one to use next.
	Value string `xorm:"TEXT NOT NULL"`
	// Seed is the environment value Value grew from. A different value in the environment
	// is a newer bootstrap, the same one is a redeploy with a stale file.
	Seed        string `xorm:"TEXT NOT NULL DEFAULT ''"`
	UpdatedUnix int64  `xorm:"updated"`
}

func init() {
	db.RegisterModel(new(Credential))
}

// ErrCredentialNotExist is returned for a provider that has no stored secret yet.
type ErrCredentialNotExist struct{ Name string }

func (err ErrCredentialNotExist) Error() string {
	return fmt.Sprintf("credential %q not found", err.Name)
}
func (err ErrCredentialNotExist) Unwrap() error { return db.ErrNotExist }

// Get returns the stored credential of a provider.
func Get(ctx context.Context, name string) (*Credential, error) {
	c := new(Credential)
	has, err := db.GetEngine(ctx).Where("name = ?", name).Get(c)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrCredentialNotExist{Name: name}
	}
	return c, nil
}

// Save stores value and seed as the credential of a provider.
func Save(ctx context.Context, name, value, seed string) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		n, err := db.GetEngine(ctx).Where("name = ?", name).Cols("value", "seed").Update(&Credential{Value: value, Seed: seed})
		if err != nil || n > 0 {
			return err
		}
		_, err = db.GetEngine(ctx).Insert(&Credential{Name: name, Value: value, Seed: seed})
		return err
	})
}
