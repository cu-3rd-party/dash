// Package category groups expenses: servers, domains, …
package category

import (
	"context"
	"fmt"
	"regexp"
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
	_, err := db.GetEngine(ctx).ID(c.ID).Cols("name", "slug", "description", "sort_order").Update(c)
	return err
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
