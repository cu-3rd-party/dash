package migrations

import "xorm.io/xorm"

func v1CreateFinanceTables(x *xorm.Engine) error {
	type Member struct {
		ID          int64  `xorm:"pk autoincr"`
		Login       string `xorm:"VARCHAR(40) NOT NULL UNIQUE"`
		Name        string `xorm:"VARCHAR(100) NOT NULL DEFAULT ''"`
		Role        string `xorm:"VARCHAR(16) NOT NULL"`
		CreatedBy   string `xorm:"VARCHAR(40) NOT NULL DEFAULT ''"`
		CreatedUnix int64  `xorm:"created"`
	}
	type Category struct {
		ID          int64  `xorm:"pk autoincr"`
		Name        string `xorm:"VARCHAR(100) NOT NULL"`
		Slug        string `xorm:"VARCHAR(40) NOT NULL UNIQUE"`
		Description string `xorm:"VARCHAR(500) NOT NULL DEFAULT ''"`
		SortOrder   int    `xorm:"NOT NULL DEFAULT 0"`
	}
	type Expense struct {
		ID          int64  `xorm:"pk autoincr"`
		CategoryID  int64  `xorm:"NOT NULL INDEX"`
		Name        string `xorm:"VARCHAR(200) NOT NULL"`
		Description string `xorm:"TEXT"`
		Owner       string `xorm:"VARCHAR(100) NOT NULL DEFAULT ''"`
		Provider    string `xorm:"VARCHAR(200) NOT NULL DEFAULT ''"`
		URL         string `xorm:"VARCHAR(500) NOT NULL DEFAULT ''"`
		Amount      int64  `xorm:"NOT NULL"`
		Currency    string `xorm:"VARCHAR(3) NOT NULL"`
		Period      string `xorm:"VARCHAR(16) NOT NULL"`
		NextDue     string `xorm:"VARCHAR(10) NOT NULL DEFAULT ''"`
		IsActive    bool   `xorm:"NOT NULL DEFAULT true"`
		IsPublic    bool   `xorm:"NOT NULL DEFAULT true"`
		CreatedUnix int64  `xorm:"created"`
		UpdatedUnix int64  `xorm:"updated"`
	}
	type Payment struct {
		ID          int64  `xorm:"pk autoincr"`
		ExpenseID   int64  `xorm:"NOT NULL INDEX"`
		PaidOn      string `xorm:"VARCHAR(10) NOT NULL INDEX"`
		Amount      int64  `xorm:"NOT NULL"`
		Currency    string `xorm:"VARCHAR(3) NOT NULL"`
		Periods     int    `xorm:"NOT NULL DEFAULT 1"`
		Note        string `xorm:"VARCHAR(500) NOT NULL DEFAULT ''"`
		CreatedBy   string `xorm:"VARCHAR(40) NOT NULL DEFAULT ''"`
		CreatedUnix int64  `xorm:"created"`
	}
	type Donation struct {
		ID          int64  `xorm:"pk autoincr"`
		Donor       string `xorm:"VARCHAR(100) NOT NULL DEFAULT ''"`
		Amount      int64  `xorm:"NOT NULL"`
		Currency    string `xorm:"VARCHAR(3) NOT NULL"`
		DonatedOn   string `xorm:"VARCHAR(10) NOT NULL INDEX"`
		Source      string `xorm:"VARCHAR(40) NOT NULL DEFAULT 'manual' INDEX(source_external)"`
		ExternalID  string `xorm:"VARCHAR(200) NOT NULL DEFAULT '' INDEX(source_external)"`
		Message     string `xorm:"VARCHAR(500) NOT NULL DEFAULT ''"`
		IsPublic    bool   `xorm:"NOT NULL DEFAULT true"`
		CreatedBy   string `xorm:"VARCHAR(40) NOT NULL DEFAULT ''"`
		CreatedUnix int64  `xorm:"created"`
	}
	type SiteSetting struct {
		ID    int64  `xorm:"pk autoincr"`
		Name  string `xorm:"VARCHAR(64) NOT NULL UNIQUE"`
		Value string `xorm:"TEXT"`
	}
	type Audit struct {
		ID          int64  `xorm:"pk autoincr"`
		Actor       string `xorm:"VARCHAR(64) NOT NULL"`
		Action      string `xorm:"VARCHAR(64) NOT NULL"`
		Details     string `xorm:"VARCHAR(1000) NOT NULL DEFAULT ''"`
		CreatedUnix int64  `xorm:"INDEX created"`
	}
	if err := x.Sync(new(Member), new(Category), new(Expense), new(Payment), new(Donation), new(SiteSetting), new(Audit)); err != nil {
		return err
	}
	if _, err := x.Insert([]*Category{
		{Name: "Серверы", Slug: "servers", Description: "VPS, выделенные серверы, хостинг", SortOrder: 10},
		{Name: "Домены", Slug: "domains", Description: "Регистрация и продление доменов", SortOrder: 20},
	}); err != nil {
		return err
	}
	_, err := x.Insert([]*SiteSetting{
		{Name: "site_title", Value: "Финансы cu3rd"},
		{Name: "public_intro", Value: "Здесь видно, сколько стоит инфраструктура cu3rd и кто помогает её оплачивать."},
		{Name: "donate_text", Value: "Способ доната скоро появится. Пока можно написать любому из админов."},
		{Name: "rate_USD", Value: "95"},
		{Name: "rate_EUR", Value: "105"},
	})
	return err
}
