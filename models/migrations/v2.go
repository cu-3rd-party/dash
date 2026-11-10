package migrations

import "xorm.io/xorm"

// v2AddMemberTelegram: the feature was removed, but the migration stays: deployed databases are already at version 2
// and Migrate refuses a database newer than the binary. The column is unused.
func v2AddMemberTelegram(x *xorm.Engine) error {
	type Member struct {
		ID       int64  `xorm:"pk autoincr"`
		Telegram string `xorm:"VARCHAR(32) NOT NULL DEFAULT ''"`
	}
	return x.Sync(new(Member))
}
