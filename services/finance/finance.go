// Package finance computes totals across expenses, payments and donations and
// records payments together with the due date they move.
package finance

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"git.cu3rd.ru/cu3rd/dash/models/db"
	donation_model "git.cu3rd.ru/cu3rd/dash/models/donation"
	expense_model "git.cu3rd.ru/cu3rd/dash/models/expense"
	"git.cu3rd.ru/cu3rd/dash/modules/money"
)

// UpcomingDays is how far ahead the dashboard lists due payments.
const UpcomingDays = 45

// Total is a sum over a group of expenses, in roubles.
type Total struct {
	Name    string
	Count   int
	Monthly int64
	Yearly  int64
}

// Share is the part of the monthly total, in percent.
func (t Total) Share(of int64) int {
	if of <= 0 {
		return 0
	}
	return int(math.Round(float64(t.Monthly) * 100 / float64(of)))
}

// Row is an expense with its values in roubles and its state on a date.
type Row struct {
	*expense_model.Expense
	Monthly int64
	Status  expense_model.Status
	Days    int
}

// Group is the expenses of one category.
type Group struct {
	Category string
	Rows     []*Row
	Monthly  int64
}

// Share is the part of the monthly total, in percent.
func (g Group) Share(of int64) int { return Total{Monthly: g.Monthly}.Share(of) }

// Summary is everything the overview pages show. All money is in rouble kopecks.
type Summary struct {
	MonthlyTotal   int64
	YearlyTotal    int64
	ActiveCount    int
	HiddenCount    int
	ByCategory     []Total
	ByOwner        []Total
	Upcoming       []*Row
	OverdueCount   int
	SoonCount      int
	DonationsTotal int64
	DonationsMonth int64
	Donations12m   int64
	DonorsCount    int
	PaidTotal      int64
	PaidMonth      int64
	Paid12m        int64
	Balance        int64
	// Coverage is the percent of the monthly expenses covered by donations of the current month.
	Coverage int
}

// CoveragePercent is Coverage clamped to a progress bar.
func (s *Summary) CoveragePercent() int { return min(max(s.Coverage, 0), 100) }

// Rows wraps expenses with their rouble values and status on now.
func Rows(list []*expense_model.Expense, rates money.Rates, now time.Time) []*Row {
	rows := make([]*Row, 0, len(list))
	for _, e := range list {
		days, _ := money.DaysUntil(e.NextDue, now)
		rows = append(rows, &Row{
			Expense: e,
			Monthly: money.Monthly(rates.ToBase(e.Amount, e.Currency), e.Period),
			Status:  e.Status(now),
			Days:    days,
		})
	}
	return rows
}

// GroupRows groups rows by category, keeping their order; inactive rows add nothing to the sum.
func GroupRows(rows []*Row) []*Group {
	var groups []*Group
	for _, r := range rows {
		if len(groups) == 0 || groups[len(groups)-1].Category != r.CategoryName {
			groups = append(groups, &Group{Category: r.CategoryName})
		}
		g := groups[len(groups)-1]
		g.Rows = append(g.Rows, r)
		if r.IsActive {
			g.Monthly += r.Monthly
		}
	}
	return groups
}

// Summarize computes the totals.
func Summarize(rows []*Row, payments []*expense_model.Payment, donations []*donation_model.Donation, rates money.Rates, now time.Time) *Summary {
	s := &Summary{}
	cats, owners := map[string]*Total{}, map[string]*Total{}
	var catOrder []string
	for _, r := range rows {
		if !r.IsActive {
			continue
		}
		s.ActiveCount++
		if !r.IsPublic {
			s.HiddenCount++
		}
		yearly := money.Yearly(rates.ToBase(r.Amount, r.Currency), r.Period)
		s.MonthlyTotal += r.Monthly
		s.YearlyTotal += yearly
		if cats[r.CategoryName] == nil {
			cats[r.CategoryName] = &Total{Name: r.CategoryName}
			catOrder = append(catOrder, r.CategoryName)
		}
		owner := r.Owner
		if owner == "" {
			owner = "—"
		}
		if owners[owner] == nil {
			owners[owner] = &Total{Name: owner}
		}
		for _, t := range []*Total{cats[r.CategoryName], owners[owner]} {
			t.Count++
			t.Monthly += r.Monthly
			t.Yearly += yearly
		}
		switch r.Status {
		case expense_model.StatusOverdue:
			s.OverdueCount++
			s.Upcoming = append(s.Upcoming, r)
		case expense_model.StatusSoon:
			s.SoonCount++
			s.Upcoming = append(s.Upcoming, r)
		case expense_model.StatusOK:
			if r.Days <= UpcomingDays {
				s.Upcoming = append(s.Upcoming, r)
			}
		}
	}
	for _, k := range catOrder {
		s.ByCategory = append(s.ByCategory, *cats[k])
	}
	for _, t := range owners {
		s.ByOwner = append(s.ByOwner, *t)
	}
	sort.Slice(s.ByOwner, func(i, j int) bool {
		if s.ByOwner[i].Monthly != s.ByOwner[j].Monthly {
			return s.ByOwner[i].Monthly > s.ByOwner[j].Monthly
		}
		return s.ByOwner[i].Name < s.ByOwner[j].Name
	})
	sort.SliceStable(s.Upcoming, func(i, j int) bool { return s.Upcoming[i].NextDue < s.Upcoming[j].NextDue })

	today := money.Today(now)
	monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, money.Moscow).Format(money.DateLayout)
	yearAgo := today.AddDate(-1, 0, 0).Format(money.DateLayout)
	donors := map[string]bool{}
	for _, d := range donations {
		v := rates.ToBase(d.Amount, d.Currency)
		s.DonationsTotal += v
		if d.DonatedOn >= monthStart {
			s.DonationsMonth += v
		}
		if d.DonatedOn >= yearAgo {
			s.Donations12m += v
		}
		if name := strings.ToLower(d.Donor); name != "" {
			donors[name] = true
		} else {
			s.DonorsCount++ // every anonymous donation counts as a separate person
		}
	}
	s.DonorsCount += len(donors)
	for _, p := range payments {
		v := rates.ToBase(p.Amount, p.Currency)
		s.PaidTotal += v
		if p.PaidOn >= monthStart {
			s.PaidMonth += v
		}
		if p.PaidOn >= yearAgo {
			s.Paid12m += v
		}
	}
	s.Balance = s.DonationsTotal - s.PaidTotal
	if s.MonthlyTotal > 0 {
		s.Coverage = int(math.Round(float64(s.DonationsMonth) * 100 / float64(s.MonthlyTotal)))
	}
	return s
}

// RecordPayment stores a payment and, when advance is set, moves the next due date
// by the periods it covers. It returns the new due date.
func RecordPayment(ctx context.Context, e *expense_model.Expense, p *expense_model.Payment, advance bool) (string, error) {
	next := e.NextDue
	err := db.WithTx(ctx, func(ctx context.Context) error {
		p.ExpenseID = e.ID
		if err := expense_model.CreatePayment(ctx, p); err != nil {
			return err
		}
		if !advance || e.Period == "once" {
			return nil
		}
		next = money.AdvanceDue(e.NextDue, p.PaidOn, e.Period, p.Periods)
		return expense_model.SetNextDue(ctx, e.ID, next)
	})
	return next, err
}
