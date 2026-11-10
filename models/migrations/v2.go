package migrations

import "xorm.io/xorm"

func v2AddMemberTelegram(x *xorm.Engine) error {
	type Member struct {
		ID       int64  `xorm:"pk autoincr"`
		Telegram string `xorm:"VARCHAR(32) NOT NULL DEFAULT ''"`
	}
	return x.Sync(new(Member))
}
