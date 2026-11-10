package migrations

import "xorm.io/xorm"

// v3CreateCredential adds the table of provider secrets and moves the CloudTips refresh token
// there from the site settings, where its first version kept it.
func v3CreateCredential(x *xorm.Engine) error {
	type Credential struct {
		ID          int64  `xorm:"pk autoincr"`
		Name        string `xorm:"VARCHAR(64) NOT NULL UNIQUE"`
		Value       string `xorm:"TEXT NOT NULL"`
		Seed        string `xorm:"TEXT NOT NULL DEFAULT ''"`
		UpdatedUnix int64  `xorm:"updated"`
	}
	type SiteSetting struct {
		ID    int64  `xorm:"pk autoincr"`
		Name  string `xorm:"VARCHAR(64) NOT NULL UNIQUE"`
		Value string `xorm:"TEXT"`
	}
	if err := x.Sync(new(Credential)); err != nil {
		return err
	}
	_, err := x.Transaction(func(s *xorm.Session) (any, error) {
		var rows []SiteSetting
		if err := s.In("name", "cloudtips_refresh", "cloudtips_seed").Find(&rows); err != nil {
			return nil, err
		}
		old := map[string]string{}
		for _, r := range rows {
			old[r.Name] = r.Value
		}
		if old["cloudtips_refresh"] != "" {
			if _, err := s.Insert(&Credential{Name: "cloudtips", Value: old["cloudtips_refresh"], Seed: old["cloudtips_seed"]}); err != nil {
				return nil, err
			}
		}
		_, err := s.In("name", "cloudtips_refresh", "cloudtips_seed").Delete(&SiteSetting{})
		return nil, err
	})
	return err
}
