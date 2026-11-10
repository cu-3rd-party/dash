// Package category groups expenses: servers, domains, …
package category

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"git.cu3rd.ru/cu3rd/dash/models/db"
)

// Category is a group of expenses.
type Category struct {
	ID          int64  `xorm:"pk autoincr"`
	Name        string `xorm:"VARCHAR(100) NOT NULL"`
	Slug        string `xorm:"VARCHAR(40) NOT NULL UNIQUE"`
	Description string `xorm:"VARCHAR(500) NOT NULL DEFAULT ''"`
	SortOrder   int    `xorm:"NOT NULL DEFAULT 0"`

	ExpenseCount int64 `xorm:"-"`
}

func init() {
	db.RegisterModel(new(Category))
}

// ErrCategoryNotExist is returned for an unknown id.
type ErrCategoryNotExist struct{ ID int64 }

func (err ErrCategoryNotExist) Error() string {
	return fmt.Sprintf("категория #%d не найдена", err.ID)
}
func (err ErrCategoryNotExist) Unwrap() error { return db.ErrNotExist }

// ErrInvalidCategory is returned for bad input.
type ErrInvalidCategory struct{ Reason string }

func (err ErrInvalidCategory) Error() string { return err.Reason }
func (err ErrInvalidCategory) Unwrap() error { return db.ErrInvalidArgument }

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// Validate normalizes and checks the fields.
func (c *Category) Validate() error {
	c.Name = strings.TrimSpace(c.Name)
	c.Slug = strings.ToLower(strings.TrimSpace(c.Slug))
	c.Description = strings.TrimSpace(c.Description)
	switch {
	case c.Name == "":
		return ErrInvalidCategory{Reason: "укажите название категории"}
	case utf8.RuneCountInString(c.Name) > 100 || utf8.RuneCountInString(c.Description) > 500:
		return ErrInvalidCategory{Reason: "слишком длинное название или описание"}
	case !slugRe.MatchString(c.Slug):
		return ErrInvalidCategory{Reason: "код — латиница, цифры и дефис, например «saas»"}
	}
	return nil
}

// Get returns a category.
func Get(ctx context.Context, id int64) (*Category, error) {
	c := new(Category)
	has, err := db.GetEngine(ctx).ID(id).Get(c)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrCategoryNotExist{ID: id}
	}
	return c, nil
}

// List returns categories in display order, with the number of expenses in each.
func List(ctx context.Context) ([]*Category, error) {
	cats := make([]*Category, 0, 8)
	if err := db.GetEngine(ctx).Asc("sort_order", "name").Find(&cats); err != nil {
		return nil, err
	}
	type row struct {
		CategoryID int64
		N          int64
	}
	var rows []row
	if err := db.GetEngine(ctx).Table("expense").Select("category_id, COUNT(*) AS n").GroupBy("category_id").Find(&rows); err != nil {
		return nil, err
	}
	counts := map[int64]int64{}
	for _, r := range rows {
		counts[r.CategoryID] = r.N
	}
	for _, c := range cats {
		c.ExpenseCount = counts[c.ID]
	}
	return cats, nil
}

// Create inserts a category.
func Create(ctx context.Context, c *Category) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := checkSlugFree(ctx, c); err != nil {
		return err
	}
	if c.SortOrder == 0 {
		// a new category goes to the end of the list
		var last struct{ Max int }
		if _, err := db.GetEngine(ctx).Table("category").Select("COALESCE(MAX(sort_order), 0) AS max").Get(&last); err != nil {
			return err
		}
		c.SortOrder = last.Max + orderStep
	}
	_, err := db.GetEngine(ctx).Insert(c)
	return err
}

// Update saves the editable fields.
func Update(ctx context.Context, c *Category) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := checkSlugFree(ctx, c); err != nil {
		return err
	}
	_, err := db.GetEngine(ctx).ID(c.ID).Cols("name", "slug", "description").Update(c)
	return err
}

// orderStep spaces the positions so that renumbering is rarely needed.
const orderStep = 10

// Move shifts a category one position up (delta -1) or down (delta 1) and
// renumbers all positions 10, 20, 30… At the edge of the list it does nothing.
func Move(ctx context.Context, id int64, delta int) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		cats := make([]*Category, 0, 8)
		if err := db.GetEngine(ctx).Asc("sort_order", "name").Find(&cats); err != nil {
			return err
		}
		i := slices.IndexFunc(cats, func(c *Category) bool { return c.ID == id })
		if i < 0 {
			return ErrCategoryNotExist{ID: id}
		}
		if j := i + delta; j >= 0 && j < len(cats) {
			cats[i], cats[j] = cats[j], cats[i]
		}
		for n, c := range cats {
			if want := (n + 1) * orderStep; c.SortOrder != want {
				c.SortOrder = want
				if _, err := db.GetEngine(ctx).ID(c.ID).Cols("sort_order").Update(c); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func checkSlugFree(ctx context.Context, c *Category) error {
	n, err := db.GetEngine(ctx).Where("slug = ? AND id <> ?", c.Slug, c.ID).Count(new(Category))
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrInvalidCategory{Reason: "код «" + c.Slug + "» уже занят"}
	}
	return nil
}

// Delete removes an empty category.
func Delete(ctx context.Context, id int64) error {
	n, err := db.GetEngine(ctx).Table("expense").Where("category_id = ?", id).Count()
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrInvalidCategory{Reason: fmt.Sprintf("в категории %d статей расходов: сначала перенесите или удалите их", n)}
	}
	_, err = db.GetEngine(ctx).ID(id).Delete(new(Category))
	return err
}
