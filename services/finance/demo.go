package finance

import (
	"context"
	"time"

	"git.cu3rd.ru/cu3rd/dash/models/db"
	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	expense_model "git.cu3rd.ru/cu3rd/dash/models/expense"
	member_model "git.cu3rd.ru/cu3rd/dash/models/member"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
)

// SeedDemo fills an empty database with sample data for local runs (DASH_SEED_DEMO=true).
func SeedDemo(ctx context.Context) error {
	existing, err := expense_model.List(ctx)
	if err != nil || len(existing) > 0 {
		return err
	}
	day := func(offset int) string { return money.Today(time.Now()).AddDate(0, 0, offset).Format(money.DateLayout) }
	type demo struct {
		e        *expense_model.Expense
		payments []*expense_model.Payment
	}
	demos := []demo{
		{
			&expense_model.Expense{CategoryID: 1, Name: "grm — выделенный сервер", Description: "Основной узел кластера: Postgres, Forgejo, большинство прод-сервисов.", Owner: "admin", Provider: "Hetzner", URL: "https://robot.hetzner.com", Amount: 4490, Currency: "EUR", Period: "month", NextDue: day(5), IsActive: true, IsPublic: true},
			[]*expense_model.Payment{{PaidOn: day(-56), Amount: 4490, Currency: "EUR", Periods: 1}, {PaidOn: day(-25), Amount: 4490, Currency: "EUR", Periods: 1}},
		},
		{
			&expense_model.Expense{CategoryID: 1, Name: "nxs — VPS", Description: "Второй узел: Traefik для nxs и xmsk, CI-раннеры.", Owner: "admin", Provider: "Timeweb", Amount: 150000, Currency: "RUB", Period: "month", NextDue: day(-2), IsActive: true, IsPublic: true},
			[]*expense_model.Payment{{PaidOn: day(-32), Amount: 150000, Currency: "RUB", Periods: 1}},
		},
		{&expense_model.Expense{CategoryID: 1, Name: "Бэкапы (S3, холодное хранилище)", Description: "Ночные бэкапы баз и репозиториев.", Owner: "editor", Provider: "Selectel", Amount: 35000, Currency: "RUB", Period: "month", NextDue: day(20), IsActive: true}, nil},
		{
			&expense_model.Expense{CategoryID: 2, Name: "cu3rd.ru", Description: "Основной домен: git, ai, сервисы.", Owner: "admin", Provider: "REG.RU", Amount: 79000, Currency: "RUB", Period: "year", NextDue: day(140), IsActive: true, IsPublic: true},
			[]*expense_model.Payment{{PaidOn: day(-225), Amount: 79000, Currency: "RUB", Periods: 1, Note: "продление на год"}},
		},
		{&expense_model.Expense{CategoryID: 2, Name: "cu3rd.dev", Description: "Резервный домен под dev-стенды.", Owner: "editor", Provider: "Cloudflare Registrar", Amount: 1218, Currency: "USD", Period: "year", NextDue: day(11), IsActive: true, IsPublic: true}, nil},
	}
	return db.WithTx(ctx, func(ctx context.Context) error {
		for _, d := range demos {
			if err := expense_model.Create(ctx, d.e); err != nil {
				return err
			}
			for _, p := range d.payments {
				p.ExpenseID, p.CreatedBy = d.e.ID, "seed"
				if err := expense_model.CreatePayment(ctx, p); err != nil {
					return err
				}
			}
		}
		for _, d := range []*donation_model.Donation{
			{Donor: "Вася Пупкин", Amount: 50000, Currency: "RUB", DonatedOn: day(-3), Message: "Спасибо за сервисы!", IsPublic: true},
			{Amount: 100000, Currency: "RUB", DonatedOn: day(-10), IsPublic: true},
			{Donor: "kot", Amount: 1000, Currency: "USD", DonatedOn: day(-40), Message: "на домены", IsPublic: true},
			{Donor: "Аня", Amount: 30000, Currency: "RUB", DonatedOn: day(-70)},
		} {
			d.CreatedBy = "seed"
			if _, err := donation_model.Create(ctx, d); err != nil {
				return err
			}
		}
		for _, m := range []*member_model.Member{
			{Login: "admin", Name: "Демо-владелец", Role: member_model.RoleOwner},
			{Login: "editor", Name: "Демо-редактор", Role: member_model.RoleEditor},
			{Login: "viewer", Name: "Демо-наблюдатель", Role: member_model.RoleViewer},
		} {
			m.CreatedBy = "seed"
			if err := member_model.Upsert(ctx, m); err != nil {
				return err
			}
		}
		return nil
	})
}
